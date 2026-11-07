package bootstrap

import (
	"context"
	"fmt"
	"net/http"

	aiusecase "go-server/internal/app/ai/usecase"
	chatusecase "go-server/internal/app/chat/usecase"
	knowledgeusecase "go-server/internal/app/knowledge/usecase"
	"go-server/internal/app/saas/port"
	"go-server/internal/app/saas/usecase"
	"go-server/internal/app/storage"
	aidriver "go-server/internal/infra/ai"
	"go-server/internal/infra/ai/presets"
	"go-server/internal/infra/database"
	knowledgedriver "go-server/internal/infra/knowledge"
	"go-server/internal/infra/persistence/saas"
	objectstorage "go-server/internal/infra/storage"
	"go-server/internal/infra/wechat"
	"go-server/internal/transport/http"
	"gorm.io/gorm"
)

// Application 拥有同一组用例和 HTTP 入口；调用方保留传入数据库的关闭责任。
type Application struct {
	Handler   http.Handler
	Services  usecase.Services
	Knowledge *knowledgeusecase.Service
	AI        *aiusecase.Service
	Chat      *chatusecase.Service
}

type dependencies struct {
	wechat  port.WeChatProvider
	objects storage.Store
}

// Option 为集成环境显式替换外部能力，不改变业务规则或存储事务实现。
type Option func(*dependencies) error

// WithWeChatProvider 注入身份验证能力；测试替身仍贯通真实业务和数据库。
func WithWeChatProvider(provider port.WeChatProvider) Option {
	return func(deps *dependencies) error {
		if provider == nil {
			return fmt.Errorf("微信身份验证依赖不能为空")
		}
		deps.wechat = provider
		return nil
	}
}

// WithObjectStorage 注入统一存储；头像规则、归属校验和资料提交仍执行真实用例。
func WithObjectStorage(store storage.Store) Option {
	return func(deps *dependencies) error {
		if store == nil {
			return fmt.Errorf("对象存储依赖不能为空")
		}
		deps.objects = store
		return nil
	}
}

// New 是运行时依赖的唯一组装入口，不执行建表、连接池关闭或隐式目录创建。
func New(db *gorm.DB, cfg Config, options ...Option) (*Application, error) {
	if db == nil {
		return nil, fmt.Errorf("数据库依赖不能为空")
	}
	cfg, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	var external dependencies
	for _, option := range options {
		if option == nil {
			return nil, fmt.Errorf("应用依赖选项不能为空")
		}
		if err := option(&external); err != nil {
			return nil, err
		}
	}
	// 同一只读目录贯通配置校验与实际调用；启动时拒绝无效注册，不在请求中降级。
	registry, err := aidriver.NewBuiltinRegistry()
	if err != nil {
		return nil, fmt.Errorf("创建模型注册目录: %w", err)
	}
	templates, err := presets.NewBuiltin()
	if err != nil {
		return nil, fmt.Errorf("创建产品预设目录: %w", err)
	}
	vectors, err := knowledgedriver.NewBuiltinVectors()
	if err != nil {
		return nil, fmt.Errorf("创建向量引擎注册目录: %w", err)
	}
	if external.wechat == nil {
		external.wechat = wechat.NewProvider(wechat.New())
	}
	if external.objects == nil {
		external.objects, err = objectstorage.New(cfg.Storage)
		if err != nil {
			return nil, err
		}
	}
	services := usecase.New(usecase.Dependencies{Persistence: repository.New(db), WeChat: external.wechat, Objects: external.objects}, usecase.Config{SessionTTL: cfg.SessionTTL, PlatformHosts: cfg.PlatformHosts})
	guard := database.NewConfigGuard(db)
	refs := &aiReferences{}
	ai := assembleAI(db, services.Auth, refs, guard, registry, templates)
	knowledge := assembleKnowledge(db, services.Auth, external.objects, ai, guard, registry, vectors)
	chat := assembleChat(db, services.Auth, ai, knowledge, guard, registry)
	refs.knowledge = knowledge
	handler := httpapi.New(httpapi.Dependencies{Auth: services.Auth, Accounts: services.Accounts, Companies: services.Companies, Sites: services.Sites, Maintenance: services.Maintenance, Knowledge: knowledge, AI: ai, Chat: chat}, httpapi.Config{AllowedOrigins: cfg.AllowedOrigins})
	return &Application{Handler: handler, Services: services, Knowledge: knowledge, AI: ai, Chat: chat}, nil
}

// Bootstrap 组装初始化用例，不覆盖已有账号，也不启动 HTTP 或其他外部能力。
func Bootstrap(ctx context.Context, db *gorm.DB, username, password string) error {
	if db == nil {
		return fmt.Errorf("数据库依赖不能为空")
	}
	return usecase.NewInitializer(repository.New(db)).Bootstrap(ctx, username, password)
}
