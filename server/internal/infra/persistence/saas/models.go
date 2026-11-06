package repository

import (
	"strconv"
	"time"

	"go-server/internal/domain/saas"
)

// 持久化模型仅限本包使用，刻意不声明 GORM 关联，防止隐式关联写入。
// 企业归属和会话主体的逻辑关系由应用用例在事务内校验。
type tenantModel struct {
	ID                     int64 `gorm:"primaryKey"`
	Name, Hostname, Status string
	RegistrationEnabled    bool
	AppID, AppSecret       string
	WeChatConfigVersion    int64 `gorm:"column:wechat_config_version"`
	CreatedAt, UpdatedAt   time.Time
}

func (tenantModel) TableName() string { return "company" }
func (m tenantModel) view() domain.Tenant {
	return domain.Tenant{ID: strconv.FormatInt(m.ID, 10), Name: m.Name, Hostname: m.Hostname, Status: m.Status, RegistrationEnabled: m.RegistrationEnabled, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

type userModel struct {
	ID                                          int64 `gorm:"primaryKey"`
	TenantID                                    int64
	Username, Nickname, AvatarURL, Role, Status string
	PasswordHash                                string
	WeChatAppID                                 string `gorm:"column:wechat_app_id"`
	OpenID                                      string
	CreatedAt, UpdatedAt                        time.Time
}

func (userModel) TableName() string { return "users" }
func (m userModel) view() domain.User {
	return domain.User{ID: strconv.FormatInt(m.ID, 10), TenantID: strconv.FormatInt(m.TenantID, 10), Username: m.Username, Nickname: m.Nickname, AvatarURL: m.AvatarURL, Role: m.Role, Status: m.Status, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

type adminModel struct {
	ID                     int64 `gorm:"primaryKey"`
	Username, PasswordHash string
	CreatedAt, UpdatedAt   time.Time
}

func (adminModel) TableName() string { return "super_admin" }
func (m adminModel) view() domain.PlatformAdmin {
	return domain.PlatformAdmin{ID: strconv.FormatInt(m.ID, 10), Username: m.Username, CreatedAt: m.CreatedAt}
}

type sessionModel struct {
	ID                   int64 `gorm:"primaryKey"`
	TokenHash, Scope     string
	SubjectID            int64
	TenantID             *int64
	ExpiresAt, CreatedAt time.Time
}

func (sessionModel) TableName() string { return "auth_sessions" }
