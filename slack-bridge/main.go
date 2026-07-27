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
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type config struct {
	slackBotToken, slackAppToken, slackChannelID string
	opencodeURL, opencodeUser, opencodePass       string
}

type bridge struct {
	cfg config
	hc  *http.Client
	mu  sync.Mutex
	mapByThread map[string]string
}

type envelope struct { ID string `json:"envelope_id"`; Payload json.RawMessage `json:"payload"` }
type callback struct {
	Type string `json:"type"`
	Event struct {
		Type     string `json:"type"`
		Channel  string `json:"channel"`
		User     string `json:"user"`
		Text     string `json:"text"`
		TS       string `json:"ts"`
		ThreadTS string `json:"thread_ts"`
		BotID    string `json:"bot_id"`
	} `json:"event"`
}
type sessionResult struct { ID string `json:"id"` }
type messageResult struct { Parts []struct { Type, Text string } `json:"parts"` }

var mention = regexp.MustCompile(`<@[^>]+>\s*`)

func main() {
	b := &bridge{
		cfg: config{
			slackBotToken: required("SLACK_BOT_TOKEN"), slackAppToken: required("SLACK_APP_TOKEN"),
			slackChannelID: os.Getenv("SLACK_CHANNEL_ID"),
			opencodeURL: strings.TrimRight(value("OPENCODE_URL", "http://opencode:4096"), "/"),
			opencodeUser: value("OPENCODE_SERVER_USERNAME", "opencode"), opencodePass: os.Getenv("OPENCODE_SERVER_PASSWORD"),
		},
		hc: &http.Client{Timeout: 60 * time.Second}, mapByThread: make(map[string]string),
	}
	go func() { http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }); log.Fatal(http.ListenAndServe(":8080", nil)) }()
	for { if err := b.run(); err != nil { log.Printf("socket connection failed: %v", err); time.Sleep(5 * time.Second) } }
}

func (b *bridge) run() error {
	req, err := http.NewRequest(http.MethodPost, "https://slack.com/api/apps.connections.open", nil); if err != nil { return err }
	req.Header.Set("Authorization", "Bearer "+b.cfg.slackAppToken)
	res, err := b.hc.Do(req); if err != nil { return err }; defer res.Body.Close()
	var opened struct { OK bool `json:"ok"`; URL, Error string }; if err := json.NewDecoder(res.Body).Decode(&opened); err != nil { return err }
	if !opened.OK || opened.URL == "" { return fmt.Errorf("apps.connections.open: %s", opened.Error) }
	socket, _, err := (&websocket.Dialer{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}).Dial(opened.URL, nil); if err != nil { return err }; defer socket.Close()
	for {
		_, raw, err := socket.ReadMessage(); if err != nil { return err }
		var e envelope; if err := json.Unmarshal(raw, &e); err != nil { return err }; if e.ID == "" { continue }
		if err := socket.WriteJSON(map[string]string{"envelope_id": e.ID}); err != nil { return err }
		var c callback; if err := json.Unmarshal(e.Payload, &c); err != nil { log.Printf("invalid event: %v", err); continue }
		if c.Type != "event_callback" || c.Event.Type != "app_mention" || c.Event.BotID != "" || (b.cfg.slackChannelID != "" && c.Event.Channel != b.cfg.slackChannelID) { continue }
		go b.handle(c.Event.Channel, c.Event.TS, c.Event.ThreadTS, c.Event.Text)
	}
}

func (b *bridge) handle(channel, ts, threadTS, text string) {
	text = strings.TrimSpace(mention.ReplaceAllString(text, "")); if text == "" { text = "Please describe what you need help with." }
	root := threadTS; if root == "" { root = ts }
	session, err := b.session(channel, root); var response string; if err == nil { response, err = b.prompt(session, text) }
	if err != nil { log.Printf("processing Slack message: %v", err); response = "I could not process that request." }; if response == "" { response = "The request completed without a text response." }
	if err := b.post(channel, root, response); err != nil { log.Printf("posting Slack response: %v", err) }
}

func (b *bridge) session(channel, thread string) (string, error) {
	key := channel + ":" + thread; b.mu.Lock(); defer b.mu.Unlock(); if id := b.mapByThread[key]; id != "" { return id, nil }
	body, _ := json.Marshal(map[string]string{"title": "Slack " + key}); var result sessionResult; if err := b.openCode(http.MethodPost, "/session", body, &result); err != nil { return "", err }; if result.ID == "" { return "", errors.New("OpenCode returned an empty session ID") }; b.mapByThread[key] = result.ID; return result.ID, nil
}

func (b *bridge) prompt(sessionID, text string) (string, error) {
	body, _ := json.Marshal(map[string]any{"parts": []map[string]string{{"type": "text", "text": text}}}); var result messageResult
	if err := b.openCode(http.MethodPost, "/session/"+url.PathEscape(sessionID)+"/message", body, &result); err != nil { return "", err }
	var answer strings.Builder; for _, p := range result.Parts { if p.Type == "text" { answer.WriteString(p.Text) } }; return strings.TrimSpace(answer.String()), nil
}

func (b *bridge) openCode(method, path string, body []byte, result any) error {
	req, err := http.NewRequest(method, b.cfg.opencodeURL+path, bytes.NewReader(body)); if err != nil { return err }; req.Header.Set("Content-Type", "application/json"); if b.cfg.opencodePass != "" { req.SetBasicAuth(b.cfg.opencodeUser, b.cfg.opencodePass) }
	res, err := b.hc.Do(req); if err != nil { return err }; defer res.Body.Close(); if res.StatusCode < 200 || res.StatusCode >= 300 { data, _ := io.ReadAll(io.LimitReader(res.Body, 4096)); return fmt.Errorf("OpenCode HTTP %s: %s", res.Status, strings.TrimSpace(string(data))) }; return json.NewDecoder(res.Body).Decode(result)
}

func (b *bridge) post(channel, thread, text string) error {
	body, _ := json.Marshal(map[string]string{"channel": channel, "thread_ts": thread, "text": text}); req, err := http.NewRequest(http.MethodPost, "https://slack.com/api/chat.postMessage", bytes.NewReader(body)); if err != nil { return err }; req.Header.Set("Authorization", "Bearer "+b.cfg.slackBotToken); req.Header.Set("Content-Type", "application/json")
	res, err := b.hc.Do(req); if err != nil { return err }; defer res.Body.Close(); var result struct { OK bool `json:"ok"`; Error string `json:"error"` }; if err := json.NewDecoder(res.Body).Decode(&result); err != nil { return err }; if !result.OK { return errors.New(result.Error) }; return nil
}

func required(name string) string { if v := os.Getenv(name); v != "" { return v }; log.Fatalf("%s is required", name); return "" }
func value(name, fallback string) string { if v := os.Getenv(name); v != "" { return v }; return fallback }
