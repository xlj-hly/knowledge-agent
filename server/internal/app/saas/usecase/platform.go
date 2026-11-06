package usecase

import (
	"context"

	"go-server/internal/app/saas/dto"
	"go-server/internal/domain/saas"
)

// ListTenants 要求平台身份，按已验证条件搜索企业，不使用企业管理员权限。
func (s *Companies) ListTenants(ctx context.Context, token string, p dto.Pagination) (out dto.Page[dto.Tenant], err error) {
	if err := p.Validate(); err != nil {
		return out, err
	}
	err = s.platform(ctx, token, domain.Shared, func(tx domain.Store, _ platformState) error {
		page, err := tx.ListTenants(p.Domain())
		out = tenantsDTO(page)
		return err
	})
	return
}

// CreateTenant 要求平台身份，在同一事务创建企业与首个管理员，失败时不保留部分数据。
func (s *Companies) CreateTenant(ctx context.Context, token string, in dto.CreateTenantInput) (out dto.CreatedTenant, err error) {
	if err := domain.ValidateName(in.Name, 128, "企业名称"); err != nil {
		return out, err
	}
	host, err := domain.NormalizeHostname(in.Hostname)
	if err != nil {
		return out, err
	}
	if s.isPlatformHost(host) {
		return out, domain.Invalid("企业域名不能使用平台管理域名")
	}
	name, hash, err := prepareUser(in.AdminUsername, in.AdminPassword, "", "admin")
	if err != nil {
		return out, err
	}
	err = s.platform(ctx, token, domain.Shared, func(tx domain.Store, _ platformState) error {
		tenant, err := tx.CreateTenant(in.Name, host)
		if err != nil {
			return err
		}
		user, err := tx.CreateUser(tenant.ID, name, hash, "", "admin")
		out.Tenant = tenantDTO(tenant)
		out.User = userDTO(user)
		return err
	})
	return
}

// UpdateTenant 要求平台身份；企业停用或域名变化与撤销企业会话在同一事务完成。
func (s *Companies) UpdateTenant(ctx context.Context, token, id string, in dto.UpdateTenantInput) (out dto.Tenant, err error) {
	if _, err := domain.ParseID(id); err != nil {
		return out, err
	}
	changes, err := (domain.CompanyChanges{Name: in.Name, Hostname: in.Hostname, Status: in.Status, RegistrationEnabled: in.Registration}).Validated()
	if err != nil {
		return out, err
	}
	if changes.Hostname != nil && s.isPlatformHost(*changes.Hostname) {
		return out, domain.Invalid("企业域名不能使用平台管理域名")
	}
	// 统一按平台身份→企业→企业账号取得锁；企业侧请求不得反向依赖平台身份锁。
	err = s.platform(ctx, token, domain.Shared, func(tx domain.Store, _ platformState) error {
		old, err := tx.TenantByID(id, domain.Exclusive)
		if err != nil {
			return err
		}
		tenant, err := tx.UpdateTenant(id, changes)
		out = tenantDTO(tenant)
		if err != nil {
			return err
		}
		if old.RequiresSessionRevocation(tenant) {
			return tx.RevokeTenant(id)
		}
		return nil
	})
	return
}

// ResetAdminPassword 要求平台身份，仅重置指定企业已有管理员并撤销其会话。
func (s *Companies) ResetAdminPassword(ctx context.Context, token, id string, in dto.ResetAdminPasswordInput) (dto.OK, error) {
	if _, err := domain.ParseID(id); err != nil {
		return dto.OK{}, err
	}
	name, err := domain.NormalizeUsername(in.Username)
	if err != nil {
		return dto.OK{}, err
	}
	hash, err := passwordHash(in.New)
	if err != nil {
		return dto.OK{}, err
	}
	err = s.platform(ctx, token, domain.Shared, func(tx domain.Store, _ platformState) error {
		if _, err := tx.TenantByID(id, domain.Exclusive); err != nil {
			return err
		}
		account, err := tx.UserByName(id, name, domain.Unlocked)
		if err != nil {
			return err
		}
		if account.User.Role != "admin" {
			return domain.NotFound()
		}
		if _, err := tx.UpdateUser(id, account.User.ID, domain.UserChanges{PasswordHash: &hash}); err != nil {
			return err
		}
		return tx.RevokeUser(id, account.User.ID)
	})
	return dto.OK{OK: true}, err
}

// UpdateOwnTenant 仅允许企业管理员修改本企业名称与注册开关，不提供状态或域名变更。
func (s *Companies) UpdateOwnTenant(ctx context.Context, id dto.TenantIdentity, in dto.UpdateOwnTenantInput) (out dto.Tenant, err error) {
	changes, err := (domain.CompanyChanges{Name: in.Name, RegistrationEnabled: in.Registration}).Validated()
	if err != nil {
		return out, err
	}
	err = s.tenant(ctx, id, domain.Exclusive, domain.Shared, true, true, func(tx domain.Store, state tenantState) error {
		tenant, err := tx.UpdateTenant(state.tenant.ID, changes)
		out = tenantDTO(tenant)
		return err
	})
	return
}
