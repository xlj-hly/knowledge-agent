package usecase

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"go-server/internal/app/knowledge/dto"
	"go-server/internal/app/knowledge/port"
	"go-server/internal/domain/ai"
	"go-server/internal/domain/knowledge"
	saas "go-server/internal/domain/saas"
)

type testAuthorization struct{}

func (testAuthorization) Authorize(context.Context, string, string) (port.Actor, error) {
	return port.Actor{TenantID: "10", UserID: "20", Role: "admin"}, nil
}

type testModels struct{ settings ai.Settings }

func (m testModels) Defaults(context.Context, string) (ai.Settings, error) { return m.settings, nil }
func (testModels) ResolveModel(_ context.Context, tenant, id, kind string) (ai.Model, error) {
	if tenant != "10" || id != "41" || kind != "embedding" {
		return ai.Model{}, saas.NotFound()
	}
	return ai.Model{ID: id, TenantID: tenant, Kind: kind, Dimension: 3}, nil
}

type memoryKnowledge struct {
	knowledge.Repository
	base      knowledge.Base
	docs      map[string]knowledge.Document
	stores    map[string]knowledge.VectorStore
	completed []knowledge.Chunk
}

func (m *memoryKnowledge) Transaction(ctx context.Context, fn func(knowledge.Repository) error) error {
	copyRepo := *m
	copyRepo.docs = make(map[string]knowledge.Document, len(m.docs))
	for id, d := range m.docs {
		d.CleanupTargets = append([]knowledge.CleanupTarget(nil), d.CleanupTargets...)
		copyRepo.docs[id] = d
	}
	if err := fn(&copyRepo); err != nil {
		return err
	}
	*m = copyRepo
	return nil
}
func (m *memoryKnowledge) Base(_ context.Context, tenant, id string, _ bool) (knowledge.Base, error) {
	if tenant != "10" || id != m.base.ID {
		return knowledge.Base{}, saas.NotFound()
	}
	b := m.base
	b.DocumentCount = int64(len(m.docs))
	for _, d := range m.docs {
		if d.ParseStatus == "completed" {
			b.ChunkCount += d.ChunkCount
		}
		if d.ParseStatus == "processing" || d.ParseStatus == "queued" || d.ParseStatus == "deleting" {
			b.ProcessingCount++
		}
	}
	return b, nil
}
func (m *memoryKnowledge) SaveBase(_ context.Context, b knowledge.Base, _ bool) error {
	m.base = b
	return nil
}
func (m *memoryKnowledge) Documents(context.Context, string, string) ([]knowledge.Document, error) {
	out := []knowledge.Document{}
	for _, d := range m.docs {
		out = append(out, d)
	}
	return out, nil
}
func (m *memoryKnowledge) Document(_ context.Context, t, id string, _ bool) (knowledge.Document, error) {
	d, ok := m.docs[id]
	if !ok || t != d.TenantID {
		return d, saas.NotFound()
	}
	return d, nil
}
func (m *memoryKnowledge) SaveDocument(_ context.Context, d knowledge.Document, _ bool) error {
	m.docs[d.ID] = d
	return nil
}
func (m *memoryKnowledge) VectorStore(_ context.Context, t, id string, _ bool) (knowledge.VectorStore, error) {
	v, ok := m.stores[id]
	if !ok || v.TenantID != t {
		return v, saas.NotFound()
	}
	return v, nil
}
func (m *memoryKnowledge) Complete(_ context.Context, d knowledge.Document, chunks []knowledge.Chunk) (bool, error) {
	m.completed = chunks
	d.ParseStatus = "completed"
	m.docs[d.ID] = d
	return true, nil
}

func rebuildFixture() (*Service, *memoryKnowledge, dto.BaseInput) {
	settings := ai.DefaultSettings()
	settings.EmbeddingModelID = "41"
	settings.VectorStoreID = "31"
	repo := &memoryKnowledge{base: knowledge.Base{ID: "11", TenantID: "10", CreatorID: "20", Name: "制度", IndexingMode: "hybrid", RetrievalMode: "hybrid", ChunkStrategy: "auto", ChunkSize: 512, ChunkOverlap: 80, EmbeddingModelID: "41", VectorStoreID: "30"}, docs: map[string]knowledge.Document{"21": {ID: "21", TenantID: "10", BaseID: "11", Content: "交通报销每日120元。", ParseStatus: "completed", Attempt: "old", IndexStoreID: "30", ChunkCount: 1}}, stores: map[string]knowledge.VectorStore{"30": {ID: "30", TenantID: "10"}, "31": {ID: "31", TenantID: "10"}}}
	service := New(Dependencies{Repository: repo, Authorizer: testAuthorization{}, Models: testModels{settings: settings}})
	overlap := 20
	return service, repo, dto.BaseInput{Name: "制度", IndexingMode: "hybrid", RetrievalMode: "hybrid", EmbeddingModelID: "41", VectorStoreID: "31", ChunkStrategy: "heading", ChunkSize: 256, ChunkOverlap: &overlap}
}

func TestRebuildQueuesDocumentsAndKeepsOldIndexOwnership(t *testing.T) {
	s, repo, in := rebuildFixture()
	out, err := s.Rebuild(context.Background(), dto.Identity{}, "11", in)
	if err != nil {
		t.Fatal(err)
	}
	d := repo.docs["21"]
	if out.ProcessingCount != 1 || out.ChunkCount != 0 || d.ParseStatus != "queued" || d.Attempt == "old" || d.UseChunks {
		t.Fatalf("未失效并排队旧资料: %+v %+v", out, d)
	}
	if !reflect.DeepEqual(d.CleanupTargets, []knowledge.CleanupTarget{{StoreID: "30", Attempt: "old"}}) {
		t.Fatalf("旧索引连接归属丢失: %+v", d.CleanupTargets)
	}
	if repo.base.VectorStoreID != "31" || repo.base.ChunkStrategy != "heading" || d.Content != "交通报销每日120元。" {
		t.Fatal("新处理配置或原文保存错误")
	}
}

func TestRebuildRejectsActiveWorkWithoutPartialChanges(t *testing.T) {
	s, repo, in := rebuildFixture()
	d := repo.docs["21"]
	d.ParseStatus = "processing"
	repo.docs[d.ID] = d
	_, err := s.Rebuild(context.Background(), dto.Identity{}, "11", in)
	var business *saas.Error
	if !errors.As(err, &business) || business.Code != "conflict" {
		t.Fatalf("活动任务未明确拒绝: %v", err)
	}
	if repo.base.VectorStoreID != "30" || repo.docs["21"].Attempt != "old" {
		t.Fatal("拒绝重建后仍改变了配置或处理代")
	}
}

func TestRetrievalStrategyChangesDoNotRequireRebuild(t *testing.T) {
	s, repo, in := rebuildFixture()
	in.VectorStoreID = "30"
	in.ChunkStrategy = "auto"
	in.ChunkSize = 512
	overlap := 80
	in.ChunkOverlap = &overlap
	in.RetrievalMode = "vector"
	if _, err := s.SaveBase(context.Background(), dto.Identity{}, "11", in); err != nil {
		t.Fatal(err)
	}
	if repo.base.RetrievalMode != "vector" || repo.docs["21"].Attempt != "old" || repo.docs["21"].ParseStatus != "completed" {
		t.Fatal("查询策略变更不应重新处理文档")
	}
}

func TestNewBaseWithoutEmbeddingDoesNotFallBackToKeyword(t *testing.T) {
	s, repo, _ := rebuildFixture()
	s.deps.Models = testModels{settings: ai.DefaultSettings()}
	_, err := s.SaveBase(context.Background(), dto.Identity{}, "", dto.BaseInput{Name: "新资料"})
	if err == nil {
		t.Fatal("缺少模型的默认混合索引被悄悄降为关键词")
	}
	if repo.base.ID != "11" {
		t.Fatal("校验失败仍写入了知识库")
	}
}

type capturedEmbedding struct{ texts []string }

func (e *capturedEmbedding) Embed(_ context.Context, _ ai.Model, texts []string) ([][]float32, error) {
	e.texts = append([]string(nil), texts...)
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = []float32{1, 2, 3}
	}
	return out, nil
}

type testProcessor struct{}

func (testProcessor) Parse(context.Context, string, []byte, string) (string, error) {
	return "", errors.New("unexpected parse")
}
func (testProcessor) Split(content string, _ knowledge.ChunkingConfig) []port.ChunkDraft {
	return []port.ChunkDraft{{Content: content, ContextHeader: "# 员工手册 > 出差", Start: 0, End: len([]rune(content))}}
}

type capturedVectors struct {
	port.Vectors
	deleted []knowledge.CleanupTarget
}

func (v *capturedVectors) DeleteAttempt(_ context.Context, store knowledge.VectorStore, _, _, _, attempt string) error {
	v.deleted = append(v.deleted, knowledge.CleanupTarget{StoreID: store.ID, Attempt: attempt})
	return nil
}
func (v *capturedVectors) ReplaceDocument(context.Context, knowledge.VectorStore, string, string, string, string, []port.VectorPoint) error {
	return nil
}

func TestProcessingUsesHeadingContextAndCleansOriginalStore(t *testing.T) {
	s, repo, _ := rebuildFixture()
	repo.base.VectorStoreID = "31"
	d := repo.docs["21"]
	d.ParseStatus = "processing"
	d.IndexStoreID = "31"
	d.Attempt = "new"
	d.CleanupTargets = []knowledge.CleanupTarget{{StoreID: "30", Attempt: "old"}}
	repo.docs[d.ID] = d
	embed := &capturedEmbedding{}
	vectors := &capturedVectors{}
	s.deps.Embedder = embed
	s.deps.Vectors = vectors
	s.deps.Processor = testProcessor{}
	stage := ""
	if err := s.process(context.Background(), d, &stage); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(embed.texts, []string{"# 员工手册 > 出差\n\n交通报销每日120元。"}) {
		t.Fatalf("向量化丢失标题: %q", embed.texts)
	}
	if !reflect.DeepEqual(vectors.deleted, []knowledge.CleanupTarget{{StoreID: "30", Attempt: "old"}}) {
		t.Fatalf("没有清理原连接: %+v", vectors.deleted)
	}
	if len(repo.completed) != 1 || repo.completed[0].Content != d.Content || repo.completed[0].ContextHeader == "" {
		t.Fatal("持久化丢失标题或破坏源文")
	}
}
