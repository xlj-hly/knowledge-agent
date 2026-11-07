// Package repository 实现企业范围内的知识库关系数据和后台任务领取。
package repository

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"go-server/internal/domain/knowledge"
	domain "go-server/internal/domain/saas"
	"go-server/internal/infra/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	"time"
)

// Store 绑定显式 GORM 连接，不拥有连接池生命周期。
type Store struct{ db *gorm.DB }

var _ knowledge.Repository = (*Store)(nil)

// New 创建知识库仓储，不执行建表或自动迁移。
func New(db *gorm.DB) *Store { return &Store{db: db} }

// Transaction 负责回滚与提交；回调外不能继续使用事务仓储。
func (s *Store) Transaction(ctx context.Context, fn func(knowledge.Repository) error) error {
	connection := database.Connection(ctx, s.db)
	if database.InTransaction(ctx) {
		return fn(New(connection))
	}
	return checked("transaction", connection.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(New(tx)) }))
}

type operationError struct {
	operation string
	cause     error
}

func (e *operationError) Error() string {
	return fmt.Sprintf("knowledge repository %s: %v", e.operation, e.cause)
}
func (e *operationError) Unwrap() error     { return e.cause }
func (e *operationError) Operation() string { return "knowledge " + e.operation }
func checked(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.NotFound()
	}
	var business *domain.Error
	if errors.As(err, &business) {
		return err
	}
	return &operationError{op, err}
}
func lock(q *gorm.DB, yes bool) *gorm.DB {
	if yes {
		return q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return q
}
func (s *Store) q(ctx context.Context, tenant string) *gorm.DB {
	return database.Connection(ctx, s.db).WithContext(ctx).Where("tenant_id = ?", tenant)
}

// ModelReferences 统计知识库引用。
func (s *Store) ModelReferences(ctx context.Context, t, id string) (int64, error) {
	var n int64
	err := s.q(ctx, t).Model(&baseRow{}).Where("embedding_model_id = ?", id).Count(&n).Error
	return n, checked("count model references", err)
}

// VectorStores 列出本企业连接。
func (s *Store) VectorStores(ctx context.Context, t string) ([]knowledge.VectorStore, error) {
	rows := []vectorRow{}
	err := s.q(ctx, t).Order("created_at DESC, id DESC").Find(&rows).Error
	if err != nil {
		return nil, checked("list vector stores", err)
	}
	out := make([]knowledge.VectorStore, 0, len(rows))
	for _, r := range rows {
		store, e := r.view()
		if e != nil {
			return nil, checked("decode vector store", e)
		}
		out = append(out, store)
	}
	return out, checked("list vector stores", err)
}

// VectorStore 读取本企业连接。
func (s *Store) VectorStore(ctx context.Context, t, id string, write bool) (knowledge.VectorStore, error) {
	var r vectorRow
	err := lock(s.q(ctx, t), write).First(&r, "id = ?", id).Error
	if err != nil {
		return knowledge.VectorStore{}, checked("get vector store", err)
	}
	store, err := r.view()
	return store, checked("decode vector store", err)
}

// SaveVectorStore 保存完整连接配置。
func (s *Store) SaveVectorStore(ctx context.Context, m knowledge.VectorStore, create bool) error {
	r, err := vectorRecord(m)
	if err != nil {
		return checked("encode vector store", err)
	}
	q := database.Connection(ctx, s.db).WithContext(ctx)
	if create {
		return checked("create vector store", q.Create(&r).Error)
	}
	return checked("update vector store", q.Model(&vectorRow{}).Where("tenant_id = ? AND id = ?", m.TenantID, m.ID).Select("name", "provider", "config", "updated_at").Updates(&r).Error)
}

// DeleteVectorStore 删除未被引用的连接。
func (s *Store) DeleteVectorStore(ctx context.Context, t, id string) error {
	return checked("delete vector store", s.q(ctx, t).Delete(&vectorRow{}, "id = ?", id).Error)
}

// VectorStoreReferences 统计知识库引用。
func (s *Store) VectorStoreReferences(ctx context.Context, t, id string) (int64, error) {
	var n int64
	err := s.q(ctx, t).Model(&baseRow{}).Where("vector_store_id = ?", id).Count(&n).Error
	if err != nil {
		return 0, checked("count vector references", err)
	}
	var pending int64
	err = s.q(ctx, t).Model(&documentRow{}).Where("index_store_id = ? OR EXISTS (SELECT 1 FROM jsonb_array_elements(cleanup_targets::jsonb) AS target WHERE target->>'store_id' = ?)", id, id).Count(&pending).Error
	return n + pending, checked("count vector cleanup references", err)
}
func baseQuery(q *gorm.DB) *gorm.DB {
	return q.Select("knowledge_bases.*, (SELECT count(*) FROM knowledge_documents d WHERE d.base_id = knowledge_bases.id AND d.tenant_id = knowledge_bases.tenant_id) AS document_count, (SELECT coalesce(sum(d.chunk_count),0) FROM knowledge_documents d WHERE d.base_id = knowledge_bases.id AND d.tenant_id = knowledge_bases.tenant_id AND d.parse_status = 'completed') AS chunk_count, (SELECT count(*) FROM knowledge_documents d WHERE d.base_id=knowledge_bases.id AND d.tenant_id=knowledge_bases.tenant_id AND d.parse_status IN ('queued','processing','deleting')) AS processing_count")
}

// Bases 列出当前资料与分块统计。
func (s *Store) Bases(ctx context.Context, t string) ([]knowledge.Base, error) {
	rows := []baseRow{}
	err := baseQuery(s.q(ctx, t)).Order("created_at DESC, id DESC").Find(&rows).Error
	out := make([]knowledge.Base, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.view())
	}
	return out, checked("list bases", err)
}

// Base 获取知识库；写操作共享此行锁来协调资料导入与索引设置更新。
func (s *Store) Base(ctx context.Context, t, id string, write bool) (knowledge.Base, error) {
	var r baseRow
	// 先取得行锁再读取计数，避免等待并发导入提交后仍使用锁前语句快照里的旧计数。
	if write {
		if err := lock(s.q(ctx, t), true).First(&r, "id = ?", id).Error; err != nil {
			return r.view(), checked("lock base", err)
		}
	}
	err := baseQuery(s.q(ctx, t)).First(&r, "id = ?", id).Error
	return r.view(), checked("get base", err)
}

// SaveBase 保存完整知识库配置。
func (s *Store) SaveBase(ctx context.Context, b knowledge.Base, create bool) error {
	r := baseRecord(b)
	q := database.Connection(ctx, s.db).WithContext(ctx)
	if create {
		return checked("create base", q.Create(&r).Error)
	}
	return checked("update base", q.Model(&baseRow{}).Where("tenant_id = ? AND id = ?", b.TenantID, b.ID).Select("name", "description", "embedding_model_id", "vector_store_id", "indexing_mode", "retrieval_mode", "chunk_strategy", "parser_endpoint", "chunk_size", "chunk_overlap", "updated_at").Updates(&r).Error)
}

// DeleteBase 删除用例确认已没有资料的知识库。
func (s *Store) DeleteBase(ctx context.Context, t, id string) error {
	return checked("delete base", s.q(ctx, t).Delete(&baseRow{}, "id = ?", id).Error)
}

// Documents 列出知识库资料，省略大文本读取。
func (s *Store) Documents(ctx context.Context, t, b string) ([]knowledge.Document, error) {
	rows := []documentRow{}
	err := s.q(ctx, t).Omit("content").Where("base_id = ?", b).Order("created_at DESC,id DESC").Find(&rows).Error
	if err != nil {
		return nil, checked("list documents", err)
	}
	out := make([]knowledge.Document, 0, len(rows))
	for _, r := range rows {
		d, e := r.view()
		if e != nil {
			return nil, checked("decode document cleanup", e)
		}
		out = append(out, d)
	}
	return out, nil
}

// Document 获取单篇资料及内容。
func (s *Store) Document(ctx context.Context, t, id string, write bool) (knowledge.Document, error) {
	var r documentRow
	err := lock(s.q(ctx, t), write).First(&r, "id = ?", id).Error
	if err != nil {
		return knowledge.Document{}, checked("get document", err)
	}
	d, err := r.view()
	return d, checked("decode document cleanup", err)
}

// SaveDocument 保存状态和处理代，必须处于已确定的资料事务内。
func (s *Store) SaveDocument(ctx context.Context, d knowledge.Document, create bool) error {
	r, err := documentRecord(d)
	if err != nil {
		return checked("encode document cleanup", err)
	}
	q := database.Connection(ctx, s.db).WithContext(ctx)
	if create {
		return checked("create document", q.Create(&r).Error)
	}
	return checked("update document", q.Model(&documentRow{}).Where("tenant_id = ? AND id = ?", d.TenantID, d.ID).Select("title", "content", "file_size", "parse_status", "error_message", "attempt", "cleanup_targets", "index_store_id", "use_chunks", "lease_until", "chunk_count", "updated_at").Updates(&r).Error)
}

// DeleteDocument 清理资料和派生分块，由同一事务提交。
func (s *Store) DeleteDocument(ctx context.Context, t, id string) error {
	if err := s.q(ctx, t).Delete(&chunkRow{}, "document_id = ?", id).Error; err != nil {
		return checked("delete chunks", err)
	}
	return checked("delete document", s.q(ctx, t).Delete(&documentRow{}, "id = ?", id).Error)
}

// Chunks 只展示当前处理代，避免旧任务结果混入。
func (s *Store) Chunks(ctx context.Context, t, id string) ([]knowledge.Chunk, error) {
	rows := []chunkRow{}
	err := database.Connection(ctx, s.db).WithContext(ctx).Table("knowledge_chunks c").Select("c.*").Joins("JOIN knowledge_documents d ON d.id = c.document_id AND d.tenant_id = c.tenant_id AND d.attempt = c.attempt").Where("c.tenant_id = ? AND c.document_id = ?", t, id).Order("c.index ASC").Scan(&rows).Error
	out := make([]knowledge.Chunk, 0, len(rows))
	for _, r := range rows {
		out = append(out, knowledge.Chunk(r))
	}
	return out, checked("list chunks", err)
}

// Chunk 获取归属本企业的分块；用例仍需校验资料当前代。
func (s *Store) Chunk(ctx context.Context, t, id string) (knowledge.Chunk, error) {
	var r chunkRow
	err := s.q(ctx, t).First(&r, "id = ?", id).Error
	return knowledge.Chunk(r), checked("get chunk", err)
}

// SaveChunk 保存用例生成的新处理代分块。
func (s *Store) SaveChunk(ctx context.Context, c knowledge.Chunk) error {
	r := chunkRow(c)
	return checked("create chunk", database.Connection(ctx, s.db).WithContext(ctx).Create(&r).Error)
}

// Claim 在短事务内领取一个任务；进程退出后的过期 processing 可由新进程继续。
func (s *Store) Claim(ctx context.Context, now, lease time.Time, generation string) (knowledge.Document, error) {
	var out knowledge.Document
	err := s.Transaction(ctx, func(repo knowledge.Repository) error {
		tx := repo.(*Store)
		var candidate documentRow
		if err := database.Connection(ctx, tx.db).WithContext(ctx).Where("parse_status = 'queued' OR (parse_status IN ('processing','deleting') AND (lease_until IS NULL OR lease_until < ?))", now).Order("updated_at ASC, id ASC").First(&candidate).Error; err != nil {
			return checked("claim document", err)
		}
		b, err := tx.Base(ctx, candidate.TenantID, candidate.BaseID, true)
		if err != nil {
			return err
		}
		d, err := tx.Document(ctx, candidate.TenantID, candidate.ID, true)
		if err != nil {
			return err
		}
		expired := d.LeaseUntil == nil || d.LeaseUntil.Before(now)
		if d.ParseStatus != "queued" && !(expired && (d.ParseStatus == "processing" || d.ParseStatus == "deleting")) {
			return domain.NotFound()
		}
		if d.ParseStatus == "processing" {
			d.RememberIndex()
			if d.UseChunks {
				chunks, e := tx.Chunks(ctx, d.TenantID, d.ID)
				if e != nil {
					return e
				}
				for _, c := range chunks {
					var random [8]byte
					if _, e = rand.Read(random[:]); e != nil {
						return e
					}
					n := binary.BigEndian.Uint64(random[:]) & 0x7fffffffffffffff
					if n == 0 {
						n = 1
					}
					c.ID = strconv.FormatUint(n, 10)
					c.Attempt = generation
					if e = tx.SaveChunk(ctx, c); e != nil {
						return e
					}
				}
			}
			d.Attempt = generation
		}
		if d.ParseStatus != "deleting" {
			d.ParseStatus = "processing"
			d.IndexStoreID = b.VectorStoreID
		}
		d.LeaseUntil = &lease
		d.UpdatedAt = now
		out = d
		return tx.SaveDocument(ctx, d, false)
	})
	return out, err
}

// Complete 以处理代和状态 CAS 提交分块，旧任务不能覆盖重新处理或删除。
func (s *Store) Complete(ctx context.Context, d knowledge.Document, chunks []knowledge.Chunk) (bool, error) {
	ok := false
	err := s.Transaction(ctx, func(repo knowledge.Repository) error {
		tx := repo.(*Store)
		if _, err := tx.Base(ctx, d.TenantID, d.BaseID, true); err != nil {
			return err
		}
		current, err := tx.Document(ctx, d.TenantID, d.ID, true)
		if err != nil {
			var e *domain.Error
			if errors.As(err, &e) && e.Code == "not_found" {
				return nil
			}
			return err
		}
		if current.Attempt != d.Attempt || current.ParseStatus != "processing" {
			return nil
		}
		if err := tx.q(ctx, d.TenantID).Delete(&chunkRow{}, "document_id = ?", d.ID).Error; err != nil {
			return checked("replace chunks", err)
		}
		for _, c := range chunks {
			if err := tx.SaveChunk(ctx, c); err != nil {
				return err
			}
		}
		d.ParseStatus = "completed"
		d.ErrorMessage = ""
		d.ChunkCount = int64(len(chunks))
		d.LeaseUntil = nil
		d.UpdatedAt = time.Now().UTC()
		if err := tx.SaveDocument(ctx, d, false); err != nil {
			return err
		}
		ok = true
		return nil
	})
	return ok, err
}

// Fail 只更新仍属于本任务的资料，不会覆盖新的处理状态。
func (s *Store) Fail(ctx context.Context, d knowledge.Document, message string) error {
	return checked("fail processing", s.q(ctx, d.TenantID).Model(&documentRow{}).Where("id = ? AND attempt = ? AND parse_status = 'processing'", d.ID, d.Attempt).Updates(map[string]any{"parse_status": "failed", "error_message": message, "lease_until": nil, "updated_at": time.Now().UTC()}).Error)
}

type hitRow struct {
	Chunk                   chunkRow `gorm:"embedded"`
	DocumentTitle, FileName string
	Score                   float64
}

func hitQuery(q *gorm.DB) *gorm.DB {
	return q.Table("knowledge_chunks c").Joins("JOIN knowledge_documents d ON d.id = c.document_id AND d.tenant_id = c.tenant_id AND d.attempt = c.attempt").Where("d.parse_status = 'completed'")
}
func hits(rows []hitRow) []knowledge.Hit {
	out := make([]knowledge.Hit, 0, len(rows))
	for _, r := range rows {
		out = append(out, knowledge.Hit{Chunk: knowledge.Chunk(r.Chunk), DocumentTitle: r.DocumentTitle, FileName: r.FileName, Score: r.Score})
	}
	return out
}

// CurrentHits 对向量返回的 ID 再做企业、知识库、状态和处理代校验。
func (s *Store) CurrentHits(ctx context.Context, t, b string, ids []string) ([]knowledge.Hit, error) {
	if len(ids) == 0 {
		return []knowledge.Hit{}, nil
	}
	rows := []hitRow{}
	err := hitQuery(database.Connection(ctx, s.db).WithContext(ctx)).Select("c.*,d.title AS document_title,d.file_name").Where("c.tenant_id = ? AND c.base_id = ? AND c.id IN ?", t, b, ids).Scan(&rows).Error
	return hits(rows), checked("load vector hits", err)
}
