package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"go-server/internal/infra/database"
)

// Run 拥有进程配置、数据库连接和服务生命周期；ctx 由命令入口取消。
func Run(ctx context.Context, args []string) error {
	if len(args) > 1 || len(args) == 1 && args[0] != "bootstrap" {
		return fmt.Errorf("未知命令，仅支持 bootstrap")
	}
	if err := LoadEnvironment(); err != nil {
		return err
	}
	// 初始化命令只需要数据库凭据，不要求企业入口或头像存储配置。
	if len(args) == 1 {
		return runBootstrap(ctx)
	}
	cfg, err := LoadConfig(os.Getenv)
	if err != nil {
		return err
	}
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	db, err := database.Open(check, cfg.DatabaseURL)
	cancel()
	if err != nil {
		return err
	}
	pool, err := db.DB()
	if err != nil {
		return fmt.Errorf("数据库连接不可用")
	}
	defer pool.Close()
	check, cancel = context.WithTimeout(ctx, 5*time.Second)
	err = database.Ready(check, db)
	cancel()
	if err != nil {
		return err
	}
	gin.SetMode(gin.ReleaseMode)
	application, err := New(db, cfg)
	if err != nil {
		return err
	}
	server := &http.Server{Addr: cfg.ListenAddr, Handler: application.Handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	return serve(ctx, server, application.Services.Maintenance.CleanupSessions)
}

func runBootstrap(ctx context.Context) error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("请设置 DATABASE_URL")
	}
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	db, err := database.Open(check, dsn)
	cancel()
	if err != nil {
		return err
	}
	pool, err := db.DB()
	if err != nil {
		return fmt.Errorf("数据库连接不可用")
	}
	defer pool.Close()
	if err := Bootstrap(ctx, db, os.Getenv("BOOTSTRAP_USERNAME"), os.Getenv("BOOTSTRAP_PASSWORD")); err != nil {
		return fmt.Errorf("平台初始化失败：%w", err)
	}
	slog.Info("平台管理员已创建")
	return nil
}

func serve(ctx context.Context, server *http.Server, cleanup func(context.Context) error) error {
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("SaaS API 已启动", "address", server.Addr)
		serverErr <- server.ListenAndServe()
	}()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := server.Shutdown(shutdown)
			cancel()
			if err != nil {
				return errors.Join(err, server.Close())
			}
			return nil
		case err := <-serverErr:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-ticker.C:
			maintenance, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := cleanup(maintenance)
			cancel()
			if err != nil {
				logCleanupFailure(err)
			}
		}
	}
}

func logCleanupFailure(err error) {
	// 驱动错误可能包含绑定值，只记录操作和可公开的诊断字段。
	var state interface{ SQLState() string }
	sqlState, cause := "", "infrastructure"
	if errors.As(err, &state) {
		sqlState = state.SQLState()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		cause = "deadline_exceeded"
	} else if errors.Is(err, context.Canceled) {
		cause = "canceled"
	}
	slog.Warn("过期会话清理失败", "operation", "clean expired sessions", "cause", cause, "sqlstate", sqlState)
}
