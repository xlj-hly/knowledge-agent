package saas_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	ad "go-server/internal/app/ai/dto"
	cd "go-server/internal/app/chat/dto"
	kd "go-server/internal/app/knowledge/dto"
)

type chatStreamEvent struct {
	name string
	data json.RawMessage
}

func parseChatEvents(t *testing.T, result apiResult) []chatStreamEvent {
	t.Helper()
	if result.status != 200 {
		t.Fatalf("chat stream HTTP %d: %s", result.status, result.body)
	}
	out := []chatStreamEvent{}
	for _, block := range strings.Split(string(result.body), "\n\n") {
		var event chatStreamEvent
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "event: ") {
				event.name = strings.TrimPrefix(line, "event: ")
			}
			if strings.HasPrefix(line, "data: ") {
				event.data = json.RawMessage(strings.TrimPrefix(line, "data: "))
			}
		}
		if event.name != "" {
			if !json.Valid(event.data) {
				t.Fatalf("invalid SSE JSON: %s", event.data)
			}
			out = append(out, event)
		}
	}
	if len(out) == 0 {
		t.Fatalf("empty chat stream: %s", result.body)
	}
	return out
}
func chatDone(t *testing.T, events []chatStreamEvent) cd.Message {
	t.Helper()
	for _, event := range events {
		if event.name == "error" {
			t.Fatalf("stream error: %s", event.data)
		}
		if event.name == "done" {
			var m cd.Message
			if err := json.Unmarshal(event.data, &m); err != nil {
				t.Fatal(err)
			}
			return m
		}
	}
	t.Fatal("no completed message event")
	return cd.Message{}
}
func createChatModel(t *testing.T, s *integrationSuite, host, token, endpoint string) ad.Model {
	t.Helper()
	saved := s.request("POST", host, "/api/ai/models", token, map[string]any{"name": "集成测试对话模型", "kind": "chat", "provider": "openai-compatible", "endpoint": endpoint + "/v1", "model_name": "fixture-chat", "dimension": 0, "api_key": "chat-integration-private-key"})
	m := decodeSuccess[ad.Model](t, saved)
	if m.APIKey != "chat-integration-private-key" {
		t.Fatal("model configuration did not return the supplied key")
	}
	return m
}

func TestChatKnowledgeConversationLifecycleAndIsolation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer chat-integration-private-key" {
			http.Error(w, "invalid model request", 400)
			return
		}
		var input struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			Stream bool `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		contextFound := false
		for _, m := range input.Messages {
			if m.Role == "system" && strings.Contains(m.Content, "星河计划负责人是林沐") {
				contextFound = true
			}
		}
		if !input.Stream || !contextFound {
			http.Error(w, "retrieval context absent", 422)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"负责人是\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"林沐。[1]\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()
	s := newIntegrationSuite(t)
	s.createTenant(t, "chat.alpha.test")
	s.createTenant(t, "chat.beta.test")
	a := s.login(t, "chat.alpha.test", "tenant.admin", adminPassword)
	b := s.login(t, "chat.beta.test", "tenant.admin", adminPassword)
	decodeSuccess[userView](t, s.request("POST", "chat.alpha.test", "/api/admin/users", a.Token, map[string]any{"username": "chat.member", "password": memberPassword, "nickname": "对话成员", "role": "user"}))
	u := s.login(t, "chat.alpha.test", "chat.member", memberPassword)
	model := createChatModel(t, s, "chat.alpha.test", a.Token, upstream.URL)
	decodeSuccess[ad.Settings](t, s.request("PUT", "chat.alpha.test", "/api/ai/models/"+model.ID+"/activate", a.Token, nil))
	settings := decodeSuccess[ad.Settings](t, s.request("GET", "chat.alpha.test", "/api/ai/settings", a.Token, nil))
	settings.RetrievalMode = "keyword"
	settings.SystemPrompt = "根据企业资料回答，并使用来源编号。"
	decodeSuccess[ad.Settings](t, s.request("PUT", "chat.alpha.test", "/api/ai/settings", a.Token, settingsInput(settings)))
	base := decodeSuccess[kd.Base](t, s.request("POST", "chat.alpha.test", "/api/knowledge/bases", a.Token, map[string]any{"name": "星河项目资料", "indexing_mode": "keyword", "retrieval_mode": "keyword"}))
	document := decodeSuccess[kd.Document](t, s.request("POST", "chat.alpha.test", "/api/knowledge/bases/"+base.ID+"/documents", a.Token, map[string]any{"title": "星河说明", "content": "星河计划负责人是林沐，评审地点为青禾会议室。"}))
	knowledgeProcess(t, s)
	session := decodeSuccess[cd.Session](t, s.request("POST", "chat.alpha.test", "/api/chat/sessions", a.Token, map[string]any{"title": "项目负责人", "knowledge_base_ids": []string{base.ID}}))
	if session.ModelID != "" || session.SystemPrompt != settings.SystemPrompt || len(session.KnowledgeBaseIDs) != 1 {
		t.Fatalf("default snapshot not copied: %+v", session)
	}
	settings.SystemPrompt = "修改后的企业默认提示词"
	decodeSuccess[ad.Settings](t, s.request("PUT", "chat.alpha.test", "/api/ai/settings", a.Token, settingsInput(settings)))
	unchanged := decodeSuccess[cd.Session](t, s.request("GET", "chat.alpha.test", "/api/chat/sessions/"+session.ID, a.Token, nil))
	if unchanged.SystemPrompt == settings.SystemPrompt {
		t.Fatal("changing template modified an existing conversation")
	}
	foreignBase := decodeSuccess[kd.Base](t, s.request("POST", "chat.beta.test", "/api/knowledge/bases", b.Token, map[string]any{"name": "乙企业资料", "indexing_mode": "keyword", "retrieval_mode": "keyword"}))
	assertFailure(t, s.request("PUT", "chat.alpha.test", "/api/chat/sessions/"+session.ID, a.Token, map[string]any{"knowledge_base_ids": []string{foreignBase.ID}}), 404)
	result := s.request("POST", "chat.alpha.test", "/api/chat/sessions/"+session.ID+"/messages", a.Token, map[string]any{"content": "星河计划"})
	events := parseChatEvents(t, result)
	answer := chatDone(t, events)
	if answer.Status != "completed" || answer.Content != "负责人是林沐。[1]" || len(answer.Sources) != 1 || answer.Sources[0].DocumentID != document.ID || answer.Sources[0].DocumentTitle != "星河说明" {
		t.Fatalf("answer or citation mismatch: %+v", answer)
	}
	if bytes.Contains(result.body, []byte("chat-integration-private-key")) {
		t.Fatal("model secret exposed in answer stream")
	}
	history := decodeSuccess[[]cd.Message](t, s.request("GET", "chat.alpha.test", "/api/chat/sessions/"+session.ID+"/messages", a.Token, nil))
	if len(history) != 2 || history[0].Role != "user" || history[1].ID != answer.ID || history[1].Sources[0].Content != answer.Sources[0].Content {
		t.Fatalf("history not persisted: %+v", history)
	}
	assertFailure(t, s.request("GET", "chat.alpha.test", "/api/chat/sessions/"+session.ID, u.Token, nil), 404)
	assertFailure(t, s.request("GET", "chat.beta.test", "/api/chat/sessions/"+session.ID+"/messages", b.Token, nil), 404)
	assertFailure(t, s.request("POST", "chat.alpha.test", "/api/chat/sessions/"+session.ID+"/messages", u.Token, map[string]any{"content": "星河计划"}), 404)
	assertFailure(t, s.request("DELETE", "chat.alpha.test", "/api/chat/sessions/"+session.ID, u.Token, nil), 404)
	memberSession := decodeSuccess[cd.Session](t, s.request("POST", "chat.alpha.test", "/api/chat/sessions", u.Token, map[string]any{}))
	if memberSession.SystemPrompt != settings.SystemPrompt {
		t.Fatal("new session did not use the updated enterprise template")
	}
	owned := decodeSuccess[[]cd.Session](t, s.request("GET", "chat.alpha.test", "/api/chat/sessions", u.Token, nil))
	if len(owned) != 1 || owned[0].ID != memberSession.ID {
		t.Fatal("session list leaked another member's conversation")
	}
	assertFailure(t, s.request("DELETE", "chat.alpha.test", "/api/ai/models/"+model.ID, a.Token, nil), 409)
	decodeSuccess[cd.OK](t, s.request("DELETE", "chat.alpha.test", "/api/chat/sessions/"+session.ID, a.Token, nil))
	assertFailure(t, s.request("GET", "chat.alpha.test", "/api/chat/sessions/"+session.ID+"/messages", a.Token, nil), 404)
}

func TestChatFailureStopConcurrentSendAndRecovery(t *testing.T) {
	var mode atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"已生成部分\"}}]}\n\n")
		w.(http.Flusher).Flush()
		if mode.Load() == 1 {
			close(started)
			select {
			case <-r.Context().Done():
			case <-release:
			}
		}
	}))
	defer upstream.Close()
	defer close(release)
	s := newIntegrationSuite(t)
	s.createTenant(t, "stream.alpha.test")
	a := s.login(t, "stream.alpha.test", "tenant.admin", adminPassword)
	model := createChatModel(t, s, "stream.alpha.test", a.Token, upstream.URL)
	decodeSuccess[ad.Settings](t, s.request("PUT", "stream.alpha.test", "/api/ai/models/"+model.ID+"/activate", a.Token, nil))
	session := decodeSuccess[cd.Session](t, s.request("POST", "stream.alpha.test", "/api/chat/sessions", a.Token, map[string]any{"title": "停止与恢复"}))
	failure := parseChatEvents(t, s.request("POST", "stream.alpha.test", "/api/chat/sessions/"+session.ID+"/messages", a.Token, map[string]any{"content": "第一次提问"}))
	foundError := false
	for _, event := range failure {
		if event.name == "error" {
			foundError = true
		}
		if event.name == "done" {
			t.Fatal("truncated upstream stream was reported as complete")
		}
	}
	if !foundError {
		t.Fatal("truncated upstream did not report a stream error")
	}
	history := decodeSuccess[[]cd.Message](t, s.request("GET", "stream.alpha.test", "/api/chat/sessions/"+session.ID+"/messages", a.Token, nil))
	if len(history) != 2 || history[1].Status != "failed" || history[1].Content != "已生成部分" {
		t.Fatalf("failed partial answer lost: %+v", history)
	}
	mode.Store(1)
	finished := make(chan apiResult, 1)
	joined := false
	t.Cleanup(func() {
		if !joined {
			select {
			case <-finished:
			case <-time.After(20 * time.Second):
				t.Error("chat request did not finish before database cleanup")
			}
		}
	})
	go func() {
		finished <- s.request("POST", "stream.alpha.test", "/api/chat/sessions/"+session.ID+"/messages", a.Token, map[string]any{"content": "第二次提问"})
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("second generation did not reach model")
	}
	assertFailure(t, s.request("POST", "stream.alpha.test", "/api/chat/sessions/"+session.ID+"/messages", a.Token, map[string]any{"content": "不能并发写入"}), 409)
	assertFailure(t, s.request("PUT", "stream.alpha.test", "/api/chat/sessions/"+session.ID, a.Token, map[string]any{"title": "不能生成中修改"}), 409)
	assertFailure(t, s.request("DELETE", "stream.alpha.test", "/api/chat/sessions/"+session.ID, a.Token, nil), 409)
	decodeSuccess[cd.OK](t, s.request("POST", "stream.alpha.test", "/api/chat/sessions/"+session.ID+"/stop", a.Token, nil))
	select {
	case result := <-finished:
		joined = true
		answer := chatDone(t, parseChatEvents(t, result))
		if answer.Status != "cancelled" || answer.Content != "已生成部分" {
			t.Fatalf("cancelled answer: %+v", answer)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not finish the request")
	}
	history = decodeSuccess[[]cd.Message](t, s.request("GET", "stream.alpha.test", "/api/chat/sessions/"+session.ID+"/messages", a.Token, nil))
	if len(history) != 4 || history[3].Status != "cancelled" {
		t.Fatalf("concurrent request wrote extra messages: %+v", history)
	}
	if err := s.db.Exec("UPDATE chat_messages SET status = 'generating' WHERE id = ?", history[3].ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.application.Chat.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	history = decodeSuccess[[]cd.Message](t, s.request("GET", "stream.alpha.test", "/api/chat/sessions/"+session.ID+"/messages", a.Token, nil))
	if history[3].Status != "failed" || history[3].Content != "已生成部分" {
		t.Fatal("restart recovery lost content or left generation running")
	}
	decodeSuccess[cd.OK](t, s.request("DELETE", "stream.alpha.test", "/api/chat/sessions/"+session.ID, a.Token, nil))
	var count int64
	if err := s.db.Table("chat_messages").Where("session_id = ?", session.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("session delete did not remove messages: count=%d error=%v", count, err)
	}
}

func TestEnabledModelAppliesToExistingSessionsAndPreservesInFlightGeneration(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if in.Model == "fixture-chat" {
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": in.Model}}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
	}))
	defer upstream.Close()
	defer close(release)
	s := newIntegrationSuite(t)
	const host = "active.alpha.test"
	s.createTenant(t, host)
	s.createTenant(t, "active.beta.test")
	admin := s.login(t, host, "tenant.admin", adminPassword)
	beta := s.login(t, "active.beta.test", "tenant.admin", adminPassword)
	decodeSuccess[userView](t, s.request("POST", host, "/api/admin/users", admin.Token, map[string]any{"username": "chat.member", "password": memberPassword, "nickname": "普通成员", "role": "user"}))
	member := s.login(t, host, "chat.member", memberPassword)
	first := createChatModel(t, s, host, admin.Token, upstream.URL)
	second := decodeSuccess[ad.Model](t, s.request("POST", host, "/api/ai/models", admin.Token, map[string]any{"name": "第二个 LLM", "kind": "chat", "provider": "openai-compatible", "endpoint": upstream.URL + "/v1", "model_name": "second-chat"}))
	embedding := decodeSuccess[ad.Model](t, s.request("POST", host, "/api/ai/models", admin.Token, map[string]any{"name": "文本向量", "kind": "embedding", "provider": "openai-compatible", "endpoint": upstream.URL + "/v1", "model_name": "embedding", "dimension": 2}))
	stale := decodeSuccess[ad.Settings](t, s.request("GET", host, "/api/ai/settings", admin.Token, nil))
	session := decodeSuccess[cd.Session](t, s.request("POST", host, "/api/chat/sessions", admin.Token, map[string]any{}))
	path := "/api/chat/sessions/" + session.ID
	assertFailure(t, s.request("POST", host, path+"/messages", admin.Token, map[string]any{"content": "未启用模型"}), 400)
	if rows := decodeSuccess[[]cd.Message](t, s.request("GET", host, path+"/messages", admin.Token, nil)); len(rows) != 0 {
		t.Fatal("missing active model created partial messages")
	}
	assertFailure(t, s.request("PUT", host, path, admin.Token, map[string]any{"model_id": first.ID}), 400)
	assertFailure(t, s.request("PUT", host, "/api/ai/settings", admin.Token, stale), 400)
	activate := func(model, token, targetHost string) apiResult {
		return s.request("PUT", targetHost, "/api/ai/models/"+model+"/activate", token, nil)
	}
	if active := decodeSuccess[ad.Settings](t, activate(first.ID, admin.Token, host)); active.ChatModelID != first.ID {
		t.Fatal("first LLM was not activated")
	}
	finished := make(chan apiResult, 1)
	joined := false
	t.Cleanup(func() {
		if !joined {
			select {
			case <-finished:
			case <-time.After(20 * time.Second):
				t.Error("generation did not exit before test database cleanup")
			}
		}
	})
	go func() {
		finished <- s.request("POST", host, path+"/messages", admin.Token, map[string]any{"content": "第一条消息"})
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first LLM was not called")
	}
	assertFailure(t, s.request("POST", host, path+"/messages", member.Token, map[string]any{"content": "其他成员不能读取生成状态"}), 404)
	assertFailure(t, s.request("POST", "active.beta.test", path+"/messages", beta.Token, map[string]any{"content": "其他企业不能占用生成资格"}), 404)
	active := decodeSuccess[ad.Settings](t, activate(second.ID, admin.Token, host))
	if active.ChatModelID != second.ID {
		t.Fatal("second LLM was not activated")
	}
	stale.SystemPrompt = "修改模板时保留当前启用模型"
	saved := decodeSuccess[ad.Settings](t, s.request("PUT", host, "/api/ai/settings", admin.Token, settingsInput(stale)))
	if saved.ChatModelID != second.ID || saved.SystemPrompt != stale.SystemPrompt {
		t.Fatal("saving a stale template replaced the enabled model or lost the template")
	}
	// 历史模型快照不是配置引用，旧模型可删除；已捕获的请求仍完成原来的回答。
	decodeSuccess[ad.OK](t, s.request("DELETE", host, "/api/ai/models/"+first.ID, admin.Token, nil))
	release <- struct{}{}
	select {
	case result := <-finished:
		joined = true
		if answer := chatDone(t, parseChatEvents(t, result)); answer.Content != "fixture-chat" {
			t.Fatalf("in-flight generation changed model: %+v", answer)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight generation did not finish")
	}
	assertFailure(t, activate(second.ID, "", host), 401)
	assertFailure(t, activate(second.ID, member.Token, host), 403)
	assertFailure(t, activate(second.ID, beta.Token, "active.beta.test"), 404)
	assertFailure(t, activate(embedding.ID, admin.Token, host), 400)
	assertFailure(t, activate("not-an-id", admin.Token, host), 400)
	assertFailure(t, activate(first.ID, admin.Token, host), 404)
	if current := decodeSuccess[ad.Settings](t, s.request("GET", host, "/api/ai/settings", admin.Token, nil)); current.ChatModelID != second.ID {
		t.Fatal("rejected activation changed the enabled LLM")
	}
	if foreign := decodeSuccess[ad.Settings](t, s.request("GET", "active.beta.test", "/api/ai/settings", beta.Token, nil)); foreign.ChatModelID != "" {
		t.Fatal("activation crossed the tenant boundary")
	}
	if answer := chatDone(t, parseChatEvents(t, s.request("POST", host, path+"/messages", admin.Token, map[string]any{"content": "已有对话下一条"}))); answer.Content != "second-chat" {
		t.Fatalf("existing conversation retained its old model: %+v", answer)
	}
	snapshot := decodeSuccess[cd.Session](t, s.request("GET", host, path, admin.Token, nil))
	if snapshot.ModelID != second.ID || snapshot.SystemPrompt == stale.SystemPrompt {
		t.Fatal("last generation snapshot or per-conversation prompt ownership is incorrect")
	}
	history := decodeSuccess[[]cd.Message](t, s.request("GET", host, path+"/messages", admin.Token, nil))
	if len(history) != 4 || history[1].Content != "fixture-chat" || history[3].Content != "second-chat" {
		t.Fatal("switching LLM rewrote history or lost the next answer")
	}
	newSession := decodeSuccess[cd.Session](t, s.request("POST", host, "/api/chat/sessions", admin.Token, map[string]any{}))
	if answer := chatDone(t, parseChatEvents(t, s.request("POST", host, "/api/chat/sessions/"+newSession.ID+"/messages", admin.Token, map[string]any{"content": "新对话"}))); answer.Content != "second-chat" {
		t.Fatalf("new conversation did not use enabled LLM: %+v", answer)
	}
}
