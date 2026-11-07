package providers

func newApipieProvider(logo string) Definition {
	d := compatible("apipie", "APIpie", "APIpie 聚合模型服务。", "https://apipie.ai/v1", logo, "chat")
	return d
}
