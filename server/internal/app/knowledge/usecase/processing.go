package usecase

import (
	"context"
	"errors"
	"fmt"
	"go-server/internal/app/knowledge/port"
	"go-server/internal/domain/knowledge"
	domain "go-server/internal/domain/saas"
	"io"
	"log/slog"
	"strings"
	"time"
)

const processTimeout = 2 * time.Minute
const cleanupTimeout = 5 * time.Second

// Run 持续消费持久化任务，取消时退出；单项失败保存状态并允许页面显式重试。
func (s *Service) Run(ctx context.Context) error {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			worked, err := s.ProcessPending(ctx)
			if err != nil && ctx.Err() == nil {
				op := "document processing"
				var source interface{ Operation() string }
				if errors.As(err, &source) {
					op = source.Operation()
				}
				slog.Error("Knowledge background task failed", "operation", op)
			}
			delay := time.Second
			if worked {
				delay = 10 * time.Millisecond
			}
			timer.Reset(delay)
		}
	}
}

// ProcessPending 领取并处理一项；租约长于有界处理期限，重启可恢复已过期任务。
func (s *Service) ProcessPending(ctx context.Context) (bool, error) {
	generation, err := attempt()
	if err != nil {
		return false, err
	}
	d, err := s.deps.Repository.Claim(ctx, now(), now().Add(2*processTimeout+cleanupTimeout), generation)
	if err != nil {
		var business *domain.Error
		if errors.As(err, &business) && business.Code == "not_found" {
			return false, nil
		}
		return false, err
	}
	workCtx, cancel := context.WithTimeout(ctx, processTimeout)
	defer cancel()
	if d.ParseStatus == "deleting" {
		return true, s.finishDelete(workCtx, d)
	}
	stage := "资料处理失败，请检查配置后重试"
	err = s.process(workCtx, d, &stage)
	if err == nil {
		return true, nil
	}
	cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cleanupCancel()
	failed := s.deps.Repository.Fail(cleanupCtx, d, stage)
	return true, errors.Join(err, failed)
}

func (s *Service) process(ctx context.Context, d knowledge.Document, stage *string) error {
	b, err := s.deps.Repository.Base(ctx, d.TenantID, d.BaseID, false)
	if err != nil {
		return err
	}
	*stage = "旧向量清理失败，请检查原向量连接后重试"
	if err = s.cleanupTargets(ctx, d); err != nil {
		return err
	}
	var drafts []port.ChunkDraft
	if d.UseChunks {
		chunks, e := s.deps.Repository.Chunks(ctx, d.TenantID, d.ID)
		if e != nil {
			return e
		}
		for _, c := range chunks {
			drafts = append(drafts, port.ChunkDraft{Content: c.Content, ContextHeader: c.ContextHeader, Index: c.Index, Start: c.Start, End: c.End})
		}
	} else {
		*stage = "文档解析失败，请检查文件或解析服务后重试"
		if d.Source == "file" {
			obj, e := s.deps.Objects.Get(ctx, d.ObjectKey)
			if e != nil {
				return e
			}
			body, e := io.ReadAll(io.LimitReader(obj.Body, MaxFileSize+1))
			closeErr := obj.Body.Close()
			if e != nil || closeErr != nil {
				return errors.Join(e, closeErr)
			}
			if len(body) > MaxFileSize {
				return domain.Invalid("原文件超过大小限制")
			}
			d.Content, e = s.deps.Processor.Parse(ctx, d.FileName, body, b.ParserEndpoint)
			if e != nil {
				return e
			}
		}
		if len(d.Content) > maxTextSize || strings.TrimSpace(d.Content) == "" {
			return domain.Invalid("解析文本为空或超过 2 MiB")
		}
		drafts = s.deps.Processor.Split(d.Content, b.Chunking())
	}
	if len(drafts) == 0 || len(drafts) > 10000 {
		return domain.Invalid("分块结果为空或数量超过限制")
	}
	chunks := make([]knowledge.Chunk, 0, len(drafts))
	texts := make([]string, 0, len(drafts))
	for _, draft := range drafts {
		id, e := newID()
		if e != nil {
			return e
		}
		chunks = append(chunks, knowledge.Chunk{ID: id, TenantID: d.TenantID, BaseID: d.BaseID, DocumentID: d.ID, Attempt: d.Attempt, Content: draft.Content, ContextHeader: draft.ContextHeader, Index: draft.Index, Start: draft.Start, End: draft.End, CreatedAt: now()})
		texts = append(texts, draft.EmbeddingContent())
	}
	var store knowledge.VectorStore
	indexed := false
	if b.IndexingMode != "keyword" {
		*stage = "向量化失败，请在模型配置中测试连接后重试"
		model, e := s.deps.Models.ResolveModel(ctx, d.TenantID, b.EmbeddingModelID, "embedding")
		if e != nil {
			return e
		}
		store, e = s.deps.Repository.VectorStore(ctx, d.TenantID, b.VectorStoreID, false)
		if e != nil {
			return e
		}
		*stage = "向量化失败，请在模型配置中测试连接后重试"
		vectors, e := s.deps.Embedder.Embed(ctx, model, texts)
		if e != nil {
			return e
		}
		if len(vectors) != len(chunks) {
			return domain.Invalid("模型返回的向量数量不匹配")
		}
		points := make([]port.VectorPoint, 0, len(chunks))
		for i, c := range chunks {
			if len(vectors[i]) != model.Dimension {
				return domain.Invalid("模型返回维数与页面配置不一致")
			}
			points = append(points, port.VectorPoint{ID: c.ID, Content: c.EmbeddingContent(), Vector: vectors[i]})
		}
		*stage = "向量索引写入失败，请在向量连接中测试后重试"
		indexed = true
		if e = s.deps.Vectors.ReplaceDocument(ctx, store, d.TenantID, d.BaseID, d.ID, d.Attempt, points); e != nil {
			return errors.Join(e, s.cleanAttempt(ctx, store, d))
		}
	}
	*stage = "资料状态保存失败，请重试处理"
	d.CleanupTargets = nil
	ok, err := s.deps.Repository.Complete(ctx, d, chunks)
	if err != nil || !ok {
		if indexed {
			err = errors.Join(err, s.cleanAttempt(ctx, store, d))
		}
		if err != nil {
			return err
		}
		return domain.Conflict("处理任务已被新的编辑或删除取代")
	}
	return nil
}
func (s *Service) cleanAttempt(ctx context.Context, store knowledge.VectorStore, d knowledge.Document) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	if err := s.deps.Vectors.DeleteAttempt(cleanupCtx, store, d.TenantID, d.BaseID, d.ID, d.Attempt); err != nil {
		return fmt.Errorf("清理失效向量: %w", err)
	}
	return nil
}

func (s *Service) cleanupTargets(ctx context.Context, d knowledge.Document) error {
	for _, target := range d.CleanupTargets {
		if target.StoreID == "" || target.Attempt == "" {
			return domain.Invalid("资料的旧索引清理记录无效")
		}
		store, err := s.deps.Repository.VectorStore(ctx, d.TenantID, target.StoreID, false)
		if err != nil {
			return err
		}
		if err = s.deps.Vectors.DeleteAttempt(ctx, store, d.TenantID, d.BaseID, d.ID, target.Attempt); err != nil {
			return err
		}
	}
	return nil
}
