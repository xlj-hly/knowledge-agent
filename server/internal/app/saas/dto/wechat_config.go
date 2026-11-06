package dto

// WeChatConfigView 仅公开AppID及密钥是否存在，不包含凭据。
type WeChatConfigView struct {
	AppID     string `json:"app_id"`
	HasSecret bool   `json:"has_secret"`
}

// WeChatConfigInput 的空密钥表示保留同一AppID的原密钥；首次保存及更换AppID必须提供密钥。
type WeChatConfigInput struct {
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}
