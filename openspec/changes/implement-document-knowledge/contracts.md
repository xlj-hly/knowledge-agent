# 配置与对话重构契约

## Ownership

- `domain/ai`、`app/ai/{dto,port,usecase}` 管理企业模型与默认配置；`infra/persistence/ai` 保存 `ai_models`、`ai_settings`；供应商目录和发现属于 `infra/ai`。模型 Kind 为 `chat` 或 `embedding`，能力校验和调用端口明确区分。模型字段 ID/TenantID/Name/Kind/Provider/Endpoint/ModelName/APIKey/Dimension/CreatedAt/UpdatedAt；JSON 用 snake_case，授权配置响应直接包含 api_key:string，不再使用 has_api_key。
- knowledge 保留知识库、向量连接、资料、分块、任务与清理。模型通过消费方接口由 bootstrap 适配 AI 公开能力；不跨读 AI 私有仓储。撤除 knowledge 自己的模型 CRUD 和旧 /knowledge/models 路由。
- chat 管理用户自己的会话、消息、生成状态、来源；读取 AI 配置、调用知识公开检索，不读其私有表。
- 配置更新采用完整表单并按提交值替换，不合并旧值或继承旧凭据；空值直接生效，必填字段为空时拒绝。已有接口读写授权与企业隔离保持，凭据不进入静态包、浏览器持久化、未授权接口或日志。Prompt、生成、检索和知识处理默认值是新建模板，不静默改已有对象；企业唯一启用 LLM 是独立状态，影响所有会话下一条消息。

## AI HTTP

- GET `/ai/providers` -> Provider[]：id,name,protocol,endpoint,kinds,models；models 每项 id,name,kind,dimension。
- GET/POST `/ai/models`，PUT/DELETE `/ai/models/:id`；模型输入 name,kind,provider,endpoint,model_name,dimension,api_key:string；输出包含实际 api_key，不含 has_api_key。保存按提交完整替换，api_key 空串直接清空；必填凭据校验失败则拒绝。chat dimension=0，embedding dimension>0。
- PUT `/ai/models/:id/activate` 无请求体，企业管理员启用本企业 chat 用途模型，返回完整 Settings。`ai_settings.chat_model_id` 是唯一启用状态，不另设 active 字段；普通成员、其他企业模型与 embedding 用途均拒绝。
- POST `/ai/models/discover` -> `{models:[{id,name,kind,dimension}]}`，使用当前表单中的 provider、kind、endpoint、api_key，不接受已存模型 id 回读凭据；模型发现不需要 model_name/dimension。
- POST `/ai/models/test` -> `{ok:true,dimension:number}`，输入为当前完整表单，不接受已存模型 id 回读凭据；embedding 维数可以先为0，由实际调用检测。
- GET/PUT `/ai/settings` -> Settings：chat_model_id,embedding_model_id,vector_store_id,system_prompt,temperature,max_tokens,history_turns,retrieval_mode,retrieval_limit,similarity_threshold,chunk_strategy,chunk_size,chunk_overlap,parser_endpoint。SaveSettings 输入只有除 chat_model_id 外的上述模板字段；chat_model_id 是响应中只读启用状态，保存其他配置必须保留当前值，旧表单不能覆盖。提示词默认使用通用中文知识助手模板，检索模式默认 hybrid，切分 auto/512/80，max_tokens=2048,history_turns=10,retrieval_limit=6,temperature=0.7,similarity_threshold=0。
- GET `/ai/presets` -> `{prompts:[{id,name,content}],chunking:[{id,name,strategy,size,overlap}],chunk_strategies:[{id,name,description}]}`。
- Service 的公开内部操作 `ResolveModel(ctx, tenantID, id, kind) (ai.Model,error)`、`Defaults(ctx,tenantID) (ai.Settings,error)`，调用方必须已验证企业身份，AI 用例仍验证模型归属与用途。
- 当前启用 LLM、默认 Embedder 和知识库等真实引用保护模型删除与身份变更；历史会话 model_id 快照不参与保护，旧模型仅剩历史快照时允许删除。

## Knowledge HTTP

- GET `/knowledge/vector-providers` -> `{id,name,description,logo,protocol,fields}[]`；fields 每项 `{key,label,type,required,secret,immutable,default,placeholder,help,options:[{value,label}]}`，type 为 text/password/number/select/boolean，字段值均使用字符串。页面不得按品牌另建字段映射。
- 当前目录注册 13 个远程引擎模块：Qdrant、PGVector、Chroma、Chroma Cloud、Pinecone、Milvus、Zilliz Cloud、Weaviate、AstraDB、Elasticsearch、OpenSearch、Doris、Tencent VectorDB。
- GET/POST `/knowledge/vector-stores`，PATCH/DELETE `/knowledge/vector-stores/:id`，POST `/knowledge/vector-stores/:id/test`。输入完整 `{name,provider,config:Record<string,string>}`，缺省/null/未知字段拒绝；输出 `{id,name,provider,config,referenced,created_at,updated_at}`，config 包含实际凭据，不含 configured_secrets。配置按提交完整替换，不继承旧字段或凭据；空串直接生效，必填字段缺失或为空时拒绝。secret 元数据仅用于输入类型及默认值约束，不过滤响应；被引用连接禁止更改模块声明的不可变参数。旧 endpoint/api_key/collection 顶层契约已移除。

- 原文档/分块/向量存储路由保留。Base 新增 chunk_strategy 和 retrieval_mode；indexing_mode 表示可用索引，retrieval_mode 表示查询默认策略；默认新库 hybrid，必要模型/存储未配置则明确报错，不静默退为 keyword。
- POST `/knowledge/bases/:id/rebuild` 接受完整 BaseInput，明确触发现有资料重新处理，返回 Base。必须处理旧任务/旧向量的归属及清理，不允许旧结果覆盖新配置。
- POST `/knowledge/preview` 输入 `{content,chunk_strategy,chunk_size,chunk_overlap}` -> ChunkDraft[]（content,context_header,index,start,end），不写入业务数据。
- Chunk/Hit 增加 context_header；源正文与字符范围不拼入标题，向量化和问答上下文使用标题+正文。
- 默认配置通过 AI Defaults 注入，模型 ResolveModel 注入；具体接口由 knowledge 消费方定义并通知 root。

## Platform WeChat HTTP

- GET/PUT `/platform/tenants/:id/wechat` 继续要求独立平台身份，响应为 `{app_id,app_secret}`，返回当前原值，不使用 has_secret。PUT 完整提交两项有效字符串，缺失、null 或空字符串拒绝，不继承旧密钥。
- 配置变更继续递增 wechat_config_version，微信外部验证提交前复核版本及企业状态；公共站点、普通微信登录和用户资料响应不提供 AppSecret、密码散列或微信临时 session_key。

## Chat HTTP

- GET/POST `/chat/sessions`；GET/PUT/DELETE `/chat/sessions/:id`；GET `/chat/sessions/:id/messages`；POST `/chat/sessions/:id/messages` 返回 SSE；POST `/chat/sessions/:id/stop` 停止本用户当前生成。
- SessionInput 包含 title,knowledge_base_ids,system_prompt,temperature,max_tokens,history_turns,retrieval_mode,retrieval_limit,similarity_threshold，不接受 model_id。输出另含 id,model_id,created_at,updated_at，其中 model_id 是服务端最近生成模型的只读快照。新会话缺省从企业模板复制 Prompt、生成与检索参数；更新只影响后续消息。
- Message 输出 id,session_id,role,content,status,sources,created_at；status 为 generating/completed/failed/cancelled；sources 每项 base_id,document_id,chunk_id,document_title,content,score。
- 发消息输入 `{content}`；SSE 事件为 `message`（assistant Message）、`sources`（Source[]）、`delta`（`{content}`）、`done`（最终 Message）、`error`（`{code,message}`）。前端先拿到 Message ID，断线后读取持久化状态确认结果。
- 每次发送都读取企业当前启用 LLM，新旧会话下一条消息均使用它，不读取会话 model_id 决定模型。请求开始时捕获完整模型配置，本次流式生成不受之后启用切换影响；没有启用模型时明确拒绝。
- 不选知识库为普通聊天；选择知识库时检索结果作为有来源的上下文，无命中要显式标明资料未命中，模型故障保留失败状态，不伪造成功。历史有界，来源快照保存在消息。
- 同一会话只允许一个正在生成的消息；其他会话/用户/企业不受影响。先确认企业及所有者，再判断生成冲突，跨用户或企业访问正在生成的会话仍返回 404。事务不持锁等待模型。上下文取消或停止保存已生成文本，清理生成状态；进程重启将遗留 generating 标为失败。
- 聊天占满工作区并保留历史侧栏，无消息时居中输入，有消息时底部输入。Enter 发送、Shift+Enter 换行，IME composing 期间不发送；历史和流式 Markdown 安全渲染，回答可复制，来源在 Drawer 查看。只有接近底部时自动跟随，向上阅读不被 token 拉回。
- 显式新建或重置必须失效旧创建、加载和流式请求，即使前后 URL 相同；过期创建结果不得触发跳转或继续发送。取消等待不代表服务端写入回滚。

## Implementation / Validation

- root 负责 AI 配置模块、完整 SQL、bootstrap/HTTP 总组装、接口集成与本地部署。
- knowledge worker 负责知识模块及其 HTTP / infra / 单元测试；不改 AI、chat、bootstrap、SQL。
- chat worker 负责 chat 四层、流式模型适配、chat HTTP 和测试；不改 AI 配置、knowledge、bootstrap、SQL。
- web worker 负责 react-app 全部变更、组件测试、E2E 配置；按本契约对接，偏差及时协调。
- 先验证明确失败用例再实现；根目录仅保留 db/saas.sql。开发期间不备份，不自动清除 SaaS 数据，不引入数据库外键/CHECK/AutoMigrate。
