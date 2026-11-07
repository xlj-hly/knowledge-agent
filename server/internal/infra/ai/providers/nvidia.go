package providers

func newNvidiaProvider(logo string) Definition {
	d := compatible("nvidia", "NVIDIA NIM", "NVIDIA 托管的 OpenAI 兼容推理入口。", "https://integrate.api.nvidia.com/v1", logo, "chat")
	return d
}
