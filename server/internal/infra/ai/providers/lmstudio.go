package providers

func newLmstudioProvider(logo string) Definition {
	d := compatible("lmstudio", "LM Studio", "连接已启动 OpenAI 兼容服务器的 LM Studio。", "http://host.docker.internal:1234/v1", logo, "chat", "embedding")
	d.CredentialRequired = false
	return d
}
