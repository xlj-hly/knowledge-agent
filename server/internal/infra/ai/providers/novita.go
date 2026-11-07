package providers

func newNovitaProvider(logo string) Definition {
	d := compatible("novita", "Novita AI", "Novita OpenAI 兼容模型服务。", "https://api.novita.ai/v3/openai", logo, "chat")
	return d
}
