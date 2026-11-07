package providers

func newCometapiProvider(logo string) Definition {
	d := compatible("cometapi", "CometAPI", "CometAPI 聚合模型服务。", "https://api.cometapi.com/v1", logo, "chat")
	return d
}
