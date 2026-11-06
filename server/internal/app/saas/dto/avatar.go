package dto

// AvatarUpload 是持久化头像地址，客户端确认资料时仍需调用Profile。
type AvatarUpload struct {
	AvatarURL string `json:"avatar_url"`
}
