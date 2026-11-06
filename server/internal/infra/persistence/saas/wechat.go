package repository

import (
	"crypto/sha256"
	"encoding/binary"
	"strconv"

	"go-server/internal/domain/saas"
)

// LockWeChatIdentity 串行化本企业同一微信身份的首次创建，锁保持至事务结束。
// 服务先锁企业，再锁微信身份，最后锁账号；哈希碰撞只会产生额外串行等待。
func (s *Store) LockWeChatIdentity(tenantID, appID, openID string) error {
	digest := sha256.Sum256([]byte("wechat:" + tenantID + ":" + appID + ":" + openID))
	key := int64(binary.BigEndian.Uint64(digest[:8]))
	return checked("lock WeChat identity", s.db.Exec("SELECT pg_advisory_xact_lock(?)", key).Error)
}

// WeChatUserID 只在给定企业和小程序范围内查找已验证身份，不公开 OpenID。
func (s *Store) WeChatUserID(tenantID, appID, openID string) (string, error) {
	var m userModel
	err := s.db.Where("tenant_id = ? AND wechat_app_id = ? AND open_id = ?", tenantID, appID, openID).Take(&m).Error
	return strconv.FormatInt(m.ID, 10), checked("find WeChat user", err)
}

// CreateWeChatUser 为首次微信登录创建普通成员，默认密码散列仅写入一次。
// 微信身份直接保存在账号字段中，账号和会话必须在同一事务提交；再次登录不得覆盖用户密码。
func (s *Store) CreateWeChatUser(tenantID, username, passwordHash, appID, openID string) (domain.User, error) {
	id, err := domain.ParseID(tenantID)
	if err != nil {
		return domain.User{}, err
	}
	m := userModel{TenantID: id, Username: username, PasswordHash: passwordHash, WeChatAppID: appID, OpenID: openID, Nickname: "微信用户", Role: "user", Status: "active"}
	err = s.db.Create(&m).Error
	return m.view(), checked("create WeChat user", err)
}
