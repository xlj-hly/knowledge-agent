package providers

func newDeepseekProvider(logo string) Definition {
	d := compatible("deepseek", "DeepSeek", "DeepSeek 对话与推理模型。", "https://api.deepseek.com/v1", logo, "chat")
	d.Models = []Model{{ID: "deepseek-chat", Kind: "chat", Dimension: 0}, {ID: "deepseek-reasoner", Kind: "chat", Dimension: 0}}
	return d
}
