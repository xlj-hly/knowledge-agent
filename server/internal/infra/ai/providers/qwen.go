package providers

func newQwenProvider(logo string) Definition {
	d := compatible("qwen", "通义千问（百炼）", "阿里云百炼 OpenAI 兼容服务。", "https://dashscope.aliyuncs.com/compatible-mode/v1", logo, "chat", "embedding")
	d.Models = []Model{{ID: "qwen-plus", Kind: "chat", Dimension: 0}, {ID: "qwen-turbo", Kind: "chat", Dimension: 0}, {ID: "text-embedding-v3", Kind: "embedding", Dimension: 1024}}
	return d
}
