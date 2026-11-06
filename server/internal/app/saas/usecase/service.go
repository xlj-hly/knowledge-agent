// Package usecase 管理 SaaS 用例、身份校验及事务与并发不变量。
package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"go-server/internal/app/saas/dto"
	"go-server/internal/app/saas/port"
	"go-server/internal/app/storage"
	"go-server/internal/domain/saas"
	"golang.org/x/crypto/bcrypt"
)

// Dependencies 由应用入口注入业务所需能力，不在业务层创建基础设施。
type Dependencies struct {
	Persistence port.Persistence
	WeChat      port.WeChatProvider
	Objects     storage.Store
}

// Services 按业务用例分组，共享身份与事务规则，不互相调用公开用例。
type Services struct {
	Auth        *Auth
	Accounts    *Accounts
	Companies   *Companies
	Sites       *Sites
	Maintenance *Maintenance
	Initializer *Initializer
}

// Auth 处理企业与平台身份的认证和会话。
type Auth struct{ *runtime }

// Accounts 处理账号资料、权限、密码与头像。
type Accounts struct{ *runtime }

// Companies 处理企业生命周期、首个管理员与微信配置。
type Companies struct{ *runtime }

// Sites 解析请求域名对应的站点身份。
type Sites struct{ *runtime }

// Maintenance 提供健康检查与过期会话清理。
type Maintenance struct{ *runtime }

// Initializer 仅拥有平台首次初始化所需的持久化能力。
type Initializer struct{ store port.Persistence }

// runtime 共享身份、事务和签发规则；不向调用方暴露跨能力的万能服务。
type runtime struct {
	store          port.Persistence
	sessionTTL     time.Duration
	wechatProvider port.WeChatProvider
	platformHosts  map[string]bool
	objects        storage.Store
}

// Config 只保存业务运行参数；部署配置和适配器校验由应用入口负责。
type Config struct {
	SessionTTL    time.Duration
	PlatformHosts []string
}

// New 组装各业务用例；非正会话期限使用七天，依赖生命周期由调用方管理。
func New(deps Dependencies, cfg Config) Services {
	ttl := cfg.SessionTTL
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	hosts := make(map[string]bool, len(cfg.PlatformHosts))
	for _, value := range cfg.PlatformHosts {
		if host, err := domain.NormalizeHostname(value); err == nil {
			hosts[host] = true
		}
	}
	shared := &runtime{store: deps.Persistence, sessionTTL: ttl, wechatProvider: deps.WeChat, platformHosts: hosts, objects: deps.Objects}
	return Services{
		Auth: &Auth{shared}, Accounts: &Accounts{shared}, Companies: &Companies{shared},
		Sites: &Sites{shared}, Maintenance: &Maintenance{shared}, Initializer: NewInitializer(deps.Persistence),
	}
}

// NewInitializer 创建只依赖持久化的平台初始化入口。
func NewInitializer(persistence port.Persistence) *Initializer {
	return &Initializer{store: persistence}
}

type tenantState struct {
	tenant    domain.Tenant
	account   domain.Account
	tokenHash string
}
type platformState struct {
	account   domain.PlatformAccount
	tokenHash string
}

func tokenDigest(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
func missingIdentity(err error) error {
	var business *domain.Error
	if errors.As(err, &business) && business.Code == "not_found" {
		return domain.Unauthorized()
	}
	return err
}

// tenant 统一先锁企业，再锁当前账号；锁模式由具体用例的不变量决定。
// auth 为 false 时仅校验企业；回调失败时所有写入随事务回滚。
func (s *runtime) tenant(ctx context.Context, identity dto.TenantIdentity, tenantLock, userLock domain.Lock, auth, admin bool, fn func(domain.Store, tenantState) error) error {
	host, err := domain.NormalizeHostname(identity.Hostname)
	if err != nil {
		return err
	}
	if s.platformHosts[host] {
		return domain.Forbidden()
	}
	return s.store.Transaction(ctx, func(tx domain.Store) error {
		t, err := tx.TenantByHost(host, tenantLock)
		if err != nil {
			return err
		}
		if err := t.RequireActive(); err != nil {
			return err
		}
		state := tenantState{tenant: t, tokenHash: tokenDigest(identity.Token)}
		if auth {
			if identity.Token == "" {
				return domain.Unauthorized()
			}
			id, err := tx.SessionSubject(state.tokenHash, "tenant", t.ID)
			if err != nil {
				return err
			}
			state.account, err = tx.UserByID(t.ID, id, userLock)
			if err != nil {
				return missingIdentity(err)
			}
			// 等待账号锁期间，改密或状态变更可能已提交，必须在取得锁后重新检查会话。
			current, err := tx.SessionSubject(state.tokenHash, "tenant", t.ID)
			if err != nil {
				return err
			}
			if current != state.account.User.ID {
				return domain.Unauthorized()
			}
			if err := state.account.User.RequireActive(); err != nil {
				return err
			}
			if admin {
				if err := state.account.User.RequireAdmin(); err != nil {
					return err
				}
			}
		}
		return fn(tx, state)
	})
}

// platform 先锁独立平台身份，再允许回调访问企业，保持所有平台用例锁顺序一致。
func (s *runtime) platform(ctx context.Context, token string, lock domain.Lock, fn func(domain.Store, platformState) error) error {
	if token == "" {
		return domain.Unauthorized()
	}
	return s.store.Transaction(ctx, func(tx domain.Store) error {
		state := platformState{tokenHash: tokenDigest(token)}
		id, err := tx.SessionSubject(state.tokenHash, "platform", "")
		if err != nil {
			return err
		}
		state.account, err = tx.AdminByID(id, lock)
		if err != nil {
			return missingIdentity(err)
		}
		current, err := tx.SessionSubject(state.tokenHash, "platform", "")
		if err != nil {
			return err
		}
		if current != id {
			return domain.Unauthorized()
		}
		return fn(tx, state)
	})
}
func passwordHash(password string) (string, error) {
	if err := domain.ValidatePassword(password); err != nil {
		return "", err
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(h), err
}
func prepareUser(username, password, nickname, role string) (string, string, error) {
	name, err := domain.NormalizeUsername(username)
	if err != nil {
		return "", "", err
	}
	if role != "user" && role != "admin" {
		return "", "", domain.Invalid("账号角色无效")
	}
	if err := domain.ValidateNickname(nickname); err != nil {
		return "", "", err
	}
	hash, err := passwordHash(password)
	return name, hash, err
}
func (s *runtime) issue(tx domain.Store, scope, id, tenantID string) (string, *time.Time, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	expires := time.Now().UTC().Add(s.sessionTTL)
	err := tx.CreateSession(tokenDigest(token), scope, id, tenantID, expires)
	return token, &expires, err
}

// CleanupSessions 清理已到期会话；调用方负责定时触发、取消和错误记录。
func (s *Maintenance) CleanupSessions(ctx context.Context) error { return s.store.CleanupSessions(ctx) }

// Health 在指定上下文内检查数据库可达性，故障保持基础设施错误语义。
func (s *Maintenance) Health(ctx context.Context) (dto.OK, error) {
	return dto.OK{OK: true}, s.store.Ping(ctx)
}

// Bootstrap 仅创建首个平台账号，不覆盖已有凭据；调用方显式提供用户名和密码。
// 散列在事务外生成，事务内用 advisory lock 串行化首次创建。
func (s *Initializer) Bootstrap(ctx context.Context, username, password string) error {
	name, err := domain.NormalizeUsername(username)
	if err != nil {
		return err
	}
	hash, err := passwordHash(password)
	if err != nil {
		return err
	}
	return s.store.Transaction(ctx, func(tx domain.Store) error {
		if err := tx.BootstrapLock(); err != nil {
			return err
		}
		count, err := tx.AdminCount()
		if err != nil {
			return err
		}
		if count != 0 {
			return domain.Conflict("平台账号已初始化，请使用现有账号")
		}
		return tx.CreateAdmin(name, hash)
	})
}
