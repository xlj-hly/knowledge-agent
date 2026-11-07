package providers

func newLitellmProvider(logo string) Definition {
	d := compatible("litellm", "LiteLLM", "连接已部署的 OpenAI 兼容模型代理。", "http://host.docker.internal:4000/v1", logo, "chat", "embedding")
	d.CredentialRequired = false
	return d
}
