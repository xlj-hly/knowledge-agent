package providers

func newOpenaiCompatibleProvider(logo string) Definition {
	d := compatible("openai-compatible", "自定义 OpenAI 兼容服务", "连接其他 OpenAI 兼容模型服务或自建网关。", "", logo, "chat", "embedding")
	d.CredentialRequired = false
	return d
}
