package saas_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// 此替身只替换 E2E 范围外的模型服务，页面配置、问答编排和消息持久化均走真实应用。
func registerBrowserModelFixture(mux *http.ServeMux) {
	mux.HandleFunc("GET /__test/model/v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"e2e-chat"},{"id":"e2e-embedding"}]}`)
	})
	mux.HandleFunc("POST /__test/model/v1/embeddings", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
			http.Error(w, "invalid fixture input", 400)
			return
		}
		rows := []map[string]any{}
		for i := range in.Input {
			rows = append(rows, map[string]any{"index": i, "embedding": []float32{0.4, 0.5, 0.6}})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": rows})
	})
	mux.HandleFunc("POST /__test/model/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Model    string                           `json:"model"`
			Stream   bool                             `json:"stream"`
			Messages []struct{ Role, Content string } `json:"messages"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&in); err != nil {
			http.Error(w, "invalid fixture input", 400)
			return
		}
		answer := "已收到你的消息。"
		for _, m := range in.Messages {
			if m.Role == "system" && strings.Contains(m.Content, "林沐") {
				answer = "航星计划的审批人是林沐。[1]"
			}
		}
		if in.Model == "e2e-chat-alt" {
			answer += "\n\n**已使用第二个启用模型。**"
		}
		if !in.Stream {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}}}})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": answer}}}})
		_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})
}
