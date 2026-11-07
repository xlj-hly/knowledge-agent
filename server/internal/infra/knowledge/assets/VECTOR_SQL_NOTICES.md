# SQL 向量驱动资源与参考

- `pgvector.png` 来自 AnythingLLM `cbaf3357` 的 `frontend/src/media/vectordbs/pgvector.png`；完整 MIT 许可见 `ANYTHINGLLM_LICENSE`。
- `doris.svg` 来自 WeKnora `e3fb2335b` 的 `frontend/src/assets/img/providers/mono/vectorstore/doris.svg`；上游许可见 `WEKNORA_LICENSE`。
- PGVector 使用 pgx 与 PostgreSQL 的 pgvector 扩展，目标数据库预先安装扩展；不复用应用业务连接，不在普通运行路径创建扩展或 schema。
- Doris 使用 MySQL 协议、`ARRAY<FLOAT>` 和 `cosine_distance` 精确余弦计算，不依赖 ANN 版本差异。原生函数契约参考 https://doris.apache.org/docs/3.x/sql-manual/sql-functions/scalar-functions/distance-functions/cosine-distance/ 。建表及数组序列化参考 WeKnora Doris repository 的组织方式，所有范围、状态和处理代契约采用本项目接口。

SQL 值通过驱动参数绑定；不能绑定的 schema、表名与维度先校验再安全引用。SQL 测试同时包含连接替身验证与按显式测试配置启用的真实引擎生命周期，替身测试不代表真实服务验收。
