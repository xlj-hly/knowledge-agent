package providers

func newGeminiProvider(logo string) Definition {
	d := compatible("gemini", "Gemini", "Google Gemini OpenAI 兼容对话入口。", "https://generativelanguage.googleapis.com/v1beta/openai", logo, "chat")
	d.Compatibility.OmitTemperature = true
	d.Compatibility.DiscoveryPath = ""
	return d
}
