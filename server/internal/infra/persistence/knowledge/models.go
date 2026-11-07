package repository

import (
	"encoding/json"
	"fmt"
	"go-server/internal/domain/knowledge"
	"time"
)

// 私有持久化类型不声明 GORM 关联；逻辑归属与状态更新由用例显式校验。
type vectorRow struct {
	ID, TenantID, Name, Provider string
	Config                       string `gorm:"type:jsonb"`
	CreatedAt, UpdatedAt         time.Time
}

func vectorRecord(store knowledge.VectorStore) (vectorRow, error) {
	data, err := json.Marshal(store.Config)
	if err != nil {
		return vectorRow{}, err
	}
	return vectorRow{ID: store.ID, TenantID: store.TenantID, Name: store.Name, Provider: store.Provider, Config: string(data), CreatedAt: store.CreatedAt, UpdatedAt: store.UpdatedAt}, nil
}

func (row vectorRow) view() (knowledge.VectorStore, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(row.Config), &fields); err != nil {
		return knowledge.VectorStore{}, err
	}
	config := make(map[string]string, len(fields))
	if fields == nil {
		return knowledge.VectorStore{}, fmt.Errorf("向量连接配置必须是对象")
	}
	for key, raw := range fields {
		var value *string
		if err := json.Unmarshal(raw, &value); err != nil || value == nil {
			return knowledge.VectorStore{}, fmt.Errorf("向量连接配置字段必须是字符串: %s", key)
		}
		config[key] = *value
	}
	return knowledge.VectorStore{ID: row.ID, TenantID: row.TenantID, Name: row.Name, Provider: row.Provider, Config: config, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}

func (vectorRow) TableName() string { return "knowledge_vector_stores" }

type baseRow struct {
	ID, TenantID, CreatorID, Name, Description                 string
	EmbeddingModelID, VectorStoreID                            *string
	IndexingMode, RetrievalMode, ParserEndpoint, ChunkStrategy string
	ChunkSize, ChunkOverlap                                    int
	DocumentCount, ChunkCount, ProcessingCount                 int64 `gorm:"->;-:migration"`
	CreatedAt, UpdatedAt                                       time.Time
}

func (baseRow) TableName() string { return "knowledge_bases" }
func (m baseRow) view() knowledge.Base {
	b := knowledge.Base{ID: m.ID, TenantID: m.TenantID, CreatorID: m.CreatorID, Name: m.Name, Description: m.Description, IndexingMode: m.IndexingMode, RetrievalMode: m.RetrievalMode, ChunkStrategy: m.ChunkStrategy, ParserEndpoint: m.ParserEndpoint, ChunkSize: m.ChunkSize, ChunkOverlap: m.ChunkOverlap, DocumentCount: m.DocumentCount, ChunkCount: m.ChunkCount, ProcessingCount: m.ProcessingCount, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
	if m.EmbeddingModelID != nil {
		b.EmbeddingModelID = *m.EmbeddingModelID
	}
	if m.VectorStoreID != nil {
		b.VectorStoreID = *m.VectorStoreID
	}
	return b
}
func baseRecord(b knowledge.Base) baseRow {
	m := baseRow{ID: b.ID, TenantID: b.TenantID, CreatorID: b.CreatorID, Name: b.Name, Description: b.Description, IndexingMode: b.IndexingMode, RetrievalMode: b.RetrievalMode, ChunkStrategy: b.ChunkStrategy, ParserEndpoint: b.ParserEndpoint, ChunkSize: b.ChunkSize, ChunkOverlap: b.ChunkOverlap, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt}
	if b.EmbeddingModelID != "" {
		m.EmbeddingModelID = &b.EmbeddingModelID
	}
	if b.VectorStoreID != "" {
		m.VectorStoreID = &b.VectorStoreID
	}
	return m
}

type documentRow struct {
	ID, TenantID, BaseID, Title, Source, FileName, FileType, ObjectKey, FileHash string
	Content, ParseStatus, ErrorMessage, Attempt, CleanupTargets                  string
	IndexStoreID                                                                 *string
	UseChunks                                                                    bool
	FileSize, ChunkCount                                                         int64
	LeaseUntil                                                                   *time.Time
	CreatedAt, UpdatedAt                                                         time.Time
}

type cleanupRow struct {
	StoreID string `json:"store_id"`
	Attempt string `json:"attempt"`
}

func (m documentRow) view() (knowledge.Document, error) {
	d := knowledge.Document{ID: m.ID, TenantID: m.TenantID, BaseID: m.BaseID, Title: m.Title, Source: m.Source, FileName: m.FileName, FileType: m.FileType, ObjectKey: m.ObjectKey, FileHash: m.FileHash, Content: m.Content, ParseStatus: m.ParseStatus, ErrorMessage: m.ErrorMessage, Attempt: m.Attempt, UseChunks: m.UseChunks, FileSize: m.FileSize, ChunkCount: m.ChunkCount, LeaseUntil: m.LeaseUntil, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
	if m.IndexStoreID != nil {
		d.IndexStoreID = *m.IndexStoreID
	}
	var targets []cleanupRow
	if m.CleanupTargets != "" {
		if err := json.Unmarshal([]byte(m.CleanupTargets), &targets); err != nil {
			return knowledge.Document{}, err
		}
	}
	for _, t := range targets {
		d.CleanupTargets = append(d.CleanupTargets, knowledge.CleanupTarget{StoreID: t.StoreID, Attempt: t.Attempt})
	}
	return d, nil
}

func documentRecord(d knowledge.Document) (documentRow, error) {
	m := documentRow{ID: d.ID, TenantID: d.TenantID, BaseID: d.BaseID, Title: d.Title, Source: d.Source, FileName: d.FileName, FileType: d.FileType, ObjectKey: d.ObjectKey, FileHash: d.FileHash, Content: d.Content, ParseStatus: d.ParseStatus, ErrorMessage: d.ErrorMessage, Attempt: d.Attempt, UseChunks: d.UseChunks, FileSize: d.FileSize, ChunkCount: d.ChunkCount, LeaseUntil: d.LeaseUntil, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt}
	if d.IndexStoreID != "" {
		m.IndexStoreID = &d.IndexStoreID
	}
	targets := make([]cleanupRow, 0, len(d.CleanupTargets))
	for _, t := range d.CleanupTargets {
		targets = append(targets, cleanupRow{StoreID: t.StoreID, Attempt: t.Attempt})
	}
	data, err := json.Marshal(targets)
	if err != nil {
		return documentRow{}, err
	}
	m.CleanupTargets = string(data)
	return m, nil
}

func (documentRow) TableName() string { return "knowledge_documents" }

type chunkRow knowledge.Chunk

func (chunkRow) TableName() string { return "knowledge_chunks" }
