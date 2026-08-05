package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestMentionRemoval(t *testing.T) {
	got := mention.ReplaceAllString("<@U123> help me with this", "")
	if got != "help me with this" {
		t.Fatalf("mention removal = %q, want %q", got, "help me with this")
	}
}

func TestParseAllowedUsers(t *testing.T) {
	users := parseAllowedUsers(" U123, U456, U123 ")
	if !(&bridge{allowedUsers: users}).isAllowedUser("U123") {
		t.Fatal("U123 should be allowed")
	}
	if (&bridge{allowedUsers: users}).isAllowedUser("U789") {
		t.Fatal("U789 should not be allowed")
	}
	if len(parseAllowedUsers("")) != 0 {
		t.Fatal("empty user list should deny all users")
	}
}

func TestShouldHandleThreadRepliesOnlyForAuthorizedUsersAndStartedThreads(t *testing.T) {
	b := &bridge{
		cfg:          config{slackChannelIDs: parseIDs("C123,C456")},
		mapByThread:  map[string]string{"C123:123.456": "ses_test"},
		allowedUsers: parseAllowedUsers("U123"),
	}

	if !b.shouldHandle(slackEvent{Type: "app_mention", Channel: "C123", User: "U123", TS: "123.456"}) {
		t.Fatal("authorized mention should start a thread")
	}
	if !b.shouldHandle(slackEvent{Type: "app_mention", Channel: "C456", User: "U123", TS: "123.456"}) {
		t.Fatal("authorized mention in a second configured channel should start a thread")
	}
	if b.shouldHandle(slackEvent{Type: "app_mention", Channel: "C789", User: "U123", TS: "123.456"}) {
		t.Fatal("mention in an unconfigured channel should be ignored")
	}
	if !b.shouldHandle(slackEvent{Type: "message", Channel: "C123", User: "U123", TS: "123.457", ThreadTS: "123.456"}) {
		t.Fatal("authorized reply in a started thread should be handled without a mention")
	}
	if b.shouldHandle(slackEvent{Type: "message", Channel: "C123", User: "U456", TS: "123.457", ThreadTS: "123.456"}) {
		t.Fatal("unauthorized reply should not be handled")
	}
	if b.shouldHandle(slackEvent{Type: "message", Channel: "C123", User: "U123", TS: "456.789", ThreadTS: "456.000"}) {
		t.Fatal("reply in an unstarted thread should not be handled")
	}
	if b.shouldHandle(slackEvent{Type: "message", Channel: "C123", User: "U123", TS: "456.789"}) {
		t.Fatal("top-level message without a mention should not be handled")
	}
	if b.shouldHandle(slackEvent{Type: "message", Channel: "C123", User: "U123", BotID: "B123", TS: "123.457", ThreadTS: "123.456"}) {
		t.Fatal("bot reply should not be handled")
	}
}

func TestShouldHandleDirectMessagesOnlyWhenEnabledAndAuthorized(t *testing.T) {
	event := slackEvent{Type: "message", Channel: "D123", ChannelType: "im", User: "U123", TS: "123.456"}
	disabled := &bridge{cfg: config{slackAllowDMs: false}, allowedUsers: parseAllowedUsers("U123")}
	if disabled.shouldHandle(event) {
		t.Fatal("direct messages should be ignored when disabled")
	}

	enabled := &bridge{cfg: config{slackAllowDMs: true}, allowedUsers: parseAllowedUsers("U123")}
	if !enabled.shouldHandle(event) {
		t.Fatal("authorized direct message should be handled when enabled")
	}
	if enabled.shouldHandle(slackEvent{Type: "message", Channel: "D123", ChannelType: "im", User: "U456", TS: "123.456"}) {
		t.Fatal("unauthorized direct message should be ignored")
	}
}

func TestDuration(t *testing.T) {
	t.Setenv("TEST_TIMEOUT", "1m")
	if got := duration("TEST_TIMEOUT", 30*time.Second); got != time.Minute {
		t.Fatalf("duration = %s, want %s", got, time.Minute)
	}
}

func TestSessionReusesThreadSession(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/session" || request.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		calls++
		_ = json.NewEncoder(response).Encode(sessionResult{ID: "ses_test"})
	}))
	defer server.Close()

	b := &bridge{
		cfg:         config{opencodeURL: server.URL},
		hc:          server.Client(),
		mapByThread: make(map[string]string),
	}

	first, err := b.session("C123", "123.456")
	if err != nil {
		t.Fatalf("first session: %v", err)
	}
	second, err := b.session("C123", "123.456")
	if err != nil {
		t.Fatalf("second session: %v", err)
	}
	if first != "ses_test" || second != first {
		t.Fatalf("sessions = %q, %q", first, second)
	}
	if calls != 1 {
		t.Fatalf("session API calls = %d, want 1", calls)
	}
}

func TestPromptCollectsTextParts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/session/ses_test/message" || request.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		_, _ = response.Write([]byte(`{"parts":[{"type":"text","text":"hello "},{"type":"tool","text":"ignored"},{"type":"text","text":"world"}]}`))
	}))
	defer server.Close()

	b := &bridge{cfg: config{opencodeURL: server.URL}, hc: server.Client()}
	got, err := b.prompt("ses_test", "hello")
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if got != "hello world" {
		t.Fatalf("prompt response = %q, want %q", got, "hello world")
	}
}

func TestReactAcknowledgesMessage(t *testing.T) {
	var gotMethod, gotPath, gotAuthorization string
	var gotBody map[string]string
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		gotMethod = request.Method
		gotPath = request.URL.Path
		gotAuthorization = request.Header.Get("Authorization")
		if err := json.NewDecoder(request.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{"ok":true}`)),
			Header:     make(http.Header),
		}, nil
	})}

	b := &bridge{cfg: config{slackBotToken: "token"}, hc: client}
	if err := b.react("C123", "123.456", "eyes"); err != nil {
		t.Fatalf("react: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/reactions.add" {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
	if gotAuthorization != "Bearer token" {
		t.Fatalf("authorization = %q", gotAuthorization)
	}
	if gotBody["channel"] != "C123" || gotBody["timestamp"] != "123.456" || gotBody["name"] != "eyes" {
		t.Fatalf("body = %#v", gotBody)
	}
}

func TestUnreactRemovesAcknowledgement(t *testing.T) {
	var gotPath string
	var gotBody map[string]string
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		gotPath = request.URL.Path
		if err := json.NewDecoder(request.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{"ok":true}`)),
			Header:     make(http.Header),
		}, nil
	})}

	b := &bridge{cfg: config{slackBotToken: "token"}, hc: client}
	if err := b.unreact("C123", "123.456", "eyes"); err != nil {
		t.Fatalf("unreact: %v", err)
	}
	if gotPath != "/api/reactions.remove" {
		t.Fatalf("request path = %s", gotPath)
	}
	if gotBody["channel"] != "C123" || gotBody["timestamp"] != "123.456" || gotBody["name"] != "eyes" {
		t.Fatalf("body = %#v", gotBody)
	}
}

func TestHandleMarksMentionCompleteAfterPostingResponse(t *testing.T) {
	var actions []string
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		var body string
		switch request.URL.Path {
		case "/api/reactions.add", "/api/reactions.remove":
			var reaction struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(request.Body).Decode(&reaction); err != nil {
				t.Fatalf("decode reaction: %v", err)
			}
			action := "add:"
			if request.URL.Path == "/api/reactions.remove" {
				action = "remove:"
			}
			actions = append(actions, action+reaction.Name)
			body = `{"ok":true}`
		case "/api/chat.postMessage":
			actions = append(actions, "post")
			body = `{"ok":true}`
		case "/session":
			body = `{"id":"ses_test"}`
		case "/session/ses_test/message":
			body = `{"parts":[{"type":"text","text":"response"}]}`
		default:
			t.Fatalf("unexpected request path: %s", request.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     make(http.Header),
		}, nil
	})}
	b := &bridge{
		cfg:         config{slackBotToken: "token", opencodeURL: "http://opencode"},
		hc:          client,
		mapByThread: make(map[string]string),
	}

	b.handle("C123", "channel", "123.456", "", "<@U123> help")

	want := []string{"add:eyes", "post", "remove:eyes", "add:white_check_mark"}
	if !reflect.DeepEqual(actions, want) {
		t.Fatalf("actions = %#v, want %#v", actions, want)
	}
}

func TestHandleDirectMessagesReuseSessionAndPostWithoutThread(t *testing.T) {
	sessionCalls := 0
	var posts []map[string]string
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		var body string
		switch request.URL.Path {
		case "/api/reactions.add", "/api/reactions.remove":
			body = `{"ok":true}`
		case "/api/chat.postMessage":
			var post map[string]string
			if err := json.NewDecoder(request.Body).Decode(&post); err != nil {
				t.Fatalf("decode post: %v", err)
			}
			posts = append(posts, post)
			body = `{"ok":true}`
		case "/session":
			sessionCalls++
			body = `{"id":"ses_test"}`
		case "/session/ses_test/message":
			body = `{"parts":[{"type":"text","text":"response"}]}`
		default:
			t.Fatalf("unexpected request path: %s", request.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(body)), Header: make(http.Header)}, nil
	})}
	b := &bridge{
		cfg:         config{slackBotToken: "token", opencodeURL: "http://opencode"},
		hc:          client,
		mapByThread: make(map[string]string),
	}

	b.handle("D123", "im", "123.456", "", "first")
	b.handle("D123", "im", "123.457", "", "second")

	if sessionCalls != 1 {
		t.Fatalf("session API calls = %d, want 1", sessionCalls)
	}
	if len(posts) != 2 {
		t.Fatalf("posts = %d, want 2", len(posts))
	}
	for _, post := range posts {
		if post["channel"] != "D123" {
			t.Fatalf("post channel = %q, want D123", post["channel"])
		}
		if _, threaded := post["thread_ts"]; threaded {
			t.Fatalf("direct message post should not have thread_ts: %#v", post)
		}
	}
}

func TestPermissionEventPostsActionsAndInteractiveResponseResumesOpenCode(t *testing.T) {
	var posted map[string]any
	var permissionReply map[string]string
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/api/chat.postMessage":
			if err := json.NewDecoder(request.Body).Decode(&posted); err != nil {
				t.Fatalf("decode Slack post: %v", err)
			}
		case "/permission/per_test/reply":
			if err := json.NewDecoder(request.Body).Decode(&permissionReply); err != nil {
				t.Fatalf("decode permission reply: %v", err)
			}
		default:
			t.Fatalf("unexpected request: %s", request.URL.Path)
		}
		body := `true`
		if request.URL.Path == "/api/chat.postMessage" {
			body = `{"ok":true}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(body)), Header: make(http.Header)}, nil
	})}
	b := &bridge{
		cfg:             config{slackBotToken: "token", opencodeURL: "http://opencode"},
		hc:              client,
		threadBySession: map[string]conversation{"ses_test": {channel: "C123", thread: "123.456"}},
		pendingByID:     make(map[string]pendingInteraction),
		allowedUsers:    parseAllowedUsers("U123"),
	}
	properties, _ := json.Marshal(permissionRequest{ID: "per_test", SessionID: "ses_test", Permission: "bash", Patterns: []string{"git status"}})
	b.handleOpenCodeEvent(openCodeEvent{Type: "permission.asked", Properties: properties})

	if posted["thread_ts"] != "123.456" {
		t.Fatalf("permission thread = %#v", posted["thread_ts"])
	}
	blocks, ok := posted["blocks"].([]any)
	if !ok || len(blocks) != 1 {
		t.Fatalf("permission blocks = %#v", posted["blocks"])
	}
	interaction := `{"type":"block_actions","user":{"id":"U123"},"channel":{"id":"C123"},"message":{"thread_ts":"123.456"},"actions":[{"action_id":"permission.once","value":"per_test"}]}`
	encoded, _ := json.Marshal(interaction)
	b.handleInteraction(encoded)
	if permissionReply["reply"] != "once" {
		t.Fatalf("permission reply = %#v", permissionReply)
	}
	if _, pending := b.pendingByID["per_test"]; pending {
		t.Fatal("permission should be removed after a successful response")
	}
}

func TestQuestionAnswerResumesOpenCode(t *testing.T) {
	var questionReply struct {
		Answers [][]string `json:"answers"`
	}
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/question/que_test/reply" {
			t.Fatalf("unexpected request: %s", request.URL.Path)
		}
		if err := json.NewDecoder(request.Body).Decode(&questionReply); err != nil {
			t.Fatalf("decode question reply: %v", err)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(`true`)), Header: make(http.Header)}, nil
	})}
	request := questionRequest{ID: "que_test", SessionID: "ses_test", Questions: []questionInfo{{Question: "Proceed?", Options: []questionOption{{Label: "Yes"}, {Label: "No"}}}}}
	b := &bridge{
		cfg:         config{opencodeURL: "http://opencode"},
		hc:          client,
		pendingByID: map[string]pendingInteraction{"que_test": {sessionID: "ses_test", question: &request, answers: make(map[int][]string)}},
	}
	if err := b.recordQuestionAnswer("que_test", 0, []string{"Yes"}, false); err != nil {
		t.Fatalf("record question answer: %v", err)
	}
	if !reflect.DeepEqual(questionReply.Answers, [][]string{{"Yes"}}) {
		t.Fatalf("answers = %#v", questionReply.Answers)
	}
	if _, pending := b.pendingByID["que_test"]; pending {
		t.Fatal("question should be removed after a successful response")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
