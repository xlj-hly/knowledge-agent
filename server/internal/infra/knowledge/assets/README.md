# 向量引擎品牌资源

品牌资源仅用于识别对应服务。AnythingLLM 资源和完整 MIT 许可见同目录 `ANYTHINGLLM_LICENSE`；Chroma、Pinecone、Weaviate、AstraDB 的来源见 [GROUP_A_NOTICES.md](GROUP_A_NOTICES.md)，PGVector、Doris 见 [VECTOR_SQL_NOTICES.md](VECTOR_SQL_NOTICES.md)，其余 HTTP 引擎见 [VECTOR_HTTP_NOTICES.md](../VECTOR_HTTP_NOTICES.md)。

`qdrant.png` 来自 AnythingLLM `cbaf3357` 的 `frontend/src/media/vectordbs/qdrant.png`。

资源通过 `go:embed` 编入服务端程序，由已注册引擎定义输出固定的图片 data URL；前端不再维护第二份引擎品牌映射。
