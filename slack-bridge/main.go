package main

import (
	"bufio"
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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type config struct {
	slackBotToken, slackAppToken            string
	slackChannelIDs                          map[string]struct{}
	slackAllowDMs                            bool
	opencodeURL, opencodeUser, opencodePass string
}

type bridge struct {
	cfg             config
	hc              *http.Client
	opencodeHC      *http.Client
	mu              sync.Mutex
	mapByThread     map[string]string
	threadBySession map[string]conversation
	pendingByID     map[string]pendingInteraction
	allowedUsers    map[string]struct{}
}

type conversation struct {
	channel string
	thread  string
}

type pendingInteraction struct {
	sessionID    string
	conversation conversation
	question     *questionRequest
	answers      map[int][]string
}

type envelope struct {
	ID      string          `json:"envelope_id"`
	Payload json.RawMessage `json:"payload"`
}
type callback struct {
	Type    string          `json:"type"`
	Event   slackEvent      `json:"event"`
	Payload json.RawMessage `json:"payload"`
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
type slackThreadMessage struct {
	TS   string `json:"ts"`
	Text string `json:"text"`
}
type sessionResult struct {
	ID string `json:"id"`
}
type messageResult struct {
	Parts []struct{ Type, Text string } `json:"parts"`
}
type openCodeEvent struct {
	Type       string          `json:"type"`
	Properties json.RawMessage `json:"properties"`
}
type permissionRequest struct {
	ID, SessionID, Permission string
	Patterns                  []string
}
type questionOption struct {
	Label, Description string
}
type questionInfo struct {
	Question, Header string
	Options          []questionOption
	Multiple, Custom bool
}
type questionRequest struct {
	ID, SessionID string
	Questions     []questionInfo
}
type slackInteraction struct {
	Type    string `json:"type"`
	User    struct{ ID string } `json:"user"`
	Channel struct{ ID string } `json:"channel"`
	Message struct {
		TS       string `json:"ts"`
		ThreadTS string `json:"thread_ts"`
	} `json:"message"`
	Actions []struct {
		ActionID        string `json:"action_id"`
		Value           string `json:"value"`
		SelectedOption  *struct{ Value string `json:"value"` } `json:"selected_option"`
		SelectedOptions []struct{ Value string `json:"value"` } `json:"selected_options"`
	} `json:"actions"`
}
type questionAnswerAction struct {
	ID    string `json:"id"`
	Index int    `json:"index"`
	Answer string `json:"answer"`
}

var mention = regexp.MustCompile(`<@[^>]+>\s*`)

func main() {
	b := &bridge{
		cfg: config{
			slackBotToken: required("SLACK_BOT_TOKEN"), slackAppToken: required("SLACK_APP_TOKEN"),
			slackChannelIDs: parseIDs(os.Getenv("SLACK_CHANNEL_ID")),
			slackAllowDMs:  enabled("SLACK_ALLOW_DMS", false),
			opencodeURL:    strings.TrimRight(value("OPENCODE_URL", "http://opencode:4096"), "/"),
			opencodeUser:   value("OPENCODE_SERVER_USERNAME", "opencode"), opencodePass: os.Getenv("OPENCODE_SERVER_PASSWORD"),
		},
		hc: &http.Client{Timeout: 60 * time.Second}, mapByThread: make(map[string]string),
		threadBySession: make(map[string]conversation), pendingByID: make(map[string]pendingInteraction),
		opencodeHC:   &http.Client{Timeout: duration("OPENCODE_REQUEST_TIMEOUT", 30*time.Minute)},
		allowedUsers: parseAllowedUsers(os.Getenv("SLACK_ALLOWED_USER_IDS")),
	}
	log.Printf("starting Slack bridge: channels=%s allowed_users=%d allow_dms=%t opencode_url=%s", formatIDs(b.cfg.slackChannelIDs), len(b.allowedUsers), b.cfg.slackAllowDMs, b.cfg.opencodeURL)
	go b.watchEvents()
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
	log.Printf("connected to Slack Socket Mode")
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
		switch c.Type {
		case "event_callback":
			if b.shouldHandle(c.Event) {
				log.Printf("handling Slack event: type=%s channel=%s channel_type=%s user=%s ts=%s thread_ts=%s", c.Event.Type, c.Event.Channel, c.Event.ChannelType, c.Event.User, c.Event.TS, c.Event.ThreadTS)
				go b.handle(c.Event.Channel, c.Event.ChannelType, c.Event.TS, c.Event.ThreadTS, c.Event.Text)
			}
		case "interactive":
			go b.handleInteraction(c.Payload)
		}
	}
}

func (b *bridge) shouldHandle(event slackEvent) bool {
	if event.BotID != "" {
		return false
	}
	if !b.isAllowedUser(event.User) {
		log.Printf("ignoring Slack event from unauthorized user: type=%s channel=%s user=%s", event.Type, event.Channel, event.User)
		return false
	}
	if event.ChannelType == "im" {
		return b.cfg.slackAllowDMs && event.Type == "message" && event.Subtype == ""
	}
	if len(b.cfg.slackChannelIDs) > 0 {
		if _, allowed := b.cfg.slackChannelIDs[event.Channel]; !allowed {
			log.Printf("ignoring Slack event from unconfigured channel: type=%s channel=%s configured_channels=%s", event.Type, event.Channel, formatIDs(b.cfg.slackChannelIDs))
			return false
		}
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
	log.Printf("processing Slack message: channel=%s ts=%s thread_ts=%s", channel, ts, threadTS)
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
	if b.answerQuestionText(channel, root, text) {
		_ = b.unreact(channel, ts, "eyes")
		_ = b.react(channel, ts, "white_check_mark")
		return
	}
	if threadTS != "" && !b.hasSession(channel, root) {
		context, err := b.threadContext(channel, root, ts)
		if err != nil {
			log.Printf("loading Slack thread context: channel=%s thread=%s error=%v", channel, root, err)
		} else if context != "" {
			text = "Slack thread context (messages before this request):\n" + context + "\n\nCurrent request:\n" + text
		}
	}
	session, err := b.session(channel, root)
	var response string
	if err == nil {
		response, err = b.prompt(session, text)
	}
	if err != nil {
		log.Printf("processing Slack message failed: channel=%s root=%s error=%v", channel, root, err)
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
		log.Printf("reusing OpenCode session: session=%s channel=%s thread=%s", id, channel, thread)
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
	if b.threadBySession == nil {
		b.threadBySession = make(map[string]conversation)
	}
	b.mapByThread[key] = result.ID
	b.threadBySession[result.ID] = conversation{channel: channel, thread: thread}
	log.Printf("created OpenCode session: session=%s channel=%s thread=%s", result.ID, channel, thread)
	return result.ID, nil
}

func (b *bridge) hasSession(channel, thread string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.mapByThread[channel+":"+thread] != ""
}

func (b *bridge) threadContext(channel, thread, currentTS string) (string, error) {
	endpoint, _ := url.Parse("https://slack.com/api/conversations.replies")
	query := endpoint.Query()
	query.Set("channel", channel)
	query.Set("ts", thread)
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+b.cfg.slackBotToken)
	res, err := b.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("Slack conversations.replies HTTP %s", res.Status)
	}
	var result struct {
		OK       bool                 `json:"ok"`
		Error    string               `json:"error"`
		Messages []slackThreadMessage `json:"messages"`
	}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return "", err
	}
	if !result.OK {
		return "", fmt.Errorf("Slack conversations.replies: %s", result.Error)
	}
	var messages []string
	for _, message := range result.Messages {
		if message.TS == currentTS || strings.TrimSpace(message.Text) == "" {
			continue
		}
		messages = append(messages, fmt.Sprintf("[%s] %s", message.TS, strings.TrimSpace(message.Text)))
	}
	return strings.Join(messages, "\n"), nil
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

// watchEvents keeps a separate connection open so a blocked message request can
// still be surfaced to Slack and resumed by an authorized user.
func (b *bridge) watchEvents() {
	for {
		if err := b.watchEventsOnce(); err != nil {
			log.Printf("watching OpenCode events: %v", err)
		}
		time.Sleep(5 * time.Second)
	}
}

func (b *bridge) watchEventsOnce() error {
	req, err := http.NewRequest(http.MethodGet, b.cfg.opencodeURL+"/event", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	if b.cfg.opencodePass != "" {
		req.SetBasicAuth(b.cfg.opencodeUser, b.cfg.opencodePass)
	}
	client := b.opencodeHC
	if client == nil {
		client = b.hc
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("OpenCode events HTTP %s: %s", res.Status, strings.TrimSpace(string(data)))
	}
	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event openCodeEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			log.Printf("decoding OpenCode event: %v", err)
			continue
		}
		b.handleOpenCodeEvent(event)
	}
	return scanner.Err()
}

func (b *bridge) handleOpenCodeEvent(event openCodeEvent) {
	switch event.Type {
	case "permission.asked":
		var request permissionRequest
		if json.Unmarshal(event.Properties, &request) != nil || request.ID == "" {
			return
		}
		conversation, ok := b.conversationForSession(request.SessionID)
		if !ok {
			return
		}
		b.mu.Lock()
		if b.pendingByID == nil {
			b.pendingByID = make(map[string]pendingInteraction)
		}
		if _, exists := b.pendingByID[request.ID]; exists {
			b.mu.Unlock()
			return
		}
		b.pendingByID[request.ID] = pendingInteraction{sessionID: request.SessionID, conversation: conversation}
		b.mu.Unlock()
		text := fmt.Sprintf("OpenCode needs approval for `%s`.", request.Permission)
		if len(request.Patterns) > 0 {
			text += "\nRequested: `" + strings.Join(request.Patterns, "`, `") + "`"
		}
		if err := b.postBlocks(conversation.channel, conversation.thread, text, []map[string]any{{
			"type": "actions", "elements": []map[string]any{
				button("permission.once", request.ID, "Allow once", "primary"),
				button("permission.always", request.ID, "Always allow", "primary"),
				button("permission.reject", request.ID, "Deny", "danger"),
			},
		}}); err != nil {
			log.Printf("posting permission request to Slack: %v", err)
		}
	case "question.asked":
		var request questionRequest
		if json.Unmarshal(event.Properties, &request) != nil || request.ID == "" || len(request.Questions) == 0 {
			return
		}
		conversation, ok := b.conversationForSession(request.SessionID)
		if !ok {
			return
		}
		b.mu.Lock()
		if b.pendingByID == nil {
			b.pendingByID = make(map[string]pendingInteraction)
		}
		if _, exists := b.pendingByID[request.ID]; exists {
			b.mu.Unlock()
			return
		}
		b.pendingByID[request.ID] = pendingInteraction{sessionID: request.SessionID, conversation: conversation, question: &request, answers: make(map[int][]string)}
		b.mu.Unlock()
		if err := b.postQuestion(conversation, request); err != nil {
			log.Printf("posting question to Slack: %v", err)
		}
	}
}

func button(actionID, value, text, style string) map[string]any {
	button := map[string]any{"type": "button", "action_id": actionID, "value": value, "text": map[string]string{"type": "plain_text", "text": text}}
	if style != "" {
		button["style"] = style
	}
	return button
}

func (b *bridge) postQuestion(conversation conversation, request questionRequest) error {
	blocks := make([]map[string]any, 0, len(request.Questions)+2)
	for index, question := range request.Questions {
		text := "*" + question.Header + "*\n" + question.Question
		if question.Custom {
			text += "\nReply in this thread to provide a custom answer."
		}
		blocks = append(blocks, map[string]any{"type": "section", "text": map[string]string{"type": "mrkdwn", "text": text}})
		elements := make([]map[string]any, 0, len(question.Options))
		for _, option := range question.Options {
			value, _ := json.Marshal(questionAnswerAction{ID: request.ID, Index: index, Answer: option.Label})
			elements = append(elements, button("question.answer", string(value), option.Label, ""))
		}
		if len(elements) > 0 {
			blocks = append(blocks, map[string]any{"type": "actions", "elements": elements})
		}
	}
	if questionNeedsSubmit(request) {
		blocks = append(blocks, map[string]any{"type": "actions", "elements": []map[string]any{button("question.submit", request.ID, "Submit answers", "primary")}})
	}
	return b.postBlocks(conversation.channel, conversation.thread, "OpenCode needs your input.", blocks)
}

func (b *bridge) conversationForSession(sessionID string) (conversation, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	conversation, ok := b.threadBySession[sessionID]
	return conversation, ok
}

func (b *bridge) handleInteraction(raw json.RawMessage) {
	var interaction slackInteraction
	if err := json.Unmarshal(raw, &interaction); err != nil {
		var encoded string
		if json.Unmarshal(raw, &encoded) != nil || json.Unmarshal([]byte(encoded), &interaction) != nil {
			return
		}
	}
	if interaction.Type != "block_actions" || !b.isAllowedUser(interaction.User.ID) || len(interaction.Actions) != 1 {
		return
	}
	action := interaction.Actions[0]
	switch action.ActionID {
	case "permission.once", "permission.always", "permission.reject":
		reply := strings.TrimPrefix(action.ActionID, "permission.")
		if err := b.replyPermission(action.Value, reply); err != nil {
			log.Printf("replying to OpenCode permission: %v", err)
			return
		}
		_ = b.post(interaction.Channel.ID, interaction.Message.ThreadTS, "Permission response recorded. Continuing.")
	case "question.answer":
		var answer questionAnswerAction
		if json.Unmarshal([]byte(action.Value), &answer) != nil {
			return
		}
		if err := b.recordQuestionAnswer(answer.ID, answer.Index, []string{answer.Answer}, false); err != nil {
			log.Printf("replying to OpenCode question: %v", err)
		}
	case "question.submit":
		if err := b.recordQuestionAnswer(action.Value, -1, nil, true); err != nil {
			_ = b.post(interaction.Channel.ID, interaction.Message.ThreadTS, "Select an answer for every question before submitting.")
		}
	}
}

func (b *bridge) answerQuestionText(channel, thread, text string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, pending := range b.pendingByID {
		if pending.question == nil || pending.conversation != (conversation{channel: channel, thread: thread}) {
			continue
		}
		for index := range pending.question.Questions {
			if len(pending.answers[index]) == 0 {
				pending.answers[index] = []string{strings.TrimSpace(text)}
				b.pendingByID[id] = pending
				if len(pending.question.Questions) == 1 {
					go b.submitQuestion(id)
				}
				return true
			}
		}
	}
	return false
}

func (b *bridge) replyPermission(id, reply string) error {
	b.mu.Lock()
	pending, ok := b.pendingByID[id]
	b.mu.Unlock()
	if !ok || pending.question != nil {
		return errors.New("unknown permission request")
	}
	body, _ := json.Marshal(map[string]string{"reply": reply})
	if err := b.openCode(http.MethodPost, "/permission/"+url.PathEscape(id)+"/reply", body, new(bool)); err != nil {
		return err
	}
	b.mu.Lock()
	delete(b.pendingByID, id)
	b.mu.Unlock()
	return nil
}

func (b *bridge) recordQuestionAnswer(id string, index int, answer []string, submit bool) error {
	b.mu.Lock()
	pending, ok := b.pendingByID[id]
	if !ok || pending.question == nil {
		b.mu.Unlock()
		return errors.New("unknown question request")
	}
	if index >= 0 {
		if index >= len(pending.question.Questions) {
			b.mu.Unlock()
			return errors.New("invalid question index")
		}
		if pending.question.Questions[index].Multiple {
			pending.answers[index] = appendUnique(pending.answers[index], answer...)
		} else {
			pending.answers[index] = answer
		}
		b.pendingByID[id] = pending
	}
	complete := true
	for index := range pending.question.Questions {
		if len(pending.answers[index]) == 0 {
			complete = false
			break
		}
	}
	b.mu.Unlock()
	if !complete {
		return nil
	}
	if submit || !questionNeedsSubmit(*pending.question) {
		return b.submitQuestion(id)
	}
	return nil
}

func (b *bridge) submitQuestion(id string) error {
	b.mu.Lock()
	pending, ok := b.pendingByID[id]
	b.mu.Unlock()
	if !ok || pending.question == nil {
		return errors.New("unknown question request")
	}
	answers := make([][]string, len(pending.question.Questions))
	for index := range answers {
		if len(pending.answers[index]) == 0 {
			return errors.New("incomplete question response")
		}
		answers[index] = pending.answers[index]
	}
	body, _ := json.Marshal(map[string]any{"answers": answers})
	if err := b.openCode(http.MethodPost, "/question/"+url.PathEscape(id)+"/reply", body, new(bool)); err != nil {
		return err
	}
	b.mu.Lock()
	delete(b.pendingByID, id)
	b.mu.Unlock()
	return nil
}

func questionNeedsSubmit(request questionRequest) bool {
	if len(request.Questions) > 1 {
		return true
	}
	return len(request.Questions) == 1 && request.Questions[0].Multiple
}

func appendUnique(values []string, additions ...string) []string {
	for _, addition := range additions {
		found := false
		for _, value := range values {
			if value == addition {
				found = true
				break
			}
		}
		if !found {
			values = append(values, addition)
		}
	}
	return values
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
	return b.postBlocks(channel, thread, text, nil)
}

func (b *bridge) postBlocks(channel, thread, text string, blocks []map[string]any) error {
	bodyData := map[string]any{"channel": channel, "text": text}
	if thread != "" {
		bodyData["thread_ts"] = thread
	}
	if len(blocks) > 0 {
		bodyData["blocks"] = blocks
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
	return parseIDs(value)
}

func parseIDs(value string) map[string]struct{} {
	ids := make(map[string]struct{})
	for _, id := range strings.Split(value, ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids[id] = struct{}{}
		}
	}
	return ids
}

func formatIDs(ids map[string]struct{}) string {
	if len(ids) == 0 {
		return "all"
	}
	values := make([]string, 0, len(ids))
	for id := range ids {
		values = append(values, id)
	}
	sort.Strings(values)
	return strings.Join(values, ",")
}

func (b *bridge) isAllowedUser(user string) bool {
	_, allowed := b.allowedUsers[user]
	return allowed
}
