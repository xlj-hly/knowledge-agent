package usecase

import (
	"context"
	"go-server/internal/app/knowledge/dto"
	"go-server/internal/domain/knowledge"
	domain "go-server/internal/domain/saas"
	"strings"
)

// Bases 列出当前企业知识库。
func (s *Service) Bases(ctx context.Context, id dto.Identity) ([]dto.Base, error) {
	a, err := s.actor(ctx, id, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.deps.Repository.Bases(ctx, a.TenantID)
	out := make([]dto.Base, 0, len(rows))
	for _, r := range rows {
		out = append(out, baseView(r))
	}
	return out, err
}

// Base 返回知识库详情与当前计数。
func (s *Service) Base(ctx context.Context, id dto.Identity, target string) (dto.Base, error) {
	a, err := s.actor(ctx, id, false)
	if err != nil {
		return dto.Base{}, err
	}
	if err = validID(target); err != nil {
		return dto.Base{}, err
	}
	b, err := s.deps.Repository.Base(ctx, a.TenantID, target, false)
	return baseView(b), err
}

// SaveBase 保存元信息与检索策略；影响已有索引的配置需要显式重建。
func (s *Service) SaveBase(ctx context.Context, id dto.Identity, target string, in dto.BaseInput) (dto.Base, error) {
	return s.saveBase(ctx, id, target, in, false)
}

// Rebuild 原子失效全部资料的旧处理代，再由后台按新配置重新处理。
func (s *Service) Rebuild(ctx context.Context, id dto.Identity, target string, in dto.BaseInput) (dto.Base, error) {
	if target == "" {
		return dto.Base{}, domain.Invalid("请指定要重建的知识库")
	}
	return s.saveBase(ctx, id, target, in, true)
}

func (s *Service) saveBase(ctx context.Context, id dto.Identity, target string, in dto.BaseInput, rebuild bool) (dto.Base, error) {
	a, err := s.actor(ctx, id, false)
	if err != nil {
		return dto.Base{}, err
	}
	create := target == ""
	if create {
		target, err = newID()
	} else {
		err = validID(target)
	}
	if err != nil {
		return dto.Base{}, err
	}
	defaults, err := s.deps.Models.Defaults(ctx, a.TenantID)
	if err != nil {
		return dto.Base{}, err
	}
	mode := in.IndexingMode
	if mode == "" {
		mode = "hybrid"
	}
	retrieval := in.RetrievalMode
	if retrieval == "" {
		retrieval = defaults.RetrievalMode
		if mode == "keyword" {
			retrieval = "keyword"
		}
	}
	size, overlap, strategy := in.ChunkSize, defaults.ChunkOverlap, in.ChunkStrategy
	if size == 0 {
		size = defaults.ChunkSize
	}
	if in.ChunkOverlap != nil {
		overlap = *in.ChunkOverlap
	}
	if strategy == "" {
		strategy = defaults.ChunkStrategy
	}
	modelID, storeID := in.EmbeddingModelID, in.VectorStoreID
	if mode != "keyword" {
		if modelID == "" {
			modelID = defaults.EmbeddingModelID
		}
		if storeID == "" {
			storeID = defaults.VectorStoreID
		}
	} else {
		modelID = ""
		storeID = ""
	}
	parser := strings.TrimRight(strings.TrimSpace(in.ParserEndpoint), "/")
	if create && parser == "" {
		parser = defaults.ParserEndpoint
	}
	b := knowledge.Base{ID: target, TenantID: a.TenantID, CreatorID: a.UserID, Name: strings.TrimSpace(in.Name), Description: strings.TrimSpace(in.Description), IndexingMode: mode, RetrievalMode: retrieval, EmbeddingModelID: modelID, VectorStoreID: storeID, ParserEndpoint: parser, ChunkStrategy: strategy, ChunkSize: size, ChunkOverlap: overlap, CreatedAt: now(), UpdatedAt: now()}
	if err = b.Validate(); err != nil {
		return dto.Base{}, err
	}
	for _, v := range []string{modelID, storeID} {
		if v != "" {
			if err = validID(v); err != nil {
				return dto.Base{}, err
			}
		}
	}
	err = s.withConfiguration(ctx, a.TenantID, func(ctx context.Context) error {
		if modelID != "" {
			if _, err = s.deps.Models.ResolveModel(ctx, a.TenantID, modelID, "embedding"); err != nil {
				return err
			}
		}
		return s.deps.Repository.Transaction(ctx, func(tx knowledge.Repository) error {
			if !create {
				old, e := tx.Base(ctx, a.TenantID, target, true)
				if e != nil {
					return e
				}
				changed := old.IndexingMode != b.IndexingMode || old.EmbeddingModelID != b.EmbeddingModelID || old.VectorStoreID != b.VectorStoreID || old.ChunkSize != b.ChunkSize || old.ChunkOverlap != b.ChunkOverlap || old.ChunkStrategy != b.ChunkStrategy || old.ParserEndpoint != b.ParserEndpoint
				if old.DocumentCount > 0 && changed && !rebuild {
					return domain.Conflict("处理配置已变化，请使用保存并重建以重新处理已有资料")
				}
				b.CreatorID, b.CreatedAt, b.DocumentCount, b.ChunkCount, b.ProcessingCount = old.CreatorID, old.CreatedAt, old.DocumentCount, old.ChunkCount, old.ProcessingCount
				if rebuild {
					docs, e := tx.Documents(ctx, a.TenantID, target)
					if e != nil {
						return e
					}
					// 所有写入和任务领取先锁知识库，再锁资料，避免重建与新任务交错。
					for _, item := range docs {
						d, e := tx.Document(ctx, a.TenantID, item.ID, true)
						if e != nil {
							return e
						}
						if d.ParseStatus == "processing" || d.ParseStatus == "deleting" {
							return domain.Conflict("知识库仍有处理中或删除中的资料，请完成后重建")
						}
						if e = queue(ctx, tx, &d, false); e != nil {
							return e
						}
					}
					b.ChunkCount = 0
					b.ProcessingCount = int64(len(docs))
				}
			}
			if b.VectorStoreID != "" {
				if _, e := tx.VectorStore(ctx, a.TenantID, b.VectorStoreID, true); e != nil {
					return e
				}
			}
			return tx.SaveBase(ctx, b, create)
		})
	})
	return baseView(b), err
}

// Preview 使用与后台一致的分割契约，不保存文件、分块或向量。
func (s *Service) Preview(ctx context.Context, id dto.Identity, _ string, in dto.PreviewInput) ([]dto.PreviewChunk, error) {
	if _, err := s.actor(ctx, id, false); err != nil {
		return nil, err
	}
	if err := validateText("切分预览", in.Content); err != nil {
		return nil, err
	}
	cfg := knowledge.ChunkingConfig{Strategy: in.ChunkStrategy, Size: in.ChunkSize, Overlap: in.ChunkOverlap}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	chunks := s.deps.Processor.Split(in.Content, cfg)
	if len(chunks) > 10000 {
		return nil, domain.Invalid("分块数量超过限制")
	}
	out := make([]dto.PreviewChunk, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, dto.PreviewChunk{Content: c.Content, ContextHeader: c.ContextHeader, Index: c.Index, Start: c.Start, End: c.End})
	}
	return out, nil
}

// ModelReferences 向模型配置用例提供知识库引用数量，不暴露知识私有仓储。
func (s *Service) ModelReferences(ctx context.Context, tenant, id string) (int64, error) {
	return s.deps.Repository.ModelReferences(ctx, tenant, id)
}

// ResolveVectorStore 验证已授权企业的向量连接可供默认模板引用。
func (s *Service) ResolveVectorStore(ctx context.Context, tenant, id string) error {
	_, err := s.deps.Repository.VectorStore(ctx, tenant, id, false)
	return err
}

// DeleteBase 先回收资料资源，再在知识库行锁内确认没有并发新增资料。
// 外部清理失败时保留可重试记录；已完成的删除不会因后续失败而恢复。
func (s *Service) DeleteBase(ctx context.Context, id dto.Identity, target string) (dto.OK, error) {
	a, err := s.actor(ctx, id, false)
	if err != nil {
		return dto.OK{}, err
	}
	if err = validID(target); err != nil {
		return dto.OK{}, err
	}
	if _, err = s.deps.Repository.Base(ctx, a.TenantID, target, false); err != nil {
		return dto.OK{}, err
	}
	docs, err := s.deps.Repository.Documents(ctx, a.TenantID, target)
	if err != nil {
		return dto.OK{}, err
	}
	for _, d := range docs {
		if err = s.deleteDocument(ctx, a.TenantID, d.ID); err != nil {
			return dto.OK{}, err
		}
	}
	err = s.deps.Repository.Transaction(ctx, func(tx knowledge.Repository) error {
		b, e := tx.Base(ctx, a.TenantID, target, true)
		if e != nil {
			return e
		}
		if b.DocumentCount != 0 {
			return domain.Conflict("知识库仍有新增或正在清理的资料，请等待后重试删除")
		}
		return tx.DeleteBase(ctx, a.TenantID, target)
	})
	return dto.OK{OK: err == nil}, err
}
