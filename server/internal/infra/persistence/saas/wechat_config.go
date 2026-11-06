package repository

import (
	"strconv"
	"time"

	"go-server/internal/domain/saas"
)

// WeChatConfig 按租户读取当前配置；调用方通过企业行锁保护最终复核及写入。
func (s *Store) WeChatConfig(tenantID string) (domain.WeChatConfiguration, error) {
	var m tenantModel
	if err := s.db.Where("id = ?", tenantID).Take(&m).Error; err != nil {
		return domain.WeChatConfiguration{}, checked("read company WeChat configuration", err)
	}
	if m.AppID == "" || m.AppSecret == "" {
		return domain.WeChatConfiguration{}, domain.NotFound()
	}
	return domain.WeChatConfiguration{TenantID: strconv.FormatInt(m.ID, 10), AppID: m.AppID, AppSecret: m.AppSecret, Version: m.WeChatConfigVersion}, nil
}

// SaveWeChatConfig 要求已持有企业排他锁；配置版本只增不减，且凭据必须完整。
func (s *Store) SaveWeChatConfig(value domain.WeChatConfiguration) error {
	now := time.Now().UTC()
	return checked("update company WeChat configuration", s.db.Model(&tenantModel{}).Where("id = ?", value.TenantID).Updates(map[string]any{"app_id": value.AppID, "app_secret": value.AppSecret, "wechat_config_version": value.Version, "updated_at": now}).Error)
}
