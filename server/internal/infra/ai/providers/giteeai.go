package providers

func newGiteeaiProvider(logo string) Definition {
	d := compatible("giteeai", "Gitee AI", "Gitee AI 托管对话模型服务。", "https://ai.gitee.com/v1", logo, "chat")
	return d
}
