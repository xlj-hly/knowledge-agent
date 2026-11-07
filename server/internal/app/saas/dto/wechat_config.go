package dto

// WeChatConfigView 是平台授权配置入口使用的完整小程序配置。
type WeChatConfigView struct {
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}

// WeChatConfigInput 完整替换小程序配置，AppID 与 AppSecret 均为必填。
type WeChatConfigInput struct {
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}
