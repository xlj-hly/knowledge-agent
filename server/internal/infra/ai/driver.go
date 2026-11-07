package ai

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	domain "go-server/internal/domain/ai"
	"go-server/internal/infra/httpclient"
)

type protocolDriver struct {
	protocol string
	client   *http.Client
}

// NewOpenAIDriver 创建 OpenAI Chat Completions、Embedding 与模型发现驱动。
func NewOpenAIDriver() Driver { return &protocolDriver{protocol: "openai", client: httpclient.New()} }

// NewOllamaDriver 创建 Ollama NDJSON 与批量 Embedding 驱动。
func NewOllamaDriver() Driver { return &protocolDriver{protocol: "ollama", client: httpclient.New()} }

func (d *protocolDriver) Protocol() string { return d.protocol }
func (d *protocolDriver) Kinds() []string  { return []string{"chat", "embedding"} }
func headers(model domain.Model) map[string]string {
	out := map[string]string{}
	if model.APIKey != "" {
		out["Authorization"] = "Bearer " + model.APIKey
	}
	return out
}

func chatPath(protocol string) string {
	if protocol == "ollama" {
		return "/api/chat"
	}
	return "/chat/completions"
}

// protocolEndpoint 保留管理员配置的网关前缀及完整操作地址；根路径由预设明确声明。
func protocolEndpoint(endpoint, rootPath, suffix string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if suffix != "" && strings.HasSuffix(base, suffix) {
		return httpclient.Endpoint(base, "")
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", errors.New("模型服务地址无效")
	}
	if parsed.Path == "" {
		base += rootPath
	}
	return httpclient.Endpoint(base, suffix)
}
