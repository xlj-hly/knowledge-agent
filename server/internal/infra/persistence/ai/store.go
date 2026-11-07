// Package repository 实现企业 AI 模型和新建模板的持久化。
package repository

import (
	"context"
	"errors"
	"fmt"
	"go-server/internal/domain/ai"
	saas "go-server/internal/domain/saas"
	"go-server/internal/infra/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Store struct{ db *gorm.DB }
type modelRow ai.Model

func (modelRow) TableName() string { return "ai_models" }

type settingsRow struct {
	TenantID                                     string
	ChatModelID, EmbeddingModelID, VectorStoreID *string
	SystemPrompt                                 string
	Temperature                                  float64
	MaxTokens, HistoryTurns                      int
	RetrievalMode                                string
	RetrievalLimit                               int
	SimilarityThreshold                          float64
	ChunkStrategy                                string
	ChunkSize, ChunkOverlap                      int
	ParserEndpoint                               string
}

func (settingsRow) TableName() string { return "ai_settings" }
func New(db *gorm.DB) *Store          { return &Store{db} }

var _ ai.Repository = (*Store)(nil)

type operationError struct {
	operation string
	cause     error
}

func (e *operationError) Error() string     { return fmt.Sprintf("AI repository %s failed", e.operation) }
func (e *operationError) Unwrap() error     { return e.cause }
func (e *operationError) Operation() string { return "AI " + e.operation }
func checked(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return saas.NotFound()
	}
	var business *saas.Error
	if errors.As(err, &business) {
		return err
	}
	return &operationError{op, err}
}
func (s *Store) Transaction(ctx context.Context, fn func(ai.Repository) error) error {
	if database.InTransaction(ctx) {
		return fn(New(database.Connection(ctx, s.db)))
	}
	return checked("transaction", database.Connection(ctx, s.db).WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(New(tx)) }))
}
func (s *Store) Models(ctx context.Context, t string) ([]ai.Model, error) {
	rows := []modelRow{}
	err := database.Connection(ctx, s.db).WithContext(ctx).Where("tenant_id = ?", t).Order("created_at DESC,id DESC").Find(&rows).Error
	out := make([]ai.Model, 0, len(rows))
	for _, r := range rows {
		out = append(out, ai.Model(r))
	}
	return out, checked("models", err)
}
func (s *Store) Model(ctx context.Context, t, id string, write bool) (ai.Model, error) {
	q := database.Connection(ctx, s.db).WithContext(ctx).Where("tenant_id = ? AND id = ?", t, id)
	if write {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var r modelRow
	err := q.First(&r).Error
	return ai.Model(r), checked("model", err)
}
func (s *Store) SaveModel(ctx context.Context, m ai.Model, create bool) error {
	r := modelRow(m)
	q := database.Connection(ctx, s.db).WithContext(ctx)
	if create {
		return checked("create model", q.Create(&r).Error)
	}
	return checked("save model", q.Model(&modelRow{}).Where("tenant_id = ? AND id = ?", m.TenantID, m.ID).Select("name", "kind", "provider", "endpoint", "model_name", "dimension", "api_key", "updated_at").Updates(&r).Error)
}
func (s *Store) DeleteModel(ctx context.Context, t, id string) error {
	return checked("delete model", database.Connection(ctx, s.db).WithContext(ctx).Where("tenant_id = ? AND id = ?", t, id).Delete(&modelRow{}).Error)
}
func (s *Store) Settings(ctx context.Context, t string) (ai.Settings, error) {
	var r settingsRow
	err := database.Connection(ctx, s.db).WithContext(ctx).Where("tenant_id = ?", t).Take(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		v := ai.DefaultSettings()
		v.TenantID = t
		return v, nil
	}
	v := ai.Settings{TenantID: r.TenantID, SystemPrompt: r.SystemPrompt, Temperature: r.Temperature, MaxTokens: r.MaxTokens, HistoryTurns: r.HistoryTurns, RetrievalMode: r.RetrievalMode, RetrievalLimit: r.RetrievalLimit, SimilarityThreshold: r.SimilarityThreshold, ChunkStrategy: r.ChunkStrategy, ChunkSize: r.ChunkSize, ChunkOverlap: r.ChunkOverlap, ParserEndpoint: r.ParserEndpoint}
	if r.ChatModelID != nil {
		v.ChatModelID = *r.ChatModelID
	}
	if r.EmbeddingModelID != nil {
		v.EmbeddingModelID = *r.EmbeddingModelID
	}
	if r.VectorStoreID != nil {
		v.VectorStoreID = *r.VectorStoreID
	}
	return v, checked("settings", err)
}
func (s *Store) SaveSettings(ctx context.Context, v ai.Settings) error {
	r := settingsRow{TenantID: v.TenantID, SystemPrompt: v.SystemPrompt, Temperature: v.Temperature, MaxTokens: v.MaxTokens, HistoryTurns: v.HistoryTurns, RetrievalMode: v.RetrievalMode, RetrievalLimit: v.RetrievalLimit, SimilarityThreshold: v.SimilarityThreshold, ChunkStrategy: v.ChunkStrategy, ChunkSize: v.ChunkSize, ChunkOverlap: v.ChunkOverlap, ParserEndpoint: v.ParserEndpoint}
	if v.ChatModelID != "" {
		r.ChatModelID = &v.ChatModelID
	}
	if v.EmbeddingModelID != "" {
		r.EmbeddingModelID = &v.EmbeddingModelID
	}
	if v.VectorStoreID != "" {
		r.VectorStoreID = &v.VectorStoreID
	}
	return checked("save settings", database.Connection(ctx, s.db).WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_id"}}, DoUpdates: clause.AssignmentColumns([]string{"chat_model_id", "embedding_model_id", "vector_store_id", "system_prompt", "temperature", "max_tokens", "history_turns", "retrieval_mode", "retrieval_limit", "similarity_threshold", "chunk_strategy", "chunk_size", "chunk_overlap", "parser_endpoint"})}).Create(&r).Error)
}
