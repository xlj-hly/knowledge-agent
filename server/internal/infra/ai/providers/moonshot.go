package providers

func newMoonshotProvider(logo string) Definition {
	d := compatible("moonshot", "Moonshot AI", "月之暗面 Kimi OpenAI 兼容对话服务。", "https://api.moonshot.ai/v1", logo, "chat")
	d.Compatibility.TokenField = "max_completion_tokens"
	d.Compatibility.OmitTemperature = true
	return d
}
