package providers

func newZaiProvider(logo string) Definition {
	d := compatible("zai", "Z.AI", "Z.AI GLM OpenAI 兼容对话服务。", "https://api.z.ai/api/paas/v4", logo, "chat")
	return d
}
