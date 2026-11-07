// Package providers 定义服务商预设，不持有企业连接、凭据或运行时客户端。
package providers

// Model 是可选模型候选；远端发现和手填模型不受候选目录限制。
type Model struct {
	ID, Kind  string
	Dimension int
}

// Compatibility 描述同一协议内已确认的路径和参数差异。
type Compatibility struct {
	RootPath               string
	DiscoveryPath          string
	EmbeddingDiscoveryPath string
	TokenField             string
	OmitTemperature        bool
}

// Definition 是一个服务商模块的完整静态定义，协议由运行时注册表解析。
type Definition struct {
	ID, Name, Description, Logo, Protocol, Endpoint string
	Kinds                                           []string
	Models                                          []Model
	// CredentialRequired 仅提示默认云服务需要密钥；自建网关可采用不同认证部署。
	CredentialRequired bool
	Compatibility      Compatibility
}

func compatible(id, name, description, endpoint, logo string, kinds ...string) Definition {
	return Definition{ID: id, Name: name, Description: description, Endpoint: endpoint, Logo: logo, Protocol: "openai", Kinds: kinds, Models: []Model{}, CredentialRequired: true,
		Compatibility: Compatibility{RootPath: "/v1", DiscoveryPath: "/models", TokenField: "max_tokens"}}
}
