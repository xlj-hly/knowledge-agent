package usecase

import (
	"go-server/internal/app/saas/dto"
	"go-server/internal/domain/saas"
)

func userDTO(u domain.User) dto.User {
	return dto.User{ID: u.ID, TenantID: u.TenantID, Username: u.Username, Nickname: u.Nickname, AvatarURL: u.AvatarURL, Role: u.Role, Status: u.Status, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt}
}

func tenantDTO(t domain.Tenant) dto.Tenant {
	return dto.Tenant{ID: t.ID, Name: t.Name, Hostname: t.Hostname, Status: t.Status, RegistrationEnabled: t.RegistrationEnabled, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
}

func adminDTO(a domain.PlatformAdmin) dto.PlatformAdmin {
	return dto.PlatformAdmin{ID: a.ID, Username: a.Username, CreatedAt: a.CreatedAt}
}

func usersDTO(page domain.Page[domain.User]) dto.Page[dto.User] {
	out := dto.Page[dto.User]{Items: make([]dto.User, 0, len(page.Items)), Total: page.Total, Page: page.Page, Size: page.Size}
	for _, user := range page.Items {
		out.Items = append(out.Items, userDTO(user))
	}
	return out
}

func tenantsDTO(page domain.Page[domain.Tenant]) dto.Page[dto.Tenant] {
	out := dto.Page[dto.Tenant]{Items: make([]dto.Tenant, 0, len(page.Items)), Total: page.Total, Page: page.Page, Size: page.Size}
	for _, tenant := range page.Items {
		out.Items = append(out.Items, tenantDTO(tenant))
	}
	return out
}
