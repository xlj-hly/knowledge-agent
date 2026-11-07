package usecase

import (
	"context"
	"go-server/internal/app/knowledge/dto"
	"go-server/internal/domain/knowledge"
	domain "go-server/internal/domain/saas"
	"sort"
	"strings"
	"unicode/utf8"
)

// Search 执行明确选择的关键词、向量或混合检索，所有结果再次限定企业与当前处理代。
func (s *Service) Search(ctx context.Context, id dto.Identity, target string, in dto.SearchInput) ([]dto.Hit, error) {
	a, err := s.actor(ctx, id, false)
	if err != nil {
		return nil, err
	}
	if err = validID(target); err != nil {
		return nil, err
	}
	in.Query = strings.TrimSpace(in.Query)
	if in.Query == "" || !utf8.ValidString(in.Query) || strings.ContainsRune(in.Query, 0) || utf8.RuneCountInString(in.Query) > 2000 {
		return nil, domain.Invalid("检索内容不能为空且最多2000字")
	}
	if in.Limit == 0 {
		in.Limit = 5
	}
	if in.Limit < 1 || in.Limit > 50 {
		return nil, domain.Invalid("检索数量为1–50")
	}
	b, err := s.deps.Repository.Base(ctx, a.TenantID, target, false)
	if err != nil {
		return nil, err
	}
	mode := in.Mode
	if mode == "" {
		mode = b.RetrievalMode
	}
	if mode != "keyword" && mode != "vector" && mode != "hybrid" {
		return nil, domain.Invalid("检索模式无效")
	}
	if mode != "keyword" && b.IndexingMode == "keyword" {
		return nil, domain.Invalid("该知识库没有向量索引，请选择关键词检索")
	}
	var keywords, vectors []knowledge.Hit
	if mode != "vector" {
		keywords, err = s.deps.Repository.KeywordSearch(ctx, a.TenantID, target, in.Query, in.Limit)
		if err != nil {
			return nil, err
		}
	}
	if mode != "keyword" {
		model, e := s.deps.Models.ResolveModel(ctx, a.TenantID, b.EmbeddingModelID, "embedding")
		if e != nil {
			return nil, e
		}
		store, e := s.deps.Repository.VectorStore(ctx, a.TenantID, b.VectorStoreID, false)
		if e != nil {
			return nil, e
		}
		embedding, e := s.deps.Embedder.Embed(ctx, model, []string{in.Query})
		if e != nil {
			return nil, e
		}
		if len(embedding) != 1 || len(embedding[0]) != model.Dimension {
			return nil, domain.Invalid("模型返回维数与页面配置不一致")
		}
		raw, e := s.deps.Vectors.Search(ctx, store, a.TenantID, target, embedding[0], in.Limit*4)
		if e != nil {
			return nil, e
		}
		ids := make([]string, 0, len(raw))
		score := map[string]float64{}
		for _, h := range raw {
			if validID(h.ID) == nil {
				ids = append(ids, h.ID)
				score[h.ID] = h.Score
			}
		}
		vectors, e = s.deps.Repository.CurrentHits(ctx, a.TenantID, target, ids)
		if e != nil {
			return nil, e
		}
		for i := range vectors {
			vectors[i].Score = score[vectors[i].Chunk.ID]
		}
		sort.Slice(vectors, func(i, j int) bool { return vectors[i].Score > vectors[j].Score })
	}
	result := keywords
	if mode == "vector" {
		result = vectors
	}
	if mode == "hybrid" {
		result = fuse(keywords, vectors)
	}
	if len(result) > in.Limit {
		result = result[:in.Limit]
	}
	out := make([]dto.Hit, 0, len(result))
	for _, h := range result {
		out = append(out, dto.Hit{Chunk: chunkView(h.Chunk), DocumentTitle: h.DocumentTitle, FileName: h.FileName, Score: h.Score})
	}
	return out, nil
}

// fuse 使用与 WeKnora 相同的 Reciprocal Rank Fusion 思路，避免比较不同检索器的原始分数。
func fuse(groups ...[]knowledge.Hit) []knowledge.Hit {
	byID := map[string]knowledge.Hit{}
	for _, group := range groups {
		for i, h := range group {
			old, exists := byID[h.Chunk.ID]
			if !exists {
				old = h
				old.Score = 0
			}
			old.Score += 1 / float64(60+i+1)
			byID[h.Chunk.ID] = old
		}
	}
	out := make([]knowledge.Hit, 0, len(byID))
	for _, h := range byID {
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Chunk.ID < out[j].Chunk.ID
		}
		return out[i].Score > out[j].Score
	})
	return out
}
