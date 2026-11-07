package providers

func newMistralProvider(logo string) Definition {
	d := compatible("mistral", "Mistral AI", "Mistral 对话与文本向量服务。", "https://api.mistral.ai/v1", logo, "chat", "embedding")
	return d
}
