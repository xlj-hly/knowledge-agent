package providers

func newCohereProvider(logo string) Definition {
	d := compatible("cohere", "Cohere", "Cohere OpenAI 兼容对话与文本向量入口。", "https://api.cohere.ai/compatibility/v1", logo, "chat", "embedding")
	d.Compatibility.DiscoveryPath = ""
	return d
}
