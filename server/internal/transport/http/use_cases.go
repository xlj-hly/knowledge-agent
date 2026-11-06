package httpapi

import (
	"context"

	"go-server/internal/app/saas/dto"
	"go-server/internal/app/storage"
)

// AuthUseCases 是认证路由实际消费的业务能力。
type AuthUseCases interface {
	Tenant(context.Context, dto.TenantIdentity) (dto.Tenant, error)
	Me(context.Context, dto.TenantIdentity) (dto.TenantSession, error)
	Login(context.Context, dto.TenantIdentity, dto.LoginInput) (dto.TenantSession, error)
	Logout(context.Context, dto.TenantIdentity) (dto.OK, error)
	PlatformLogin(context.Context, dto.LoginInput) (dto.PlatformSession, error)
	PlatformMe(context.Context, string) (dto.PlatformSession, error)
	PlatformLogout(context.Context, string) (dto.OK, error)
	WeChatLogin(context.Context, dto.TenantIdentity, dto.WeChatLoginInput) (dto.TenantSession, error)
}

// AccountUseCases 是账号与头像路由实际消费的业务能力。
type AccountUseCases interface {
	Profile(context.Context, dto.TenantIdentity, dto.ProfileInput) (dto.User, error)
	ChangePassword(context.Context, dto.TenantIdentity, dto.PasswordInput) (dto.OK, error)
	PlatformPassword(context.Context, string, dto.PasswordInput) (dto.OK, error)
	ListUsers(context.Context, dto.TenantIdentity, dto.Pagination) (dto.Page[dto.User], error)
	CreateUser(context.Context, dto.TenantIdentity, dto.CreateUserInput) (dto.User, error)
	UpdateUser(context.Context, dto.TenantIdentity, string, dto.UpdateUserInput) (dto.User, error)
	ResetPassword(context.Context, dto.TenantIdentity, string, dto.ResetPasswordInput) (dto.OK, error)
	UploadAvatar(context.Context, dto.TenantIdentity, []byte) (dto.AvatarUpload, error)
	OpenAvatar(context.Context, string, string, string) (storage.Object, error)
	ChangeWeChatPassword(context.Context, dto.TenantIdentity, dto.WeChatPasswordInput) (dto.OK, error)
}

// CompanyUseCases 是企业管理与微信配置路由实际消费的业务能力。
type CompanyUseCases interface {
	ListTenants(context.Context, string, dto.Pagination) (dto.Page[dto.Tenant], error)
	CreateTenant(context.Context, string, dto.CreateTenantInput) (dto.CreatedTenant, error)
	UpdateTenant(context.Context, string, string, dto.UpdateTenantInput) (dto.Tenant, error)
	ResetAdminPassword(context.Context, string, string, dto.ResetAdminPasswordInput) (dto.OK, error)
	UpdateOwnTenant(context.Context, dto.TenantIdentity, dto.UpdateOwnTenantInput) (dto.Tenant, error)
	WeChatConfig(context.Context, string, string) (dto.WeChatConfigView, error)
	SaveWeChatConfig(context.Context, string, string, dto.WeChatConfigInput) (dto.WeChatConfigView, error)
}

// SiteUseCases 提供站点解析和平台域名入口判断。
type SiteUseCases interface {
	Site(context.Context, string) (dto.Site, error)
	IsPlatformHost(string) bool
}

// MaintenanceUseCases 提供 HTTP 健康检查所需的能力。
type MaintenanceUseCases interface {
	Health(context.Context) (dto.OK, error)
}

// Dependencies 由应用入口注入，HTTP 层不创建业务服务或基础设施。
type Dependencies struct {
	Auth        AuthUseCases
	Accounts    AccountUseCases
	Companies   CompanyUseCases
	Sites       SiteUseCases
	Maintenance MaintenanceUseCases
}
