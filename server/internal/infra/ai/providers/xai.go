package providers

func newXaiProvider(logo string) Definition {
	d := compatible("xai", "xAI", "xAI Grok 对话服务。", "https://api.x.ai/v1", logo, "chat")
	return d
}
