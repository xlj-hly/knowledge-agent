package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go-server/internal/app/chat/dto"
	"go-server/internal/app/chat/port"
	kdto "go-server/internal/app/knowledge/dto"
	"go-server/internal/domain/ai"
	"go-server/internal/domain/chat"
	domain "go-server/internal/domain/saas"
)

type testRepository struct {
	sync.Mutex
	session  chat.Session
	messages []chat.Message
	progress int
}

func (r *testRepository) Sessions(context.Context, string, string) ([]chat.Session, error) {
	return []chat.Session{r.session}, nil
}
func (r *testRepository) Session(_ context.Context, t, u, id string) (chat.Session, error) {
	r.Lock()
	defer r.Unlock()
	if t != r.session.TenantID || u != r.session.UserID || id != r.session.ID {
		return chat.Session{}, domain.NotFound()
	}
	return r.session, nil
}
func (r *testRepository) SaveSession(_ context.Context, s chat.Session, _ bool) error {
	r.Lock()
	defer r.Unlock()
	r.session = s
	return nil
}
func (r *testRepository) DeleteSession(context.Context, string, string, string) error { return nil }
func (r *testRepository) Messages(context.Context, string, string, string) ([]chat.Message, error) {
	r.Lock()
	defer r.Unlock()
	return append([]chat.Message{}, r.messages...), nil
}
func (r *testRepository) Begin(_ context.Context, t, u, id, modelID string, user, assistant chat.Message) (chat.Session, []chat.Message, error) {
	r.Lock()
	defer r.Unlock()
	for _, m := range r.messages {
		if m.Status == "generating" {
			return chat.Session{}, nil, domain.Conflict("busy")
		}
	}
	history := append([]chat.Message{}, r.messages...)
	r.session.ModelID = modelID
	r.messages = append(r.messages, user, assistant)
	return r.session, history, nil
}
func (r *testRepository) Progress(_ context.Context, m chat.Message) error {
	r.Lock()
	defer r.Unlock()
	r.progress++
	for i := range r.messages {
		if r.messages[i].ID == m.ID {
			if r.messages[i].Status != "generating" {
				return domain.Conflict("stopped")
			}
			r.messages[i] = m
			return nil
		}
	}
	return domain.NotFound()
}
func (r *testRepository) Finish(_ context.Context, m chat.Message) (chat.Message, error) {
	r.Lock()
	defer r.Unlock()
	for i := range r.messages {
		if r.messages[i].ID == m.ID {
			if r.messages[i].Status == "cancelled" {
				m.Status = "cancelled"
			}
			r.messages[i] = m
			return m, nil
		}
	}
	return chat.Message{}, domain.NotFound()
}
func (r *testRepository) Cancel(context.Context, string, string, string) (bool, error) {
	r.Lock()
	defer r.Unlock()
	found := false
	for i := range r.messages {
		if r.messages[i].Status == "generating" {
			r.messages[i].Status = "cancelled"
			found = true
		}
	}
	return found, nil
}
func (r *testRepository) Recover(context.Context, time.Time) error { return nil }

type testAuthorizer struct{}

func (testAuthorizer) Authorize(_ context.Context, _, token string) (port.Actor, error) {
	if token == "other" {
		return port.Actor{TenantID: "1", UserID: "99"}, nil
	}
	return port.Actor{TenantID: "1", UserID: "2", Role: "admin"}, nil
}

type testModels struct{}

func (testModels) ResolveModel(context.Context, string, string, string) (ai.Model, error) {
	return ai.Model{ID: "3", Kind: "chat", Provider: "openai", ModelName: "test"}, nil
}
func (testModels) Defaults(context.Context, string) (ai.Settings, error) {
	s := ai.DefaultSettings()
	s.ChatModelID = "3"
	return s, nil
}

type testKnowledge struct{ hits []kdto.Hit }

func (testKnowledge) Base(context.Context, kdto.Identity, string) (kdto.Base, error) {
	return kdto.Base{ID: "4"}, nil
}
func (k testKnowledge) Search(context.Context, kdto.Identity, string, kdto.SearchInput) ([]kdto.Hit, error) {
	return k.hits, nil
}

type testModel struct {
	fail    error
	request port.ChatRequest
	model   ai.Model
	empty   bool
}

func (m *testModel) Stream(_ context.Context, model ai.Model, in port.ChatRequest, emit func(string) error) error {
	m.request = in
	m.model = model
	if m.empty {
		return nil
	}
	if err := emit("林沐"); err != nil {
		return err
	}
	return m.fail
}

type mutableModels struct {
	current string
	models  map[string]ai.Model
}

func (m *mutableModels) ResolveModel(_ context.Context, _, id, _ string) (ai.Model, error) {
	model, ok := m.models[id]
	if !ok {
		return ai.Model{}, domain.NotFound()
	}
	return model, nil
}
func (m *mutableModels) Defaults(context.Context, string) (ai.Settings, error) {
	settings := ai.DefaultSettings()
	settings.ChatModelID = m.current
	return settings, nil
}

type changingKnowledge struct {
	testKnowledge
	change func()
}

func (k changingKnowledge) Search(ctx context.Context, id kdto.Identity, base string, in kdto.SearchInput) ([]kdto.Hit, error) {
	k.change()
	return k.testKnowledge.Search(ctx, id, base, in)
}

func TestModelConfigurationIsCapturedBeforeRetrieval(t *testing.T) {
	s, _, output := setupChat()
	models := &mutableModels{current: "3", models: map[string]ai.Model{
		"3": {ID: "3", Kind: "chat", APIKey: "first-key"},
		"9": {ID: "9", Kind: "chat", APIKey: "second-key"},
	}}
	s.deps.Models = models
	s.deps.Knowledge = changingKnowledge{change: func() {
		// 检索期间发生启用切换及旧模型删除，本轮仍须使用锁内捕获的完整配置。
		models.current = "9"
		delete(models.models, "3")
	}}
	if err := s.Send(context.Background(), dto.Identity{}, "8", dto.MessageInput{Content: "第一问"}, func(dto.Event) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if output.model.ID != "3" || output.model.APIKey != "first-key" {
		t.Fatal("model configuration changed after retrieval began")
	}
	if err := s.Send(context.Background(), dto.Identity{}, "8", dto.MessageInput{Content: "下一问"}, func(dto.Event) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if output.model.ID != "9" || output.model.APIKey != "second-key" {
		t.Fatal("next message did not use the newly enabled model")
	}
}
func setupChat() (*Service, *testRepository, *testModel) {
	r := &testRepository{session: chat.Session{ID: "8", TenantID: "1", UserID: "2", Title: "问题", ModelID: "3", KnowledgeBaseIDs: []string{"4"}, SystemPrompt: "按资料回答", Temperature: .7, MaxTokens: 100, HistoryTurns: 10, RetrievalMode: "keyword", RetrievalLimit: 6}}
	m := &testModel{}
	s := New(Dependencies{Repository: r, Authorizer: testAuthorizer{}, Models: testModels{}, Knowledge: testKnowledge{hits: []kdto.Hit{{Chunk: kdto.Chunk{ID: "5", DocumentID: "6", Content: "星河负责人林沐"}, DocumentTitle: "项目说明", Score: 1}}}, Model: m})
	return s, r, m
}

func TestAnswerPersistsSourcesAndStreamsDeltas(t *testing.T) {
	s, r, m := setupChat()
	events := []dto.Event{}
	err := s.Send(context.Background(), dto.Identity{}, "8", dto.MessageInput{Content: "星河负责人是谁"}, func(e dto.Event) error { events = append(events, e); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(r.messages) != 2 || r.messages[1].Content != "林沐" || r.messages[1].Status != "completed" {
		t.Fatalf("messages: %#v", r.messages)
	}
	if len(r.messages[1].Sources) != 1 || r.messages[1].Sources[0].DocumentTitle != "项目说明" {
		t.Fatal("source snapshot missing")
	}
	if len(events) < 4 || events[0].Type != "message" || events[len(events)-1].Type != "done" {
		t.Fatalf("events: %#v", events)
	}
	if len(m.request.Messages) < 2 || m.request.Messages[0].Role != "system" {
		t.Fatal("prompt and context not assembled")
	}
}

func TestModelFailurePreservesPartialAnswer(t *testing.T) {
	s, r, m := setupChat()
	m.fail = errors.New("secret upstream response")
	err := s.Send(context.Background(), dto.Identity{}, "8", dto.MessageInput{Content: "星河负责人是谁"}, func(dto.Event) error { return nil })
	if err == nil {
		t.Fatal("model failure lost")
	}
	if r.messages[1].Status != "failed" || r.messages[1].Content != "林沐" {
		t.Fatal("partial generation not preserved")
	}
	if err.Error() == m.fail.Error() {
		t.Fatal("raw upstream failure leaked")
	}
}

func TestOtherUserCannotSendToSession(t *testing.T) {
	s, r, _ := setupChat()
	err := s.Send(context.Background(), dto.Identity{Token: "other"}, "8", dto.MessageInput{Content: "问题"}, func(dto.Event) error { return nil })
	var business *domain.Error
	if !errors.As(err, &business) || business.Code != "not_found" {
		t.Fatalf("unauthorized scope: %v", err)
	}
	if len(r.messages) != 0 {
		t.Fatal("unauthorized write")
	}
}

func TestSelectedKnowledgeWithoutHitsIsExplicit(t *testing.T) {
	s, r, _ := setupChat()
	s.deps.Knowledge = testKnowledge{}
	if err := s.Send(context.Background(), dto.Identity{}, "8", dto.MessageInput{Content: "问题"}, func(dto.Event) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if r.messages[1].Content != "未在所选知识库中检索到相关资料。\n\n林沐" {
		t.Fatalf("missing no-match notice: %q", r.messages[1].Content)
	}
}

func TestNoHitNoticeDoesNotHideEmptyModelResponse(t *testing.T) {
	s, r, m := setupChat()
	s.deps.Knowledge = testKnowledge{}
	m.empty = true
	err := s.Send(context.Background(), dto.Identity{}, "8", dto.MessageInput{Content: "问题"}, func(dto.Event) error { return nil })
	if err == nil || r.messages[1].Status != "failed" {
		t.Fatalf("empty model response reported success: error=%v message=%#v", err, r.messages[1])
	}
}

func TestKeywordAndHybridScoresAreNotVectorSimilarity(t *testing.T) {
	for _, mode := range []string{"keyword", "hybrid"} {
		t.Run(mode, func(t *testing.T) {
			s, r, _ := setupChat()
			r.session.RetrievalMode = mode
			r.session.SimilarityThreshold = .8
			s.deps.Knowledge = testKnowledge{hits: []kdto.Hit{{Chunk: kdto.Chunk{ID: "5", DocumentID: "6", Content: "负责人林沐"}, Score: .016}}}
			if err := s.Send(context.Background(), dto.Identity{}, "8", dto.MessageInput{Content: "项目"}, func(dto.Event) error { return nil }); err != nil {
				t.Fatal(err)
			}
			if len(r.messages[1].Sources) != 1 {
				t.Fatal("non-vector ranking score was incorrectly filtered")
			}
		})
	}
}

type waitingModel struct{ started chan struct{} }

func (m waitingModel) Stream(ctx context.Context, _ ai.Model, _ port.ChatRequest, emit func(string) error) error {
	if err := emit("已生成部分"); err != nil {
		return err
	}
	close(m.started)
	<-ctx.Done()
	return ctx.Err()
}

func TestStopCancelsGenerationAndPreservesPartialText(t *testing.T) {
	s, r, _ := setupChat()
	started := make(chan struct{})
	s.deps.Model = waitingModel{started: started}
	finished := make(chan error, 1)
	go func() {
		finished <- s.Send(context.Background(), dto.Identity{}, "8", dto.MessageInput{Content: "负责人是谁"}, func(dto.Event) error { return nil })
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("generation did not start")
	}
	if _, err := s.Stop(context.Background(), dto.Identity{}, "8"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel upstream")
	}
	if r.messages[1].Status != "cancelled" || r.messages[1].Content != "已生成部分" {
		t.Fatalf("cancelled message: %#v", r.messages[1])
	}
}

func TestConcurrentSendDoesNotCreateSecondUserMessage(t *testing.T) {
	s, r, _ := setupChat()
	started := make(chan struct{})
	s.deps.Model = waitingModel{started: started}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		finished <- s.Send(ctx, dto.Identity{}, "8", dto.MessageInput{Content: "第一次提问"}, func(dto.Event) error { return nil })
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("generation did not start")
	}
	err := s.Send(context.Background(), dto.Identity{}, "8", dto.MessageInput{Content: "第二次提问"}, func(dto.Event) error { return nil })
	var business *domain.Error
	if !errors.As(err, &business) || business.Code != "conflict" {
		t.Fatalf("concurrent send: %v", err)
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("cancel did not finish")
	}
	if len(r.messages) != 2 {
		t.Fatalf("concurrent send added messages: %d", len(r.messages))
	}
}

func TestHistorySkipsFailedPairsAndObeysTurnLimit(t *testing.T) {
	history := []chat.Message{{Role: "user", Content: "old", Status: "completed"}, {Role: "assistant", Content: "old-answer", Status: "completed"}, {Role: "user", Content: "bad", Status: "completed"}, {Role: "assistant", Content: "partial", Status: "failed"}, {Role: "user", Content: "recent", Status: "completed"}, {Role: "assistant", Content: "recent-answer", Status: "completed"}}
	turns := contextTurns(chat.Session{SystemPrompt: "system", HistoryTurns: 1}, history, "current", nil)
	if len(turns) != 4 || turns[1].Content != "recent" || turns[2].Content != "recent-answer" || turns[3].Content != "current" {
		t.Fatalf("incorrect history: %#v", turns)
	}
}
