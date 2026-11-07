package providers

func newPerplexityProvider(logo string) Definition {
	d := compatible("perplexity", "Perplexity", "Perplexity Sonar 对话服务，按原生根地址调用。", "https://api.perplexity.ai", logo, "chat")
	d.Compatibility.RootPath = ""
	d.Compatibility.DiscoveryPath = ""
	return d
}
