package providers

func newMinimaxProvider(logo string) Definition {
	d := compatible("minimax", "MiniMax", "MiniMax 国际版 OpenAI 兼容对话服务，可修改地区地址。", "https://api.minimax.io/v1", logo, "chat")
	d.Compatibility.TokenField = "max_completion_tokens"
	return d
}
