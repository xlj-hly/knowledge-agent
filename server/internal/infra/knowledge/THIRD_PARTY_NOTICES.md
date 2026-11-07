# 第三方来源

本目录部分代码复用 [Tencent/WeKnora](https://github.com/Tencent/WeKnora)，参考提交 `e3fb2335b`。
版权与 MIT 许可完整正文见 [WEKNORA_LICENSE](WEKNORA_LICENSE)。

- `chunker/*.go`：来自 `internal/infrastructure/chunker`，保留原始切分算法及相应测试。日志改为标准库 `slog`；图片引用提取不再重写输入，保持偏移对应调用方原文；未引入其业务实体和身份依赖。
- `parser/csv_converter.go`、`parser/json_converter.go` 及相应测试：来自 `internal/infrastructure/docparser`，修改包名，CSV 与 JSON 派生输出统一限制为 2 MiB。JSON 保留原语义分块，限制重复路径总体积，缩进编码超限时改紧凑编码；入口限制嵌套深度，避免递归放大。
- `parser/parser.go` 的文本解码顺序：复用 `builtin_converter.go` 中的 UTF-8、UTF-16、GB18030、Windows-1252、Latin-1 实现。
- `../persistence/knowledge/keyword.go` 的中文双字词法设计参考 `internal/application/repository/retriever/sqlite/repository.go` 的 `tokenizeCJKBigram` 思路；本项目自行实现去重、最小命中数和 PostgreSQL 参数化覆盖率查询，不引入其 SQLite 或 Jieba 运行依赖。

PDF 接入使用 WeKnora DocReader 的 `/read` HTTP 协议。Embedding 使用其 OpenAI 兼容及 Ollama 协议形状与响应顺序校验原则；Qdrant 采用其按向量维度分集合的方式，业务归属与处理代由本项目维护。

`assets/qdrant.png` 来自 AnythingLLM `cbaf3357` 的向量服务商品牌资源，仅用于引擎识别；完整 MIT 许可见 `assets/ANYTHINGLLM_LICENSE`。模型服务商品牌和预设来源另见 `../ai/providers/THIRD_PARTY_NOTICES.md`，Docker 镜像随附相应许可。
