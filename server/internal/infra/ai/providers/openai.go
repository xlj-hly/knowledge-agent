package providers

func newOpenaiProvider(logo string) Definition {
	d := compatible("openai", "OpenAI", "OpenAI 云端对话与文本向量服务。", "https://api.openai.com/v1", logo, "chat", "embedding")
	d.Models = []Model{{ID: "gpt-4.1-mini", Kind: "chat", Dimension: 0}, {ID: "gpt-4.1", Kind: "chat", Dimension: 0}, {ID: "text-embedding-3-small", Kind: "embedding", Dimension: 1536}, {ID: "text-embedding-3-large", Kind: "embedding", Dimension: 3072}}
	return d
}
