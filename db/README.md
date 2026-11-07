# SaaS 数据库

`saas.sql` 是 PostgreSQL 16+ 的唯一完整建表文件，包含 SaaS、AI 配置、文档知识库和聊天共十二张表。所有表和字段都有中文数据库注释；SQL 不写入账号或密码。

| 表 | 用途 |
| --- | --- |
| `company` | 企业名称、访问域名、状态，以及本企业 AppID、明文 AppSecret 和微信配置版本 |
| `users` | 固定归属一家企业的成员及企业管理员；微信 AppID 与 OpenID 直接保存在用户行中 |
| `super_admin` | 独立于企业的平台主管理账号 |
| `auth_sessions` | 企业和平台两种身份的登录会话及访问令牌摘要 |
| `ai_models` | 企业页面配置的 Chat / Embedding 模型与服务端密钥 |
| `ai_settings` | 企业当前启用 LLM，以及新建知识库和聊天会话使用的默认模板 |
| `knowledge_vector_stores` | 企业页面配置的向量引擎与通用配置，注册模块声明字段与实际驱动 |
| `knowledge_bases` | 知识库及检索、分块、解析配置 |
| `knowledge_documents` | 资料原件引用、正文、处理状态、任务租约和处理代 |
| `knowledge_chunks` | 已处理分块、字符位置及当前索引版本 |
| `chat_sessions` | 按企业和用户隔离的聊天会话与独立配置 |
| `chat_messages` | 消息、生成状态和本次回答的来源快照 |

## 数据关系

图中连线只说明逻辑关联，不代表数据库外键。企业成员通过 `users.tenant_id` 归属企业；登录会话的 `scope` 决定 `subject_id` 关联哪种账号，不能同时关联两种身份。平台会话的 `tenant_id` 为 NULL。

物理表名使用 `company`，业务中的 tenant 仍表示企业租户；现有 `tenant_id` 字段逻辑关联 `company.id`，HTTP 路径与 DTO 继续沿用 tenant 名称。

```mermaid
erDiagram
    company ||--o{ users : "企业拥有成员；账号只能属于一家企业"
    company o|--o{ auth_sessions : "仅企业会话填写 tenant_id"
    users o|--o{ auth_sessions : "scope=tenant 时 subject_id 关联企业账号"
    super_admin o|--o{ auth_sessions : "scope=platform 时 subject_id 关联平台账号"
    company {
        bigint id PK "企业主键"
        varchar name "企业名称"
        varchar hostname UK "全局唯一访问域名"
        varchar status "active 正常或 disabled 停用"
        varchar app_id "企业小程序 AppID，未配置时为空"
        varchar app_secret "AppSecret 明文，仅平台授权配置接口回显"
        bigint wechat_config_version "未配置为0，配置更新递增，登录提交前复核"
        timestamptz created_at "创建时间"
        timestamptz updated_at "更新时间，由 Go 维护"
    }
    users {
        bigint id PK "企业账号主键"
        bigint tenant_id "固定归属企业的 ID"
        varchar username "企业内唯一登录名"
        varchar password_hash "密码散列；微信首登使用独立随机初始密码"
        varchar nickname "可编辑昵称，允许空字符串"
        varchar avatar_url "已上传头像路径或 HTTP(S) 地址，允许空字符串"
        varchar role "user 普通成员或 admin 企业管理员"
        varchar status "active 正常或 disabled 停用"
        varchar wechat_app_id "已验证微信身份所属 AppID，密码账号为空字符串"
        varchar open_id "微信返回的 OpenID，密码账号为空字符串，不公开"
        timestamptz created_at "创建时间"
        timestamptz updated_at "更新时间，由 Go 维护"
    }
    super_admin {
        bigint id PK "独立平台账号主键"
        varchar username UK "全局唯一平台登录名"
        varchar password_hash "密码散列，绝不公开"
        timestamptz created_at "创建时间"
        timestamptz updated_at "更新时间，由 Go 维护"
    }
    auth_sessions {
        bigint id PK "登录会话主键"
        varchar token_hash UK "令牌 SHA256 摘要，原始令牌不入库"
        varchar scope "tenant 企业身份或 platform 平台身份"
        bigint subject_id "按 scope 逻辑关联对应账号 ID"
        bigint tenant_id "仅企业身份填写，平台身份为 NULL"
        timestamptz expires_at "绝对到期时间"
        timestamptz created_at "签发时间"
    }
```

## 初始化

在空业务数据库中执行一次，在项目根目录运行：

```powershell
psql -X --set=ON_ERROR_STOP=1 --dbname=agent --file=.\db\saas.sql
```

SQL 在事务中创建 `public` 下的十二张表，失败整批回滚。同名表已经存在时会报错，不通过 `IF NOT EXISTS` 掩盖结构不一致。首个平台管理员使用后端的 `go run ./cmd/api bootstrap` 显式初始化到 `super_admin`，见 [服务端说明](../server/README.md)。

已有业务数据库应先核对现有结构，再显式执行与目标结构对应的 DDL；完整 SQL 只用于空库初始化。应用不执行 AutoMigrate，不自动重导、清空数据或备份。模型配置统一使用 `ai_models`，处理旧开发库时同时保留已有模型 ID 和相关引用，不能直接把整段新增表 SQL 当作通用迁移脚本。

## 知识库数据关系

以下仍是逻辑关联。`tenant_id` 明确每行的所属企业，便于按企业限定读取、处理和清理；并不由前端指定。分块与向量都需验证其企业、知识库、资料和处理代，避免单凭 ID 返回过时内容。

```mermaid
erDiagram
    company ||--o{ ai_models : "企业维护 Chat 与 Embedding 配置"
    company ||--o{ knowledge_vector_stores : "企业维护向量连接"
    company ||--o{ knowledge_bases : "企业拥有知识库"
    users ||--o{ knowledge_bases : "creator_id 记录创建者"
    ai_models o|--o{ knowledge_bases : "embedding_model_id 只允许 Embedding 用途"
    knowledge_vector_stores o|--o{ knowledge_bases : "向量模式选择连接"
    knowledge_bases ||--o{ knowledge_documents : "知识库包含资料"
    knowledge_documents ||--o{ knowledge_chunks : "完成的当前处理代生成分块"
    ai_models {
        bigint id PK "Go 生成，API 用字符串"
        bigint tenant_id "所属企业"
        varchar name "显示名称"
        varchar kind "chat 对话或 embedding 文本向量"
        varchar provider "服务商，映射到 OpenAI 兼容或 Ollama 协议"
        varchar endpoint "模型服务地址"
        varchar model_name "模型名"
        varchar api_key "模型服务密钥，授权配置接口返回原值"
        int dimension "Embedding 实际向量须匹配，Chat 为 0"
        timestamptz created_at "创建时间"
        timestamptz updated_at "更新时间"
    }
    knowledge_vector_stores {
        bigint id PK "向量连接主键"
        bigint tenant_id "所属企业"
        varchar name "显示名称"
        varchar provider "显式注册的向量引擎标识"
        jsonb config "完整字符串配置对象，授权接口包含凭据；驱动声明字段及索引身份"
        timestamptz created_at "创建时间"
        timestamptz updated_at "更新时间"
    }
    knowledge_bases {
        bigint id PK "知识库主键"
        bigint tenant_id "所属企业"
        bigint creator_id "创建者"
        varchar name "知识库名称"
        text description "用途说明"
        bigint embedding_model_id "关键词模式可为空"
        bigint vector_store_id "关键词模式可为空"
        varchar indexing_mode "keyword 关键词或 hybrid 关键词与向量索引"
        varchar retrieval_mode "默认查询策略 keyword、vector、hybrid"
        varchar chunk_strategy "auto 自动、recursive 递归、heading 标题结构"
        varchar parser_endpoint "PDF DocReader HTTP 地址"
        int chunk_size "分块目标字符数"
        int chunk_overlap "相邻块重叠字符数"
        timestamptz created_at "创建时间"
        timestamptz updated_at "更新时间"
    }
    knowledge_documents {
        bigint id PK "资料主键"
        bigint tenant_id "所属企业"
        bigint base_id "所属知识库"
        varchar title "资料标题"
        varchar source "manual 或 file"
        varchar file_name "原文件名，不作为路径"
        varchar file_type "原件 MIME 媒体类型"
        varchar object_key "统一对象存储 key"
        varchar file_hash "原件 SHA256"
        text content "解析正文，上限 2 MiB"
        varchar parse_status "queued、processing、completed、failed、deleting"
        text error_message "脱敏失败原因"
        varchar attempt "当前处理代，防止旧结果覆盖"
        text cleanup_targets "待清理 store_id 与 attempt 数组，保留旧连接归属"
        bigint index_store_id "当前处理代实际使用的向量连接，可为空"
        boolean use_chunks "是否采用人工编辑分块建索引"
        bigint file_size "原件字节数"
        bigint chunk_count "成功处理分块数量"
        timestamptz lease_until "worker 租约，可恢复超时任务"
        timestamptz created_at "创建时间"
        timestamptz updated_at "状态更新时间"
    }
    knowledge_chunks {
        bigint id PK "分块主键，同时用作向量点 ID"
        bigint tenant_id "所属企业"
        bigint base_id "所属知识库"
        bigint document_id "所属资料"
        varchar attempt "须匹配已完成资料的当前代"
        text content "分块正文"
        text context_header "标题层级上下文，与原文范围分开保存"
        int index "从 0 开始的顺序"
        int start "原文起始 Unicode 字符位置"
        int end "原文结束字符位置，不含末位"
        timestamptz created_at "本处理代创建时间"
    }
```

## 企业启用模型、默认模板与聊天关系

`auth_sessions` 是登录身份会话，`chat_sessions` 是用户对话，两者职责不同。Prompt、生成、检索及知识库处理参数只在创建时复制，修改模板不会修改已有对象。`ai_settings.chat_model_id` 则是企业唯一启用 LLM，每个新旧会话在下一次发送时重新读取；正在流式的请求使用开始时捕获的完整模型。`chat_sessions.model_id` 只由服务端记录最近生成模型快照，不作为客户端可写配置，也不锁定旧模型删除。消息保存引用内容快照，后续资料编辑、重建或删除不会改写历史引用。

```mermaid
erDiagram
    company ||--o| ai_settings : "每企业至多一份启用状态与新建模板"
    ai_models o|--o{ ai_settings : "chat_model_id 当前启用 LLM，可为空"
    ai_models o|--o{ ai_settings : "embedding_model_id 默认 Embedder，可为空"
    knowledge_vector_stores o|--o{ ai_settings : "默认向量连接，未配置可为空"
    company ||--o{ chat_sessions : "企业隔离，不能跨域名读取"
    users ||--o{ chat_sessions : "用户所有，企业管理员也不能读取他人对话"
    ai_models o|--o{ chat_sessions : "model_id 历史生成快照，模型删除不受此字段限制"
    chat_sessions ||--o{ chat_messages : "会话包含用户提问及助手回答"
    ai_settings {
        bigint tenant_id PK "逻辑关联 company.id"
        bigint chat_model_id "企业当前启用 LLM，所有会话下一次发送读取"
        bigint embedding_model_id "新知识库默认 Embedder"
        bigint vector_store_id "新知识库默认向量连接"
        text system_prompt "新会话默认 System Prompt"
        float temperature "生成温度，默认 0.7"
        int max_tokens "最大输出，默认 2048"
        int history_turns "携带完整历史轮数，默认 10"
        varchar retrieval_mode "默认查询策略，默认 hybrid"
        int retrieval_limit "命中片段上限，默认 6"
        float similarity_threshold "向量检索最低相似度，默认 0"
        varchar chunk_strategy "新库切分策略，默认 auto"
        int chunk_size "新库分块字符数，默认 512"
        int chunk_overlap "新库相邻块重叠，默认 80"
        varchar parser_endpoint "新库 PDF DocReader 地址"
    }
    chat_sessions {
        bigint id PK "对话主键，API 使用字符串"
        bigint tenant_id "所属企业，逻辑关联 company.id"
        bigint user_id "所属用户，逻辑关联同企业 users.id"
        varchar title "对话名称"
        bigint model_id "服务端最近生成模型的只读快照，不决定下一次模型"
        jsonb knowledge_base_ids "本企业知识库 ID 字符串数组"
        text system_prompt "本会话独立提示词，只影响后续回答"
        float temperature "本会话生成温度"
        int max_tokens "每次回答最大输出 token 数"
        int history_turns "携带的完整历史轮数"
        varchar retrieval_mode "本会话查询策略，须符合所选库能力"
        int retrieval_limit "本轮最多使用的知识片段数量"
        float similarity_threshold "仅用于向量检索，不混用关键词或融合分数"
        timestamptz created_at "创建时间"
        timestamptz updated_at "最后配置或消息更新时间"
    }
    chat_messages {
        bigint id PK "消息主键"
        bigint tenant_id "所属企业，与会话一致"
        bigint user_id "所属用户，与会话一致"
        bigint session_id "逻辑关联 chat_sessions.id"
        varchar role "user 提问或 assistant 回答"
        text content "正文，失败或停止时保留已生成部分"
        varchar status "generating、completed、failed、cancelled"
        jsonb sources "知识库、资料、分块、分数及引用正文快照"
        timestamptz created_at "消息创建时间"
    }
```

## 索引与业务约束

- 企业域名、平台登录名全局唯一；企业登录名按 `(tenant_id, username)` 唯一。
- 用户行上的微信身份按 `(tenant_id, wechat_app_id, open_id)` 唯一；唯一索引仅覆盖 `wechat_app_id` 和 `open_id` 均非空字符串的行，普通密码账号两项均为空字符串。首次登录直接原子创建带微信身份的普通成员和企业会话，之后复用同一账号。Web 只提供账号密码登录，成员账号由管理员创建；停用账号或企业均禁止登录。
- 微信首次创建的成员使用随机公开登录名和每账号独立的随机初始密码，数据库只保存 bcrypt 散列。小程序设置页通过新微信 code 验证同一用户后设置新密码；复登不会重置密码。AppSecret 直接明文保存在 `company.app_secret`，仅平台授权配置接口返回原值；不进入公共站点、账号或微信登录响应及日志。微信临时 `session_key` 不入库、不下发。
- `company.app_id / app_secret` 未配置时为空字符串，`wechat_config_version` 初始为 0，配置更新时递增。保存完整提交两项有效值，不继承旧值，空值拒绝；配置保存后运行时读取。登录时在微信外部验证前读取快照，写入账号与会话前再次校验租户和配置版本。企业改域名保留配置，微信身份按 AppID 区分，不自动合并不同小程序的身份。
- 成员索引支持企业内按创建时间分页；登录会话索引支持按主体、企业撤销和按到期时间清理。
- 主键及逻辑关联 ID 使用 `BIGINT`；公开 API 中 ID 使用十进制字符串，避免客户端精度丢失。
- AI 连接和密钥按企业归属，配置管理限企业管理员；已有授权读取接口返回模型 api_key 与完整向量 config。保存按提交完整替换，不继承旧凭据，空值按字段必填规则校验；凭据不进入静态包、浏览器持久化、未授权接口或日志。当前启用 LLM、默认 Embedder、知识库及旧索引清理等真实引用保护连接不被删除或原地更换协议、地址、模型、维度；历史会话的 model_id 快照不参与删除保护。更换连接需新建配置并显式切换引用；知识库已有资料时执行整库重建。
- `indexing_mode` 决定处理时准备的索引，`retrieval_mode` 决定查询已有索引的方式。切换查询策略无需重处理；改变模型、向量连接、解析或切分配置须显式重建。处理代与旧连接清理目标在同一事务中失效和排队，未完成新代不参与检索。
- `chat_sessions`、`chat_messages` 保留 `tenant_id` 和 `user_id`，用于每次按企业与所有者限定访问及清理，并由应用验证与父会话一致。它们是归属字段，不允许前端指定，也不依赖搜索结果判断权限。
- `updated_at` 的默认值仅在插入时生效，后续由 Go 显式更新。登录会话只记录创建和到期时间，撤销直接删除。
- 表之间没有数据库外键、CHECK、数据库枚举、触发器或 RLS。企业归属、状态、角色、至少一名有效企业管理员及会话撤销均由 Go 在事务中维护，不能信任客户端传来的企业或账号归属。

当前业务和接口见 [SaaS 说明](../docs/saas.md)。
