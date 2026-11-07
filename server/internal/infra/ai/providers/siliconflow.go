package providers

func newSiliconflowProvider(logo string) Definition {
	d := compatible("siliconflow", "硅基流动", "托管开源对话与文本向量模型。", "https://api.siliconflow.cn/v1", logo, "chat", "embedding")
	d.Models = []Model{{ID: "Qwen/Qwen3-8B", Kind: "chat", Dimension: 0}, {ID: "BAAI/bge-m3", Kind: "embedding", Dimension: 1024}}
	return d
}
