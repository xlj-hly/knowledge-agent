// Package usecase 编排知识库、文档处理与检索，不依赖 HTTP 或具体模型服务。
package usecase

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"go-server/internal/app/configuration"
	"go-server/internal/app/knowledge/dto"
	"go-server/internal/app/knowledge/port"
	"go-server/internal/app/storage"
	"go-server/internal/domain/knowledge"
	domain "go-server/internal/domain/saas"
	"strconv"
	"time"
)

// Dependencies 由 bootstrap 组装可替换的存储、解析、模型和向量实现。
type Dependencies struct {
	Repository     knowledge.Repository
	Authorizer     port.Authorizer
	Objects        storage.Store
	Processor      port.Processor
	Embedder       port.Embedder
	Vectors        port.Vectors
	Catalog        port.VectorCatalog
	Models         port.Models
	Configurations configuration.Guard
}

// Service 提供当前内聚的知识库用例，后台生命周期由 bootstrap 管理。
type Service struct{ deps Dependencies }

// New 注入已有能力，不启动后台任务，也不在业务层创建具体适配器。
func New(deps Dependencies) *Service { return &Service{deps: deps} }

func (s *Service) withConfiguration(ctx context.Context, tenant string, fn func(context.Context) error) error {
	// 单元测试可只注入被验收的依赖；运行时由 bootstrap 强制提供共享配置事务。
	if s.deps.Configurations == nil {
		return fn(ctx)
	}
	return s.deps.Configurations.Within(ctx, tenant, fn)
}
func (s *Service) actor(ctx context.Context, in dto.Identity, admin bool) (port.Actor, error) {
	a, err := s.deps.Authorizer.Authorize(ctx, in.Hostname, in.Token)
	if err != nil {
		return a, err
	}
	if admin && a.Role != "admin" {
		return a, domain.Forbidden()
	}
	return a, nil
}
func newID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	n := binary.BigEndian.Uint64(b[:]) & 0x7fffffffffffffff
	if n == 0 {
		n = 1
	}
	return strconv.FormatUint(n, 10), nil
}
func attempt() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func validID(id string) error { _, err := domain.ParseID(id); return err }
func now() time.Time          { return time.Now().UTC() }
func baseView(b knowledge.Base) dto.Base {
	return dto.Base{ID: b.ID, Name: b.Name, Description: b.Description, IndexingMode: b.IndexingMode, RetrievalMode: b.RetrievalMode, ChunkStrategy: b.ChunkStrategy, EmbeddingModelID: b.EmbeddingModelID, VectorStoreID: b.VectorStoreID, ParserEndpoint: b.ParserEndpoint, ChunkSize: b.ChunkSize, ChunkOverlap: b.ChunkOverlap, DocumentCount: b.DocumentCount, ChunkCount: b.ChunkCount, ProcessingCount: b.ProcessingCount, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt}
}
func documentView(d knowledge.Document, content bool) dto.Document {
	out := dto.Document{ID: d.ID, BaseID: d.BaseID, Title: d.Title, Source: d.Source, FileName: d.FileName, FileType: d.FileType, FileSize: d.FileSize, ParseStatus: d.ParseStatus, ErrorMessage: d.ErrorMessage, ChunkCount: d.ChunkCount, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt}
	if content {
		out.Content = d.Content
	}
	return out
}
func chunkView(c knowledge.Chunk) dto.Chunk {
	return dto.Chunk{ID: c.ID, DocumentID: c.DocumentID, Content: c.Content, ContextHeader: c.ContextHeader, Index: c.Index, Start: c.Start, End: c.End}
}
