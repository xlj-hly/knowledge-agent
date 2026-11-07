package knowledge

import (
	"context"
	"time"
)

// Repository 限定知识库持久化范围；所有面向用户的读取显式包含企业归属。
// Transaction 内的写锁只覆盖本地状态校验与提交，不能持有锁调用外部服务。
type Repository interface {
	Transaction(context.Context, func(Repository) error) error
	ModelReferences(context.Context, string, string) (int64, error)
	VectorStores(context.Context, string) ([]VectorStore, error)
	VectorStore(context.Context, string, string, bool) (VectorStore, error)
	SaveVectorStore(context.Context, VectorStore, bool) error
	DeleteVectorStore(context.Context, string, string) error
	VectorStoreReferences(context.Context, string, string) (int64, error)
	Bases(context.Context, string) ([]Base, error)
	Base(context.Context, string, string, bool) (Base, error)
	SaveBase(context.Context, Base, bool) error
	DeleteBase(context.Context, string, string) error
	Documents(context.Context, string, string) ([]Document, error)
	Document(context.Context, string, string, bool) (Document, error)
	SaveDocument(context.Context, Document, bool) error
	DeleteDocument(context.Context, string, string) error
	Chunks(context.Context, string, string) ([]Chunk, error)
	Chunk(context.Context, string, string) (Chunk, error)
	SaveChunk(context.Context, Chunk) error
	Claim(context.Context, time.Time, time.Time, string) (Document, error)
	Complete(context.Context, Document, []Chunk) (bool, error)
	Fail(context.Context, Document, string) error
	KeywordSearch(context.Context, string, string, string, int) ([]Hit, error)
	CurrentHits(context.Context, string, string, []string) ([]Hit, error)
}
