package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
