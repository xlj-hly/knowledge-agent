package usecase

import (
	"context"
	"errors"

	"go-server/internal/app/saas/dto"
	"go-server/internal/domain/saas"
)

// isPlatformHost 只比较规范化后的 HTTP Host，不信任客户端租户覆盖头。
func (s *runtime) isPlatformHost(value string) bool {
	host, err := domain.NormalizeHostname(value)
	return err == nil && s.platformHosts[host]
}

// IsPlatformHost 判断请求是否来自已配置的平台域名。
func (s *Sites) IsPlatformHost(value string) bool { return s.isPlatformHost(value) }

// Site 解析已绑定的站点身份；停用状态由页面展示，认证仍由业务入口拒绝。
// 数据库故障保留为内部错误，不能混同于未绑定域名。
func (s *Sites) Site(ctx context.Context, hostname string) (dto.Site, error) {
	host, err := domain.NormalizeHostname(hostname)
	if err != nil {
		return dto.Site{}, err
	}
	out := dto.Site{Kind: "unknown", Hostname: host}
	if s.platformHosts[host] {
		out.Kind = "platform"
		return out, nil
	}
	tenant, err := s.store.WithContext(ctx).TenantByHost(host, domain.Unlocked)
	var business *domain.Error
	if errors.As(err, &business) && business.Code == "not_found" {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Kind = "tenant"
	view := tenantDTO(tenant)
	out.Tenant = &view
	return out, nil
}
