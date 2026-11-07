# HTTP 向量驱动来源与协议

本项目自行实现隔离配置、业务 ID、处理代、批量写入、检索和清理流程，不引入参考项目的业务实体或全局配置。

- `milvus.go`：使用 Milvus / Zilliz 的 REST v2 协议。请求及响应字段核对 [Milvus HTTP v2 handler](https://github.com/milvus-io/milvus/blob/master/internal/distributed/proxy/httpserver/handler_v2.go)、[字段映射](https://github.com/milvus-io/milvus/blob/master/internal/distributed/proxy/httpserver/utils.go)、[协议常量](https://github.com/milvus-io/milvus/blob/master/internal/distributed/proxy/httpserver/constant.go)。集合启用动态元数据，向量度量固定 COSINE；描述响应的主键字段为 `primaryKey`，创建请求为 `isPrimary`。
- `elasticsearch.go`：使用 Elasticsearch 8 / 9 的 dense_vector 与 OpenSearch k-NN 的 knn_vector REST 协议。两者通过过滤后的精确 Painless cosine 查询返回统一的 `[-1,1]` 分数。OpenSearch 的双参数 `cosineSimilarity` 重载见 [KNNScoringUtil](https://github.com/opensearch-project/k-NN/blob/main/src/main/java/org/opensearch/knn/plugin/script/KNNScoringUtil.java)。避免直接使用 `knn_score`，因为 [KNNScoringSpace](https://github.com/opensearch-project/k-NN/blob/main/src/main/java/org/opensearch/knn/plugin/script/KNNScoringSpace.java) 对 2.19 前后的索引使用不同分数转换。
- `tencent_vectordb.go`：请求路径、认证头、集合索引与文档形状核对 Tencent 官方 [vectordatabase-sdk-go v1.8.4](https://github.com/tencent/vectordatabase-sdk-go/tree/v1.8.4/tcvectordb)。HTTP API 使用 `/collection/*`、`/document/*`；配置指向已有数据库，集合按向量维度建立，过滤字段覆盖企业、知识库、资料及处理代。接口形状和索引规划参考 WeKnora `e3fb2335b` 的 `internal/application/repository/retriever/tencentvectordb`。官方 SDK MIT 许可见 [TENCENT_VECTORDB_SDK_LICENSE](assets/TENCENT_VECTORDB_SDK_LICENSE)；WeKnora 许可见 [WEKNORA_LICENSE](WEKNORA_LICENSE)。

品牌资源仅用于识别连接的服务：

- `assets/milvus.png`、`assets/zilliz.png`：来自 AnythingLLM `cbaf3357` 的 `frontend/src/media/vectordbs`，MIT 许可见 [ANYTHINGLLM_LICENSE](assets/ANYTHINGLLM_LICENSE)。
- `assets/elasticsearch.svg`、`assets/opensearch.svg`：来自 WeKnora `e3fb2335b` 的 `frontend/src/assets/img/providers/color/vectorstore`，MIT 许可见 [WEKNORA_LICENSE](WEKNORA_LICENSE)。
- `assets/tencent-vectordb.svg`：使用 [Lobe Icons Tencent Cloud 品牌资源](https://github.com/lobehub/lobe-icons/blob/master/packages/static-svg/icons/tencentcloud-color.svg)，MIT 许可见 [LOBE_ICONS_LICENSE](assets/LOBE_ICONS_LICENSE)。

`*_test.go` 的协议服务器为受控测试替身，验证本项目实际发送的请求、隔离范围、批次与响应处理；它们不代表已连接真实 Milvus、Zilliz、Elasticsearch、OpenSearch 或腾讯云实例。
