package providers

import (
	"embed"
	"encoding/base64"
	"fmt"
	"path/filepath"
)

//go:embed assets/*
var assets embed.FS

// Builtins 显式组装内置服务商，每次返回独立定义；资源缺失在启动时报告。
func Builtins() ([]Definition, error) {
	entries := []struct {
		file  string
		build func(string) Definition
	}{
		{"openai.svg", newOpenaiProvider},
		{"deepseek.svg", newDeepseekProvider},
		{"qwen.svg", newQwenProvider},
		{"siliconflow.svg", newSiliconflowProvider},
		{"ollama.png", newOllamaProvider},
		{"openrouter.svg", newOpenrouterProvider},
		{"groq.png", newGroqProvider},
		{"lmstudio.png", newLmstudioProvider},
		{"localai.png", newLocalaiProvider},
		{"litellm.svg", newLitellmProvider},
		{"moonshot.svg", newMoonshotProvider},
		{"mistral.jpeg", newMistralProvider},
		{"togetherai.png", newTogetheraiProvider},
		{"fireworksai.jpeg", newFireworksaiProvider},
		{"nvidia.svg", newNvidiaProvider},
		{"ppio.png", newPpioProvider},
		{"novita.svg", newNovitaProvider},
		{"xai.png", newXaiProvider},
		{"zai.png", newZaiProvider},
		{"minimax.svg", newMinimaxProvider},
		{"cerebras.png", newCerebrasProvider},
		{"giteeai.png", newGiteeaiProvider},
		{"sambanova.png", newSambanovaProvider},
		{"cohere.png", newCohereProvider},
		{"apipie.png", newApipieProvider},
		{"cometapi.png", newCometapiProvider},
		{"gemini.svg", newGeminiProvider},
		{"perplexity.png", newPerplexityProvider},
		{"openai-compatible.svg", newOpenaiCompatibleProvider},
	}
	out := make([]Definition, 0, len(entries))
	for _, entry := range entries {
		data, err := assets.ReadFile("assets/" + entry.file)
		if err != nil {
			return nil, fmt.Errorf("读取模型服务商图形: %w", err)
		}
		mime := "image/png"
		switch filepath.Ext(entry.file) {
		case ".svg":
			mime = "image/svg+xml"
		case ".jpeg":
			mime = "image/jpeg"
		}
		out = append(out, entry.build("data:"+mime+";base64,"+base64.StdEncoding.EncodeToString(data)))
	}
	return out, nil
}
