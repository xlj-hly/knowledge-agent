package usecase

import (
	"context"
	"errors"

	"go-server/internal/app/saas/dto"
	"go-server/internal/domain/saas"
	"golang.org/x/crypto/bcrypt"
)

var dummyHash = []byte("$2a$10$nqlz0EjvSsEhWiFDHBXS4e5Tu90TQwRO9ftWJLIbzyyvCbp7u0EMm")

func credentialsMissing(err error) bool {
	var business *domain.Error
	return errors.As(err, &business) && business.Code == "not_found"
}

// Tenant 按域名返回正常企业，不要求登录；停用企业不可通过此入口访问。
func (s *Auth) Tenant(ctx context.Context, id dto.TenantIdentity) (out dto.Tenant, err error) {
	err = s.tenant(ctx, id, domain.Shared, domain.Shared, false, false, func(_ domain.Store, state tenantState) error { out = tenantDTO(state.tenant); return nil })
	return
}

// Me 校验企业令牌、账号状态和归属，返回公开资料，不重新签发令牌。
func (s *Auth) Me(ctx context.Context, id dto.TenantIdentity) (dto.TenantSession, error) {
	return s.me(ctx, id)
}

func (s *runtime) me(ctx context.Context, id dto.TenantIdentity) (out dto.TenantSession, err error) {
	err = s.tenant(ctx, id, domain.Shared, domain.Shared, true, false, func(_ domain.Store, state tenantState) error {
		out = dto.TenantSession{User: userDTO(state.account.User), Tenant: tenantDTO(state.tenant)}
		return nil
	})
	return
}

// Login 在锁外验证密码，再于企业和账号共享锁下复核凭据后签发会话。
func (s *Auth) Login(ctx context.Context, id dto.TenantIdentity, in dto.LoginInput) (out dto.TenantSession, err error) {
	name, err := domain.NormalizeUsername(in.Username)
	if err != nil || domain.ValidatePassword(in.Password) != nil {
		return out, domain.ErrCredentials
	}
	host, err := domain.NormalizeHostname(id.Hostname)
	if err != nil {
		return out, err
	}
	store := s.store.WithContext(ctx)
	tenant, err := store.TenantByHost(host, domain.Unlocked)
	if err != nil {
		return out, err
	}
	snapshot, err := store.UserByName(tenant.ID, name, domain.Unlocked)
	if credentialsMissing(err) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(in.Password))
		return out, domain.ErrCredentials
	}
	if err != nil {
		return out, err
	}
	if snapshot.PasswordHash == "" {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(in.Password))
		return out, domain.ErrCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(snapshot.PasswordHash), []byte(in.Password)) != nil {
		return out, domain.ErrCredentials
	}
	err = s.tenant(ctx, id, domain.Shared, domain.Shared, false, false, func(tx domain.Store, state tenantState) error {
		account, err := tx.UserByName(state.tenant.ID, name, domain.Shared)
		if credentialsMissing(err) {
			return domain.ErrCredentials
		}
		if err != nil {
			return err
		}
		if account.PasswordHash != snapshot.PasswordHash {
			return domain.ErrCredentials
		}
		if err := account.User.RequireActive(); err != nil {
			return err
		}
		token, expires, err := s.issue(tx, "tenant", account.User.ID, state.tenant.ID)
		out = dto.TenantSession{Token: token, ExpiresAt: expires, User: userDTO(account.User), Tenant: tenantDTO(state.tenant)}
		return err
	})
	return
}

// Logout 校验当前企业身份并撤销本次令牌，不影响该账号的其他会话。
func (s *Auth) Logout(ctx context.Context, id dto.TenantIdentity) (dto.OK, error) {
	err := s.tenant(ctx, id, domain.Shared, domain.Shared, true, false, func(tx domain.Store, state tenantState) error { return tx.RevokeToken(state.tokenHash) })
	return dto.OK{OK: true}, err
}

// PlatformLogin 仅使用独立平台凭据登录，不接受企业账号；锁后复核密码快照。
func (s *Auth) PlatformLogin(ctx context.Context, in dto.LoginInput) (out dto.PlatformSession, err error) {
	name, err := domain.NormalizeUsername(in.Username)
	if err != nil || domain.ValidatePassword(in.Password) != nil {
		return out, domain.ErrCredentials
	}
	snapshot, err := s.store.WithContext(ctx).AdminByName(name, domain.Unlocked)
	if credentialsMissing(err) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(in.Password))
		return out, domain.ErrCredentials
	}
	if err != nil {
		return out, err
	}
	if bcrypt.CompareHashAndPassword([]byte(snapshot.PasswordHash), []byte(in.Password)) != nil {
		return out, domain.ErrCredentials
	}
	err = s.store.Transaction(ctx, func(tx domain.Store) error {
		account, err := tx.AdminByName(name, domain.Shared)
		if err != nil {
			return err
		}
		if account.PasswordHash != snapshot.PasswordHash {
			return domain.ErrCredentials
		}
		token, expires, err := s.issue(tx, "platform", account.Admin.ID, "")
		out = dto.PlatformSession{Token: token, ExpiresAt: expires, Admin: adminDTO(account.Admin)}
		return err
	})
	return
}

// PlatformMe 校验独立平台令牌，取得账号锁后按当前时钟再次检查会话有效期。
func (s *Auth) PlatformMe(ctx context.Context, token string) (out dto.PlatformSession, err error) {
	err = s.platform(ctx, token, domain.Shared, func(_ domain.Store, state platformState) error { out.Admin = adminDTO(state.account.Admin); return nil })
	return
}

// PlatformLogout 撤销当前平台令牌，不影响企业会话。
func (s *Auth) PlatformLogout(ctx context.Context, token string) (dto.OK, error) {
	err := s.platform(ctx, token, domain.Shared, func(tx domain.Store, state platformState) error { return tx.RevokeToken(state.tokenHash) })
	return dto.OK{OK: true}, err
}
