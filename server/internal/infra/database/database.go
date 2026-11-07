// Package database 管理 PostgreSQL 连接及业务表结构的就绪检查。
package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open 创建 GORM 连接并验证数据库可达；调用方负责关闭底层 SQL 连接池。
// 表结构由独立 SQL 建表文件管理，此处不执行 AutoMigrate 或创建关联约束。
func Open(ctx context.Context, dsn string) (*gorm.DB, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("DATABASE_URL 格式无效")
	}
	cfg.RuntimeParams["statement_timeout"] = "10000"
	cfg.RuntimeParams["lock_timeout"] = "5000"
	pool := stdlib.OpenDB(*cfg)
	pool.SetMaxOpenConns(10)
	pool.SetMaxIdleConns(10)
	pool.SetConnMaxIdleTime(5 * time.Minute)
	db, err := Wrap(pool)
	if err != nil {
		_ = pool.Close()
		return nil, fmt.Errorf("数据库连接配置无效")
	}
	if err := pool.PingContext(ctx); err != nil {
		_ = pool.Close()
		return nil, fmt.Errorf("数据库连接失败: %w", err)
	}
	return db, nil
}

// Wrap 将调用方拥有的 SQL 连接池适配为 GORM，也供隔离数据库测试使用。
// 此函数不接管连接池清理责任，也不自动 Ping，调用方按上下文验证可达性。
func Wrap(pool *sql.DB) (*gorm.DB, error) {
	// SQL 文本和绑定参数可能包含敏感值，因此完全关闭 GORM SQL 日志。
	return gorm.Open(postgres.New(postgres.Config{Conn: pool}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true, DisableAutomaticPing: true})
}

// Ready 核对已实现业务的表是否存在；只检查，不执行建表。
func Ready(ctx context.Context, db *gorm.DB) error {
	var ready bool
	// 仅用 PostgreSQL 目录函数检查表是否已创建，不在运行时修改表结构。
	if err := db.WithContext(ctx).Raw(`SELECT to_regclass('company') IS NOT NULL AND to_regclass('users') IS NOT NULL
		AND to_regclass('auth_sessions') IS NOT NULL AND to_regclass('super_admin') IS NOT NULL
		AND to_regclass('ai_models') IS NOT NULL AND to_regclass('ai_settings') IS NOT NULL
		AND to_regclass('chat_sessions') IS NOT NULL AND to_regclass('chat_messages') IS NOT NULL AND to_regclass('knowledge_vector_stores') IS NOT NULL
		AND to_regclass('knowledge_bases') IS NOT NULL AND to_regclass('knowledge_documents') IS NOT NULL
		AND to_regclass('knowledge_chunks') IS NOT NULL`).Scan(&ready).Error; err != nil {
		return fmt.Errorf("check schema: %w", err)
	}
	if !ready {
		return fmt.Errorf("请先执行 db/saas.sql")
	}
	return nil
}
