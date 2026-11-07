package providers

func newTogetheraiProvider(logo string) Definition {
	d := compatible("togetherai", "Together AI", "Together AI 托管模型推理服务。", "https://api.together.xyz/v1", logo, "chat")
	return d
}
