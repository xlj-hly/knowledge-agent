package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"

	"go-server/internal/app/saas/dto"
	"go-server/internal/app/saas/port"
	"go-server/internal/domain/saas"
	"golang.org/x/crypto/bcrypt"
)

var wechatCode = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)

type verifiedWeChat struct {
	tenantID      string
	configuration domain.WeChatConfiguration
	identity      port.WeChatIdentity
}

// verifyWeChat 读取租户配置快照并在事务外交换code，返回结果须在写事务内复核。
func (s *runtime) verifyWeChat(ctx context.Context, hostname, code string) (out verifiedWeChat, err error) {
	if !wechatCode.MatchString(code) {
		return out, domain.Invalid("微信登录凭证无效，请重试")
	}
	host, err := domain.NormalizeHostname(hostname)
	if err != nil {
		return out, err
	}
	if s.isPlatformHost(host) {
		return out, domain.Forbidden()
	}
	store := s.store.WithContext(ctx)
	tenant, err := store.TenantByHost(host, domain.Unlocked)
	if err != nil {
		return out, err
	}
	if err := tenant.RequireActive(); err != nil {
		return out, err
	}
	configuration, err := store.WeChatConfig(tenant.ID)
	if credentialsMissing(err) {
		return out, &domain.Error{Code: "wechat_not_configured", Message: "企业尚未配置微信登录，请联系管理员"}
	}
	if err != nil {
		return out, err
	}
	if s.wechatProvider == nil {
		return out, &domain.Error{Code: "wechat_unavailable", Message: "微信登录暂时不可用，请稍后重试"}
	}
	credentials := port.WeChatCredentials{AppID: configuration.AppID, AppSecret: configuration.AppSecret}
	// 慢上游不占用企业行锁，避免阻塞管理员轮换凭据或停用企业。
	identity, err := s.wechatProvider.Exchange(ctx, credentials, code)
	if err != nil {
		return out, wechatFailure(err)
	}
	if identity.AppID != credentials.AppID || identity.OpenID == "" || len(identity.OpenID) > 128 {
		return out, &domain.Error{Code: "wechat_upstream_error", Message: "微信登录响应无效，请稍后重试"}
	}
	return verifiedWeChat{tenantID: tenant.ID, configuration: configuration, identity: identity}, nil
}

func revalidateWeChat(tx domain.Store, tenantID string, snapshot verifiedWeChat) error {
	// 上游期间域名可能换绑、凭据可能轮换，旧身份验证结果不得提交到新配置。
	if tenantID != snapshot.tenantID {
		return domain.Conflict("微信配置已更新，请重新登录")
	}
	current, err := tx.WeChatConfig(tenantID)
	if credentialsMissing(err) {
		return domain.Conflict("微信配置已更新，请重新登录")
	}
	if err != nil {
		return err
	}
	if current.Version != snapshot.configuration.Version || current.AppID != snapshot.configuration.AppID {
		return domain.Conflict("微信配置已更新，请重新登录")
	}
	return nil
}

// WeChatLogin 将微信凭据换为企业会话；首次创建本企业普通成员，后续复用同一账号。
func (s *Auth) WeChatLogin(ctx context.Context, id dto.TenantIdentity, in dto.WeChatLoginInput) (out dto.TenantSession, err error) {
	snapshot, err := s.verifyWeChat(ctx, id.Hostname, in.Code)
	if err != nil {
		return out, err
	}
	// 每账号独立随机初始密码，不使用可推测的OpenID片段，也不向客户端公开。
	var initial [32]byte
	if _, err := rand.Read(initial[:]); err != nil {
		return out, err
	}
	defaultHash, err := bcrypt.GenerateFromPassword([]byte(hex.EncodeToString(initial[:])), bcrypt.DefaultCost)
	if err != nil {
		return out, err
	}
	identity := snapshot.identity
	err = s.tenant(ctx, id, domain.Shared, domain.Shared, false, false, func(tx domain.Store, state tenantState) error {
		if err := revalidateWeChat(tx, state.tenant.ID, snapshot); err != nil {
			return err
		}
		if err := tx.LockWeChatIdentity(state.tenant.ID, identity.AppID, identity.OpenID); err != nil {
			return err
		}
		userID, err := tx.WeChatUserID(state.tenant.ID, identity.AppID, identity.OpenID)
		var user domain.User
		if credentialsMissing(err) {
			var suffix [16]byte
			if _, err := rand.Read(suffix[:]); err != nil {
				return err
			}
			user, err = tx.CreateWeChatUser(state.tenant.ID, "wx_"+hex.EncodeToString(suffix[:]), string(defaultHash), identity.AppID, identity.OpenID)
			if err != nil {
				return err
			}
		} else {
			if err != nil {
				return err
			}
			account, err := tx.UserByID(state.tenant.ID, userID, domain.Shared)
			if err != nil {
				return missingIdentity(err)
			}
			user = account.User
		}
		if err := user.RequireActive(); err != nil {
			return err
		}
		token, expires, err := s.issue(tx, "tenant", user.ID, state.tenant.ID)
		out = dto.TenantSession{Token: token, ExpiresAt: expires, User: userDTO(user), Tenant: tenantDTO(state.tenant)}
		return err
	})
	return
}

// ChangeWeChatPassword 必须同时持有有效企业会话和属于该账号的当前微信身份。
// 修改完成撤销全部会话；普通Web改密继续使用校验当前密码的独立用例。
func (s *Accounts) ChangeWeChatPassword(ctx context.Context, id dto.TenantIdentity, in dto.WeChatPasswordInput) (dto.OK, error) {
	if err := domain.ValidatePassword(in.New); err != nil {
		return dto.OK{}, err
	}
	account, err := s.me(ctx, id)
	if err != nil {
		return dto.OK{}, err
	}
	snapshot, err := s.verifyWeChat(ctx, id.Hostname, in.Code)
	if err != nil {
		return dto.OK{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.New), bcrypt.DefaultCost)
	if err != nil {
		return dto.OK{}, err
	}
	err = s.tenant(ctx, id, domain.Shared, domain.Exclusive, true, false, func(tx domain.Store, state tenantState) error {
		if state.tenant.ID != account.Tenant.ID || state.account.User.ID != account.User.ID {
			return domain.Unauthorized()
		}
		if err := revalidateWeChat(tx, state.tenant.ID, snapshot); err != nil {
			return err
		}
		mapped, err := tx.WeChatUserID(state.tenant.ID, snapshot.identity.AppID, snapshot.identity.OpenID)
		if credentialsMissing(err) {
			return domain.Forbidden()
		}
		if err != nil {
			return err
		}
		if mapped != state.account.User.ID {
			return domain.Forbidden()
		}
		encodedHash := string(hash)
		if _, err := tx.UpdateUser(state.tenant.ID, state.account.User.ID, domain.UserChanges{PasswordHash: &encodedHash}); err != nil {
			return err
		}
		return tx.RevokeUser(state.tenant.ID, state.account.User.ID)
	})
	return dto.OK{OK: true}, err
}

func wechatFailure(err error) error {
	var upstream *port.WeChatError
	if errors.As(err, &upstream) {
		switch upstream.Code {
		case "invalid_code":
			return &domain.Error{Code: "wechat_invalid_code", Message: "微信登录凭证已失效，请重新登录"}
		case "blocked":
			return &domain.Error{Code: "wechat_login_blocked", Message: "当前微信账号无法完成登录，请使用其他账号或联系管理员"}
		case "rate_limited":
			return &domain.Error{Code: "wechat_rate_limited", Message: "微信登录尝试过于频繁，请稍后重试"}
		case "unavailable":
			return &domain.Error{Code: "wechat_unavailable", Message: "微信登录暂时不可用，请稍后重试"}
		}
	}
	return &domain.Error{Code: "wechat_upstream_error", Message: "微信登录服务异常，请稍后重试"}
}
