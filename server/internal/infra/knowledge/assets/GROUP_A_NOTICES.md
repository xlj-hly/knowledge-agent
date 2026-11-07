# 向量服务 Logo 与协议参考

`chroma.png`、`pinecone.png`、`weaviate.png`、`astra.png` 来自 AnythingLLM `cbaf3357` 的 `frontend/src/media/vectordbs/`。
资源版权及许可见同目录 `ANYTHINGLLM_LICENSE`，商标归各服务提供商所有。

驱动按协议独立实现，并参考 AnythingLLM 对应 `server/utils/vectorDbProviders/` 的产品连接方式；没有引入其全局环境配置或 Workspace 状态实现。

协议核对来源：

- Chroma 官方 v2 REST 客户端：[chroma-core/chroma 的 chromadb/api/fastapi.py](https://github.com/chroma-core/chroma/blob/main/chromadb/api/fastapi.py)；已用 Chroma 1.3.5 实例验证。
- Pinecone [2025-10 describe_index_stats](https://docs.pinecone.io/reference/api/2025-10/data-plane/describeindexstats)，包括 `dimension`、`metric` 响应；请求明确发送 `X-Pinecone-API-Version: 2025-10`。
- Weaviate [REST API](https://docs.weaviate.io/weaviate/api/rest) 与 [GraphQL](https://docs.weaviate.io/weaviate/api/graphql)；schema、batch 和 GraphQL 使用同一 HTTP 地址，无 gRPC 依赖。
- Astra DB 官方 [astrapy/data/database.py](https://github.com/datastax/astrapy/blob/main/astrapy/data/database.py) 和 [astrapy/data/collection.py](https://github.com/datastax/astrapy/blob/main/astrapy/data/collection.py)；`findCollections`、`insertMany`、`find`、`deleteMany` 的 Data API 契约。
