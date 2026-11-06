package usecase

import (
	"context"
	"regexp"
	"strconv"

	"go-server/internal/app/saas/dto"
	"go-server/internal/domain/saas"
)

var appIDFormat = regexp.MustCompile(`^wx[0-9a-fA-F]{16}$`)
var appSecretFormat = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)

// WeChatConfig 仅允许平台管理员读取目标租户的安全配置视图。
func (s *Companies) WeChatConfig(ctx context.Context, token, tenantID string) (out dto.WeChatConfigView, err error) {
	parsedID, parseErr := domain.ParseID(tenantID)
	if parseErr != nil {
		err = parseErr
		return
	}
	tenantID = strconv.FormatInt(parsedID, 10)
	err = s.platform(ctx, token, domain.Shared, func(tx domain.Store, _ platformState) error {
		if _, err := tx.TenantByID(tenantID, domain.Shared); err != nil {
			return err
		}
		cfg, err := tx.WeChatConfig(tenantID)
		if credentialsMissing(err) {
			return nil
		}
		if err != nil {
			return err
		}
		out = dto.WeChatConfigView{AppID: cfg.AppID, HasSecret: cfg.AppSecret != ""}
		return nil
	})
	return
}

// SaveWeChatConfig 在平台身份及企业锁内原子保存配置，企业改域名不会改变配置归属。
func (s *Companies) SaveWeChatConfig(ctx context.Context, token, tenantID string, in dto.WeChatConfigInput) (out dto.WeChatConfigView, err error) {
	parsedID, parseErr := domain.ParseID(tenantID)
	if parseErr != nil {
		err = parseErr
		return
	}
	tenantID = strconv.FormatInt(parsedID, 10)
	if !appIDFormat.MatchString(in.AppID) || (in.AppSecret != "" && !appSecretFormat.MatchString(in.AppSecret)) {
		return out, domain.Invalid("小程序AppID或AppSecret格式无效")
	}
	err = s.platform(ctx, token, domain.Shared, func(tx domain.Store, _ platformState) error {
		if _, err := tx.TenantByID(tenantID, domain.Exclusive); err != nil {
			return err
		}
		old, err := tx.WeChatConfig(tenantID)
		if err != nil && !credentialsMissing(err) {
			return err
		}
		exists := err == nil
		if in.AppSecret == "" {
			if !exists || old.AppID != in.AppID {
				return domain.Invalid("首次配置或更换AppID必须填写AppSecret")
			}
			out = dto.WeChatConfigView{AppID: old.AppID, HasSecret: true}
			return nil
		}
		version := int64(1)
		if exists {
			version = old.Version + 1
		}
		if err := tx.SaveWeChatConfig(domain.WeChatConfiguration{TenantID: tenantID, AppID: in.AppID, AppSecret: in.AppSecret, Version: version}); err != nil {
			return err
		}
		out = dto.WeChatConfigView{AppID: in.AppID, HasSecret: true}
		return nil
	})
	return
}
