package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-server/internal/app/chat/port"
	domain "go-server/internal/domain/ai"
)

func TestChatOpenAIStreamRequestAndCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gateway/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("incorrect endpoint or auth")
		}
		var body struct {
			Model    string                           `json:"model"`
			Stream   bool                             `json:"stream"`
			Messages []struct{ Role, Content string } `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if !body.Stream || body.Model != "chat-test" || len(body.Messages) != 2 || body.Messages[1].Content != "负责人是谁" {
			t.Errorf("unexpected model request: %#v", body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"林\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"沐\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	model := domain.Model{Kind: "chat", Provider: "openai-compatible", Endpoint: server.URL + "/gateway/v1", ModelName: "chat-test", APIKey: "test-key"}
	var got strings.Builder
	err := NewChat(builtinRegistry(t)).Stream(context.Background(), model, port.ChatRequest{Messages: []port.ChatTurn{{Role: "system", Content: "系统"}, {Role: "user", Content: "负责人是谁"}}, Temperature: .7, MaxTokens: 100}, func(part string) error { got.WriteString(part); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "林沐" {
		t.Fatalf("answer %q", got.String())
	}
}

func TestChatRejectsTruncatedStreamAndUpstreamLeaks(t *testing.T) {
	for _, body := range []string{"data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n", "data: {\"error\":{\"message\":\"secret-test-key\"}}\n\n"} {
		t.Run(body[:10], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			err := NewChat(builtinRegistry(t)).Stream(context.Background(), domain.Model{Kind: "chat", Provider: "openai", Endpoint: server.URL, ModelName: "test"}, port.ChatRequest{}, func(string) error { return nil })
			if err == nil {
				t.Fatal("broken stream reported success")
			}
			if strings.Contains(err.Error(), "secret-test-key") {
				t.Fatal("upstream body leaked")
			}
		})
	}
}

func TestChatOllamaAndCallbackCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("path %s", r.URL.Path)
		}
		fmt.Fprint(w, "{\"message\":{\"content\":\"回答\"},\"done\":false}\n{\"message\":{\"content\":\"结束\"},\"done\":true}\n")
	}))
	defer server.Close()
	stop := errors.New("consumer stopped")
	err := NewChat(builtinRegistry(t)).Stream(context.Background(), domain.Model{Kind: "chat", Provider: "ollama", Endpoint: server.URL, ModelName: "test"}, port.ChatRequest{}, func(string) error { return stop })
	if !errors.Is(err, stop) {
		t.Fatalf("callback failure was lost: %v", err)
	}
}
