package providers

func newOpenrouterProvider(logo string) Definition {
	d := compatible("openrouter", "OpenRouter", "统一连接多个模型服务；模型可远端发现或手动填写。", "https://openrouter.ai/api/v1", logo, "chat", "embedding")
	d.Compatibility.TokenField = "max_completion_tokens"
	d.Compatibility.EmbeddingDiscoveryPath = "/embeddings/models"
	return d
}
