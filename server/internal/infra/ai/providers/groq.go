package providers

func newGroqProvider(logo string) Definition {
	d := compatible("groq", "Groq", "Groq 云端低延迟推理服务。", "https://api.groq.com/openai/v1", logo, "chat")
	return d
}
