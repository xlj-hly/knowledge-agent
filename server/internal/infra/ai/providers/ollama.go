package providers

func newOllamaProvider(logo string) Definition {
	d := compatible("ollama", "Ollama", "连接已部署的本地 Ollama 模型服务。", "http://host.docker.internal:11434", logo, "chat", "embedding")
	d.CredentialRequired = false
	d.Protocol = "ollama"
	d.Compatibility = Compatibility{DiscoveryPath: "/api/tags"}
	d.Models = []Model{{ID: "qwen3:8b", Kind: "chat", Dimension: 0}, {ID: "bge-m3", Kind: "embedding", Dimension: 1024}, {ID: "nomic-embed-text", Kind: "embedding", Dimension: 768}}
	return d
}
