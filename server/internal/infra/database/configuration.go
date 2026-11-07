package database

import (
	"context"
	"errors"
	"gorm.io/gorm"
)

type configurationKey struct{}
type configurationUnit struct {
	tenant string
	db     *gorm.DB
}

// Connection 返回当前配置工作单元的连接；普通操作继续使用调用者传入的连接。
func Connection(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if u, ok := ctx.Value(configurationKey{}).(configurationUnit); ok {
		return u.db
	}
	return fallback
}

// InTransaction 标记配置工作单元，避免仓储重复建立嵌套事务。
func InTransaction(ctx context.Context) bool {
	_, ok := ctx.Value(configurationKey{}).(configurationUnit)
	return ok
}

// ConfigGuard 使用事务级企业锁保持跨模块逻辑引用一致，不需要数据库外键。
type ConfigGuard struct{ db *gorm.DB }

func NewConfigGuard(db *gorm.DB) *ConfigGuard { return &ConfigGuard{db: db} }
func (g *ConfigGuard) Within(ctx context.Context, tenant string, fn func(context.Context) error) error {
	if u, ok := ctx.Value(configurationKey{}).(configurationUnit); ok {
		if u.tenant != tenant {
			return errors.New("configuration transaction tenant mismatch")
		}
		return fn(ctx)
	}
	return g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", tenant).Error; err != nil {
			return errors.New("configuration lock unavailable")
		}
		return fn(context.WithValue(ctx, configurationKey{}, configurationUnit{tenant, tx}))
	})
}
