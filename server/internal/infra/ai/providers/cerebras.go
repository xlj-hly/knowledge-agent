package providers

func newCerebrasProvider(logo string) Definition {
	d := compatible("cerebras", "Cerebras", "Cerebras OpenAI 兼容推理服务。", "https://api.cerebras.ai/v1", logo, "chat")
	d.Compatibility.DiscoveryPath = ""
	return d
}
