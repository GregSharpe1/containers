package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type config struct {
	slackBotToken, slackAppToken, slackChannelID string
	slackAllowDMs                                bool
	opencodeURL, opencodeUser, opencodePass      string
}

type bridge struct {
	cfg          config
	hc           *http.Client
	opencodeHC   *http.Client
	mu           sync.Mutex
	mapByThread  map[string]string
	allowedUsers map[string]struct{}
}

type envelope struct {
	ID      string          `json:"envelope_id"`
	Payload json.RawMessage `json:"payload"`
}
type callback struct {
	Type  string     `json:"type"`
	Event slackEvent `json:"event"`
}
type slackEvent struct {
	Type        string `json:"type"`
	Subtype     string `json:"subtype"`
	Channel     string `json:"channel"`
	User        string `json:"user"`
	Text        string `json:"text"`
	TS          string `json:"ts"`
	ThreadTS    string `json:"thread_ts"`
	BotID       string `json:"bot_id"`
	ChannelType string `json:"channel_type"`
}
type sessionResult struct {
	ID string `json:"id"`
}
type messageResult struct {
	Parts []struct{ Type, Text string } `json:"parts"`
}

var mention = regexp.MustCompile(`<@[^>]+>\s*`)

func main() {
	b := &bridge{
		cfg: config{
			slackBotToken: required("SLACK_BOT_TOKEN"), slackAppToken: required("SLACK_APP_TOKEN"),
			slackChannelID: os.Getenv("SLACK_CHANNEL_ID"),
			slackAllowDMs:  enabled("SLACK_ALLOW_DMS", false),
			opencodeURL:    strings.TrimRight(value("OPENCODE_URL", "http://opencode:4096"), "/"),
			opencodeUser:   value("OPENCODE_SERVER_USERNAME", "opencode"), opencodePass: os.Getenv("OPENCODE_SERVER_PASSWORD"),
		},
		hc: &http.Client{Timeout: 60 * time.Second}, mapByThread: make(map[string]string),
		opencodeHC:   &http.Client{Timeout: duration("OPENCODE_REQUEST_TIMEOUT", 30*time.Minute)},
		allowedUsers: parseAllowedUsers(os.Getenv("SLACK_ALLOWED_USER_IDS")),
	}
	go func() {
		http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		log.Fatal(http.ListenAndServe(":8080", nil))
	}()
	for {
		if err := b.run(); err != nil {
			log.Printf("socket connection failed: %v", err)
			time.Sleep(5 * time.Second)
		}
	}
}

func (b *bridge) run() error {
	req, err := http.NewRequest(http.MethodPost, "https://slack.com/api/apps.connections.open", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+b.cfg.slackAppToken)
	res, err := b.hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var opened struct {
		OK         bool `json:"ok"`
		URL, Error string
	}
	if err := json.NewDecoder(res.Body).Decode(&opened); err != nil {
		return err
	}
	if !opened.OK || opened.URL == "" {
		return fmt.Errorf("apps.connections.open: %s", opened.Error)
	}
	socket, _, err := (&websocket.Dialer{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}).Dial(opened.URL, nil)
	if err != nil {
		return err
	}
	defer socket.Close()
	for {
		_, raw, err := socket.ReadMessage()
		if err != nil {
			return err
		}
		var e envelope
		if err := json.Unmarshal(raw, &e); err != nil {
			return err
		}
		if e.ID == "" {
			continue
		}
		if err := socket.WriteJSON(map[string]string{"envelope_id": e.ID}); err != nil {
			return err
		}
		var c callback
		if err := json.Unmarshal(e.Payload, &c); err != nil {
			log.Printf("invalid event: %v", err)
			continue
		}
		if c.Type != "event_callback" || !b.shouldHandle(c.Event) {
			continue
		}
		go b.handle(c.Event.Channel, c.Event.ChannelType, c.Event.TS, c.Event.ThreadTS, c.Event.Text)
	}
}

func (b *bridge) shouldHandle(event slackEvent) bool {
	if event.BotID != "" || !b.isAllowedUser(event.User) {
		return false
	}
	if event.ChannelType == "im" {
		return b.cfg.slackAllowDMs && event.Type == "message" && event.Subtype == ""
	}
	if b.cfg.slackChannelID != "" && event.Channel != b.cfg.slackChannelID {
		return false
	}
	if event.Type == "app_mention" {
		return true
	}
	if event.Type != "message" || event.Subtype != "" || event.ThreadTS == "" {
		return false
	}
	return b.hasSession(event.Channel, event.ThreadTS)
}

func (b *bridge) handle(channel, channelType, ts, threadTS, text string) {
	if err := b.react(channel, ts, "eyes"); err != nil {
		log.Printf("acknowledging Slack message: %v", err)
	}
	text = strings.TrimSpace(mention.ReplaceAllString(text, ""))
	if text == "" {
		text = "Please describe what you need help with."
	}
	root, replyThread := threadTS, threadTS
	if root == "" {
		root, replyThread = ts, ts
	}
	if channelType == "im" {
		root, replyThread = channel, ""
	}
	session, err := b.session(channel, root)
	var response string
	if err == nil {
		response, err = b.prompt(session, text)
	}
	if err != nil {
		log.Printf("processing Slack message: %v", err)
		response = "I could not process that request."
	}
	if response == "" {
		response = "The request completed without a text response."
	}
	if err := b.post(channel, replyThread, response); err != nil {
		log.Printf("posting Slack response: %v", err)
		return
	}
	if err := b.unreact(channel, ts, "eyes"); err != nil {
		log.Printf("removing Slack acknowledgement: %v", err)
	}
	if err := b.react(channel, ts, "white_check_mark"); err != nil {
		log.Printf("marking Slack message complete: %v", err)
	}
}

func (b *bridge) session(channel, thread string) (string, error) {
	key := channel + ":" + thread
	b.mu.Lock()
	defer b.mu.Unlock()
	if id := b.mapByThread[key]; id != "" {
		return id, nil
	}
	body, _ := json.Marshal(map[string]string{"title": "Slack " + key})
	var result sessionResult
	if err := b.openCode(http.MethodPost, "/session", body, &result); err != nil {
		return "", err
	}
	if result.ID == "" {
		return "", errors.New("OpenCode returned an empty session ID")
	}
	b.mapByThread[key] = result.ID
	return result.ID, nil
}

func (b *bridge) hasSession(channel, thread string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.mapByThread[channel+":"+thread] != ""
}

func (b *bridge) prompt(sessionID, text string) (string, error) {
	body, _ := json.Marshal(map[string]any{"parts": []map[string]string{{"type": "text", "text": text}}})
	var result messageResult
	if err := b.openCode(http.MethodPost, "/session/"+url.PathEscape(sessionID)+"/message", body, &result); err != nil {
		return "", err
	}
	var answer strings.Builder
	for _, p := range result.Parts {
		if p.Type == "text" {
			answer.WriteString(p.Text)
		}
	}
	return strings.TrimSpace(answer.String()), nil
}

func (b *bridge) openCode(method, path string, body []byte, result any) error {
	req, err := http.NewRequest(method, b.cfg.opencodeURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if b.cfg.opencodePass != "" {
		req.SetBasicAuth(b.cfg.opencodeUser, b.cfg.opencodePass)
	}
	client := b.hc
	if b.opencodeHC != nil {
		client = b.opencodeHC
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("OpenCode HTTP %s: %s", res.Status, strings.TrimSpace(string(data)))
	}
	return json.NewDecoder(res.Body).Decode(result)
}

func (b *bridge) post(channel, thread, text string) error {
	bodyData := map[string]string{"channel": channel, "text": text}
	if thread != "" {
		bodyData["thread_ts"] = thread
	}
	body, _ := json.Marshal(bodyData)
	req, err := http.NewRequest(http.MethodPost, "https://slack.com/api/chat.postMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+b.cfg.slackBotToken)
	req.Header.Set("Content-Type", "application/json")
	res, err := b.hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New(result.Error)
	}
	return nil
}

func (b *bridge) react(channel, ts, name string) error {
	return b.reaction("reactions.add", channel, ts, name)
}

func (b *bridge) unreact(channel, ts, name string) error {
	return b.reaction("reactions.remove", channel, ts, name)
}

func (b *bridge) reaction(method, channel, ts, name string) error {
	body, _ := json.Marshal(map[string]string{"channel": channel, "timestamp": ts, "name": name})
	req, err := http.NewRequest(http.MethodPost, "https://slack.com/api/"+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+b.cfg.slackBotToken)
	req.Header.Set("Content-Type", "application/json")
	res, err := b.hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New(result.Error)
	}
	return nil
}

func required(name string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	log.Fatalf("%s is required", name)
	return ""
}
func value(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func enabled(name string, fallback bool) bool {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		log.Fatalf("%s must be a boolean: %s", name, v)
	}
	return parsed
}

func duration(name string, fallback time.Duration) time.Duration {
	value := value(name, fallback.String())
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		log.Fatalf("%s must be a positive duration: %s", name, value)
	}
	return parsed
}

func parseAllowedUsers(value string) map[string]struct{} {
	users := make(map[string]struct{})
	for _, user := range strings.Split(value, ",") {
		if user = strings.TrimSpace(user); user != "" {
			users[user] = struct{}{}
		}
	}
	return users
}

func (b *bridge) isAllowedUser(user string) bool {
	_, allowed := b.allowedUsers[user]
	return allowed
}
