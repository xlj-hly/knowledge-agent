package providers

func newLocalaiProvider(logo string) Definition {
	d := compatible("localai", "LocalAI", "连接已部署的 LocalAI 推理与向量服务。", "http://host.docker.internal:8080/v1", logo, "chat", "embedding")
	d.CredentialRequired = false
	return d
}
