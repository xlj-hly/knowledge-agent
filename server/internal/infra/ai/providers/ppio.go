package providers

func newPpioProvider(logo string) Definition {
	d := compatible("ppio", "PPIO", "PPIO 托管开源模型服务。", "https://api.ppinfra.com/v3/openai", logo, "chat")
	return d
}
