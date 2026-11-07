package providers

func newSambanovaProvider(logo string) Definition {
	d := compatible("sambanova", "SambaNova", "SambaNova 托管开源模型服务。", "https://api.sambanova.ai/v1", logo, "chat")
	return d
}
