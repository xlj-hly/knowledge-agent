package providers

func newFireworksaiProvider(logo string) Definition {
	d := compatible("fireworksai", "Fireworks AI", "Fireworks AI 托管模型推理服务。", "https://api.fireworks.ai/inference/v1", logo, "chat")
	return d
}
