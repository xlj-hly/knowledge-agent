// Package port 声明 SaaS 用例需要的外部能力，不依赖 ORM、HTTP 或具体平台实现。
package port

import (
	"context"

	"go-server/internal/domain/saas"
)

// Persistence 提供连接视图和事务边界，事务回调内的 Store 不得在回调结束后使用。
type Persistence interface {
	WithContext(context.Context) domain.Store
	Transaction(context.Context, func(domain.Store) error) error
	Ping(context.Context) error
	CleanupSessions(context.Context) error
}
