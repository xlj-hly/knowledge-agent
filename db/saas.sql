-- SaaS 首版完整建表文件，适用于 PostgreSQL 16+ 的空业务数据库。
-- 企业、账号与登录会话均为逻辑关联；关系、权限和状态由 Go 事务校验。
-- 仅保留主键、业务唯一索引和查询索引，不创建外键、CHECK、枚举、触发器或 RLS。

BEGIN;
SET LOCAL search_path = public;

CREATE TABLE company (
    id                    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name                  VARCHAR(128) NOT NULL,
    hostname              VARCHAR(253) NOT NULL,
    status                VARCHAR(16) NOT NULL DEFAULT 'active',
    app_id                VARCHAR(64) NOT NULL DEFAULT '',
    app_secret            VARCHAR(256) NOT NULL DEFAULT '',
    wechat_config_version BIGINT NOT NULL DEFAULT 0,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX uk_company_hostname ON company (hostname);

COMMENT ON TABLE company IS '企业租户表，一家企业对应一个租户和一个访问域名';
COMMENT ON COLUMN company.id IS '企业主键，由数据库生成';
COMMENT ON COLUMN company.name IS '企业名称';
COMMENT ON COLUMN company.hostname IS '企业访问域名，全局唯一；Go 规范为小写且去除端口，拒绝协议、路径及非法域名';
COMMENT ON COLUMN company.status IS '企业状态：active 正常，disabled 停用；停用时由 Go 撤销全部企业会话';
COMMENT ON COLUMN company.app_id IS '本企业微信小程序 AppID，由后台配置；空字符串表示尚未配置';
COMMENT ON COLUMN company.app_secret IS '本企业微信小程序 AppSecret，直接保存；仅平台授权配置接口回显，禁止进入公共接口和日志';
COMMENT ON COLUMN company.wechat_config_version IS '配置变更时递增，微信验证提交前复核；未配置为 0';
COMMENT ON COLUMN company.created_at IS '创建时间，带时区';
COMMENT ON COLUMN company.updated_at IS '更新时间，后续修改由 Go 显式维护';

CREATE TABLE users (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id      BIGINT NOT NULL,
    username       VARCHAR(64) NOT NULL,
    password_hash  VARCHAR(255) NOT NULL,
    nickname       VARCHAR(64) NOT NULL DEFAULT '',
    avatar_url     VARCHAR(2048) NOT NULL DEFAULT '',
    wechat_app_id  VARCHAR(64) NOT NULL DEFAULT '',
    open_id        VARCHAR(128) NOT NULL DEFAULT '',
    role           VARCHAR(16) NOT NULL DEFAULT 'user',
    status         VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX uk_users_tenant_username ON users (tenant_id, username);
CREATE UNIQUE INDEX uk_users_wechat_identity ON users (tenant_id, wechat_app_id, open_id) WHERE wechat_app_id <> '' AND open_id <> '';
CREATE INDEX idx_users_tenant_created ON users (tenant_id, created_at DESC, id DESC);

COMMENT ON TABLE users IS '企业账号表，普通成员与企业管理员共用，一个账号固定属于一家企业';
COMMENT ON COLUMN users.id IS '账号主键，由数据库生成';
COMMENT ON COLUMN users.tenant_id IS '所属企业 ID，逻辑关联 company.id；查询和写入必须校验企业范围';
COMMENT ON COLUMN users.username IS '企业内唯一登录名，Go 规范为小写；不同企业允许同名';
COMMENT ON COLUMN users.password_hash IS '密码散列；微信首次创建使用每账号独立随机初始密码，后续微信登录不覆盖用户已改密码；禁止公开响应或写入日志';
COMMENT ON COLUMN users.nickname IS '可编辑昵称，空字符串表示未设置';
COMMENT ON COLUMN users.avatar_url IS '已上传头像的受控相对地址或有效 HTTP(S) 地址，空字符串表示未设置';
COMMENT ON COLUMN users.wechat_app_id IS '服务端核实的微信小程序 AppID；与 OpenID 一同识别微信身份，不公开';
COMMENT ON COLUMN users.open_id IS '微信返回的 OpenID，按企业和小程序唯一；空字符串表示未关联微信身份的账号，不公开';
COMMENT ON COLUMN users.role IS '账号角色：user 普通成员，admin 企业管理员；由 Go 保证至少一名有效管理员';
COMMENT ON COLUMN users.status IS '账号状态：active 正常，disabled 停用；取值与会话撤销由 Go 维护';
COMMENT ON COLUMN users.created_at IS '创建时间，带时区';
COMMENT ON COLUMN users.updated_at IS '更新时间，后续修改由 Go 显式维护';

CREATE TABLE super_admin (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username       VARCHAR(64) NOT NULL,
    password_hash  VARCHAR(255) NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX uk_super_admin_username ON super_admin (username);

COMMENT ON TABLE super_admin IS '独立平台管理员表，不属于企业，也不能作为企业账号使用';
COMMENT ON COLUMN super_admin.id IS '平台账号主键，由数据库生成';
COMMENT ON COLUMN super_admin.username IS '平台登录名，全局唯一，Go 规范为小写';
COMMENT ON COLUMN super_admin.password_hash IS '密码散列，禁止公开响应或写入日志';
COMMENT ON COLUMN super_admin.created_at IS '创建时间，带时区';
COMMENT ON COLUMN super_admin.updated_at IS '更新时间，后续修改由 Go 显式维护';

CREATE TABLE auth_sessions (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    token_hash  VARCHAR(64) NOT NULL,
    scope       VARCHAR(16) NOT NULL,
    subject_id  BIGINT NOT NULL,
    tenant_id   BIGINT,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX uk_auth_sessions_token_hash ON auth_sessions (token_hash);
CREATE INDEX idx_auth_sessions_subject ON auth_sessions (scope, subject_id);
CREATE INDEX idx_auth_sessions_tenant ON auth_sessions (tenant_id);
CREATE INDEX idx_auth_sessions_expiry ON auth_sessions (expires_at);

COMMENT ON TABLE auth_sessions IS '持久化登录会话表，仅保存随机令牌的 SHA256 摘要；撤销时直接删除';
COMMENT ON COLUMN auth_sessions.id IS '登录会话主键，由数据库生成';
COMMENT ON COLUMN auth_sessions.token_hash IS '访问令牌的 SHA256 十六进制摘要，原始令牌禁止入库';
COMMENT ON COLUMN auth_sessions.scope IS '身份范围：tenant 企业账号，platform 平台管理员；由 Go 校验';
COMMENT ON COLUMN auth_sessions.subject_id IS '逻辑关联账号 ID：scope 为 tenant 时指向 users.id，为 platform 时指向 super_admin.id';
COMMENT ON COLUMN auth_sessions.tenant_id IS '企业会话所属企业，逻辑关联 company.id 且须与账号企业一致；平台会话为 NULL';
COMMENT ON COLUMN auth_sessions.expires_at IS '绝对到期时间；等待锁不会延长有效期，Go 在取得锁后再次校验';
COMMENT ON COLUMN auth_sessions.created_at IS '签发时间，带时区';

-- 知识库结构（模型、向量连接、知识库、资料与分块；逻辑关联由 Go 校验）

CREATE TABLE ai_models (
    id          BIGINT PRIMARY KEY,
    tenant_id   BIGINT NOT NULL,
    name        VARCHAR(128) NOT NULL,
    provider    VARCHAR(64) NOT NULL,
    endpoint    VARCHAR(2048) NOT NULL,
    model_name  VARCHAR(256) NOT NULL,
    api_key     VARCHAR(4096) NOT NULL DEFAULT '',
    kind        VARCHAR(16) NOT NULL,
    dimension   INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_ai_models_tenant_created ON ai_models (tenant_id, created_at DESC, id DESC);

COMMENT ON TABLE ai_models IS '企业页面维护的 Chat 与 Embedding 模型连接配置';
COMMENT ON COLUMN ai_models.id IS '模型配置主键，由 Go 生成正 BIGINT，JSON 使用十进制字符串';
COMMENT ON COLUMN ai_models.tenant_id IS '所属企业 ID，逻辑关联 company.id；配置与密钥不能跨企业读取';
COMMENT ON COLUMN ai_models.name IS '页面显示的模型配置名称';
COMMENT ON COLUMN ai_models.provider IS '模型服务商标识，由服务商目录映射到实际调用协议';
COMMENT ON COLUMN ai_models.endpoint IS '页面配置的 HTTP(S) 服务地址，不允许内嵌凭据或查询参数';
COMMENT ON COLUMN ai_models.model_name IS '服务端实际调用的模型名称';
COMMENT ON COLUMN ai_models.api_key IS '模型服务访问密钥；授权配置接口返回原值，保存完整替换，空值按服务商规则校验；禁止进入未授权接口和日志';
COMMENT ON COLUMN ai_models.kind IS '模型用途：chat 对话或 embedding 文本向量，由 Go 校验能力';
COMMENT ON COLUMN ai_models.dimension IS 'Embedding 预期维度；chat 为 0；运行时不自动截断补齐';
COMMENT ON COLUMN ai_models.created_at IS '创建时间，带时区';
COMMENT ON COLUMN ai_models.updated_at IS '更新时间，由 Go 显式维护';

CREATE TABLE knowledge_vector_stores (
    id          BIGINT PRIMARY KEY,
    tenant_id   BIGINT NOT NULL,
    name        VARCHAR(128) NOT NULL,
    provider    VARCHAR(64) NOT NULL,
    config      JSONB NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_knowledge_vector_stores_tenant_created ON knowledge_vector_stores (tenant_id, created_at DESC, id DESC);

COMMENT ON TABLE knowledge_vector_stores IS '企业页面维护的向量数据库连接，业务通过统一向量存储端口访问';
COMMENT ON COLUMN knowledge_vector_stores.id IS '向量连接主键，由 Go 生成正 BIGINT';
COMMENT ON COLUMN knowledge_vector_stores.tenant_id IS '所属企业 ID，逻辑关联 company.id';
COMMENT ON COLUMN knowledge_vector_stores.name IS '页面显示的连接名称';
COMMENT ON COLUMN knowledge_vector_stores.provider IS '显式注册的向量引擎标识，配置与执行能力由对应模块提供';
COMMENT ON COLUMN knowledge_vector_stores.config IS '引擎完整配置字符串对象，授权配置接口包含凭据；保存完整替换，字段和索引身份由注册模块声明；凭据禁止进入未授权接口和日志';
COMMENT ON COLUMN knowledge_vector_stores.created_at IS '创建时间，带时区';
COMMENT ON COLUMN knowledge_vector_stores.updated_at IS '更新时间，由 Go 显式维护';


CREATE TABLE knowledge_bases (
    id                  BIGINT PRIMARY KEY,
    tenant_id           BIGINT NOT NULL,
    creator_id          BIGINT NOT NULL,
    name                VARCHAR(128) NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    embedding_model_id  BIGINT,
    vector_store_id     BIGINT,
    indexing_mode       VARCHAR(16) NOT NULL DEFAULT 'hybrid',
    retrieval_mode      VARCHAR(16) NOT NULL DEFAULT 'hybrid',
    chunk_strategy      VARCHAR(16) NOT NULL DEFAULT 'auto',
    parser_endpoint     VARCHAR(2048) NOT NULL DEFAULT '',
    chunk_size          INTEGER NOT NULL DEFAULT 512,
    chunk_overlap       INTEGER NOT NULL DEFAULT 80,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_knowledge_bases_tenant_created ON knowledge_bases (tenant_id, created_at DESC, id DESC);
CREATE INDEX idx_knowledge_bases_model ON knowledge_bases (tenant_id, embedding_model_id) WHERE embedding_model_id IS NOT NULL;
CREATE INDEX idx_knowledge_bases_vector_store ON knowledge_bases (tenant_id, vector_store_id) WHERE vector_store_id IS NOT NULL;

COMMENT ON TABLE knowledge_bases IS '企业知识库，独立管理资料与处理配置，供聊天和检索使用';
COMMENT ON COLUMN knowledge_bases.id IS '知识库主键，由 Go 生成正 BIGINT';
COMMENT ON COLUMN knowledge_bases.tenant_id IS '所属企业 ID，逻辑关联 company.id';
COMMENT ON COLUMN knowledge_bases.creator_id IS '创建者账号 ID，逻辑关联本企业 users.id';
COMMENT ON COLUMN knowledge_bases.name IS '知识库名称';
COMMENT ON COLUMN knowledge_bases.description IS '知识库说明，空字符串表示未填写';
COMMENT ON COLUMN knowledge_bases.embedding_model_id IS '逻辑关联本企业 ai_models.id，关键词模式允许 NULL；已有资料修改处理配置须显式重建';
COMMENT ON COLUMN knowledge_bases.vector_store_id IS '逻辑关联本企业 knowledge_vector_stores.id，关键词模式允许 NULL';
COMMENT ON COLUMN knowledge_bases.indexing_mode IS '索引能力：keyword 关键词或 hybrid 关键词与向量，向量失败不能静默降级';
COMMENT ON COLUMN knowledge_bases.retrieval_mode IS '默认查询策略 keyword、vector 或 hybrid；切换已有索引的查询方式不需重建';
COMMENT ON COLUMN knowledge_bases.chunk_strategy IS '切分策略 auto、recursive 或 heading；修改须显式重建已有资料';
COMMENT ON COLUMN knowledge_bases.parser_endpoint IS 'PDF 使用的 WeKnora DocReader HTTP 基地址，空字符串表示尚未配置；普通文本格式本地解析';
COMMENT ON COLUMN knowledge_bases.chunk_size IS '目标分块 Unicode 字符数，默认 512，由 Go 校验范围';
COMMENT ON COLUMN knowledge_bases.chunk_overlap IS '相邻分块目标重叠字符数，默认 80；必须小于分块长度';
COMMENT ON COLUMN knowledge_bases.created_at IS '创建时间，带时区';
COMMENT ON COLUMN knowledge_bases.updated_at IS '更新时间，由 Go 显式维护';

CREATE TABLE knowledge_documents (
    id             BIGINT PRIMARY KEY,
    tenant_id      BIGINT NOT NULL,
    base_id        BIGINT NOT NULL,
    title          VARCHAR(512) NOT NULL,
    source         VARCHAR(16) NOT NULL,
    file_name      VARCHAR(512) NOT NULL DEFAULT '',
    file_type      VARCHAR(128) NOT NULL DEFAULT '',
    object_key     VARCHAR(2048) NOT NULL DEFAULT '',
    file_hash      VARCHAR(64) NOT NULL DEFAULT '',
    content        TEXT NOT NULL DEFAULT '',
    parse_status   VARCHAR(16) NOT NULL DEFAULT 'queued',
    error_message  TEXT NOT NULL DEFAULT '',
    attempt        VARCHAR(64) NOT NULL,
    cleanup_targets TEXT NOT NULL DEFAULT '[]',
    index_store_id BIGINT,
    use_chunks     BOOLEAN NOT NULL DEFAULT FALSE,
    file_size      BIGINT NOT NULL DEFAULT 0,
    chunk_count    BIGINT NOT NULL DEFAULT 0,
    lease_until    TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_knowledge_documents_base_created ON knowledge_documents (tenant_id, base_id, created_at DESC, id DESC);
CREATE INDEX idx_knowledge_documents_queue ON knowledge_documents (updated_at, id) WHERE parse_status IN ('queued', 'processing', 'deleting');
CREATE INDEX idx_knowledge_documents_file_hash ON knowledge_documents (tenant_id, base_id, file_hash) WHERE file_hash <> '';

COMMENT ON TABLE knowledge_documents IS '知识库资料与持久化处理任务，原文件通过统一对象存储契约保存';
COMMENT ON COLUMN knowledge_documents.id IS '资料主键，由 Go 生成正 BIGINT';
COMMENT ON COLUMN knowledge_documents.tenant_id IS '所属企业 ID，逻辑关联 company.id，处理与检索始终限定企业';
COMMENT ON COLUMN knowledge_documents.base_id IS '所属知识库 ID，逻辑关联本企业 knowledge_bases.id';
COMMENT ON COLUMN knowledge_documents.title IS '资料标题，可由用户修改';
COMMENT ON COLUMN knowledge_documents.source IS '资料来源：文件上传或手工文本，具体取值由 Go 校验';
COMMENT ON COLUMN knowledge_documents.file_name IS '原始资料文件名，手工文本允许为空；不直接用作存储路径';
COMMENT ON COLUMN knowledge_documents.file_type IS '原文件 MIME 媒体类型，由 Go 按支持扩展名映射';
COMMENT ON COLUMN knowledge_documents.object_key IS '原文件对象存储 key，受企业和知识库命名空间约束；手工文本允许为空';
COMMENT ON COLUMN knowledge_documents.file_hash IS '上传文件内容的 SHA256 十六进制摘要，手工文本可为空';
COMMENT ON COLUMN knowledge_documents.content IS '解析后的 UTF-8 正文或手工输入正文，最大 2 MiB，由 Go 维护';
COMMENT ON COLUMN knowledge_documents.parse_status IS '处理状态：queued 等待，processing 处理中，completed 完成，failed 失败，deleting 清理中；由 Go 控制转换';
COMMENT ON COLUMN knowledge_documents.error_message IS '可公开的脱敏处理失败原因，不保存服务密钥或上游原始响应';
COMMENT ON COLUMN knowledge_documents.attempt IS '当前处理代标识；提交结果须比较状态及处理代，旧任务不能覆盖新代';
COMMENT ON COLUMN knowledge_documents.cleanup_targets IS '待清理向量 JSON 数组，每项保留 store_id 与 attempt，跨连接重建后仍可回收';
COMMENT ON COLUMN knowledge_documents.index_store_id IS '当前处理代实际向量连接快照，逻辑关联本企业 knowledge_vector_stores.id；无向量为 NULL';
COMMENT ON COLUMN knowledge_documents.use_chunks IS '本次重处理是否采用用户已编辑分块，避免覆盖人工修改；由 Go 设置';
COMMENT ON COLUMN knowledge_documents.file_size IS '原文件字节数，手工文本为相应输入大小；上传上限由 Go 校验';
COMMENT ON COLUMN knowledge_documents.chunk_count IS '当前成功处理代的分块数量，完成提交时与分块同事务更新';
COMMENT ON COLUMN knowledge_documents.lease_until IS 'worker 领取任务的租约期限，NULL 表示未持有有效租约；过期任务可重新领取';
COMMENT ON COLUMN knowledge_documents.created_at IS '创建时间，带时区';
COMMENT ON COLUMN knowledge_documents.updated_at IS '状态或内容更新时间，同时用于后台任务领取顺序';

CREATE TABLE knowledge_chunks (
    id           BIGINT PRIMARY KEY,
    tenant_id    BIGINT NOT NULL,
    base_id      BIGINT NOT NULL,
    document_id  BIGINT NOT NULL,
    attempt      VARCHAR(64) NOT NULL,
    content      TEXT NOT NULL,
    context_header TEXT NOT NULL DEFAULT '',
    "index"      INTEGER NOT NULL,
    "start"      INTEGER NOT NULL,
    "end"        INTEGER NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_knowledge_chunks_document ON knowledge_chunks (tenant_id, document_id, attempt, "index");
CREATE INDEX idx_knowledge_chunks_base ON knowledge_chunks (tenant_id, base_id);
CREATE INDEX idx_knowledge_chunks_fts ON knowledge_chunks USING GIN (to_tsvector('simple', content));

COMMENT ON TABLE knowledge_chunks IS '资料的当前处理代分块；业务库保存原文，向量数据库仅保存可重建索引';
COMMENT ON COLUMN knowledge_chunks.id IS '分块主键，每次处理新建正 BIGINT；同时作为向量点 ID，旧代命中须经业务库复核';
COMMENT ON COLUMN knowledge_chunks.tenant_id IS '所属企业 ID，逻辑关联 company.id；同时用于关键词及向量命中归属校验';
COMMENT ON COLUMN knowledge_chunks.base_id IS '所属知识库 ID，逻辑关联同企业 knowledge_bases.id';
COMMENT ON COLUMN knowledge_chunks.document_id IS '所属资料 ID，逻辑关联同企业同知识库 knowledge_documents.id';
COMMENT ON COLUMN knowledge_chunks.attempt IS '生成该分块的处理代，检索时必须等于所属已完成资料的当前处理代';
COMMENT ON COLUMN knowledge_chunks.content IS '分块正文，可在页面编辑，向量内容须重新索引后生效';
COMMENT ON COLUMN knowledge_chunks.context_header IS '标题层级上下文，向量化与问答时和正文组合，不改变原文字符范围';
COMMENT ON COLUMN knowledge_chunks."index" IS '资料内从 0 开始的分块顺序';
COMMENT ON COLUMN knowledge_chunks."start" IS '在解析正文中从 0 开始的 Unicode 字符位置，不是 UTF-8 字节偏移';
COMMENT ON COLUMN knowledge_chunks."end" IS '正文范围结束的 Unicode 字符位置，不含该字符；人工编辑后的展示偏移由 Go 维护';
COMMENT ON COLUMN knowledge_chunks.created_at IS '本处理代分块创建时间，带时区';

CREATE TABLE ai_settings (
    tenant_id BIGINT PRIMARY KEY,
    chat_model_id BIGINT,
    embedding_model_id BIGINT,
    vector_store_id BIGINT,
    system_prompt TEXT NOT NULL,
    temperature DOUBLE PRECISION NOT NULL,
    max_tokens INTEGER NOT NULL,
    history_turns INTEGER NOT NULL,
    retrieval_mode VARCHAR(16) NOT NULL,
    retrieval_limit INTEGER NOT NULL,
    similarity_threshold DOUBLE PRECISION NOT NULL,
    chunk_strategy VARCHAR(16) NOT NULL,
    chunk_size INTEGER NOT NULL,
    chunk_overlap INTEGER NOT NULL,
    parser_endpoint VARCHAR(2048) NOT NULL DEFAULT ''
);
COMMENT ON TABLE ai_settings IS '企业新建模板，保存不会静默修改已有知识库和会话';
COMMENT ON COLUMN ai_settings.tenant_id IS '所属企业，逻辑关联 company.id，每企业至多一份模板';
COMMENT ON COLUMN ai_settings.chat_model_id IS '本企业当前启用的 LLM，逻辑关联 ai_models.id；所有对话下一条消息使用此模型，未启用为 NULL';
COMMENT ON COLUMN ai_settings.embedding_model_id IS '默认文本向量模型，逻辑关联本企业 ai_models.id，未配置为 NULL';
COMMENT ON COLUMN ai_settings.vector_store_id IS '默认向量连接，逻辑关联本企业 knowledge_vector_stores.id，未配置为 NULL';
COMMENT ON COLUMN ai_settings.system_prompt IS '新建会话复制的 System Prompt，空字符串表示不添加企业提示词';
COMMENT ON COLUMN ai_settings.temperature IS '新建会话的生成温度，由 Go 校验 0–2';
COMMENT ON COLUMN ai_settings.max_tokens IS '新建会话的最大生成 token 数';
COMMENT ON COLUMN ai_settings.history_turns IS '新建会话携带的历史轮数';
COMMENT ON COLUMN ai_settings.retrieval_mode IS '新建会话与知识库默认检索策略';
COMMENT ON COLUMN ai_settings.retrieval_limit IS '默认检索片段数量';
COMMENT ON COLUMN ai_settings.similarity_threshold IS '新建会话的向量相似度阈值';
COMMENT ON COLUMN ai_settings.chunk_strategy IS '新建知识库的切分策略';
COMMENT ON COLUMN ai_settings.chunk_size IS '新建知识库的分块字符数';
COMMENT ON COLUMN ai_settings.chunk_overlap IS '新建知识库的分块重叠字符数';
COMMENT ON COLUMN ai_settings.parser_endpoint IS '新建知识库的 PDF DocReader 服务基地址';

CREATE TABLE chat_sessions (
    id BIGINT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    title VARCHAR(100) NOT NULL,
    model_id BIGINT,
    knowledge_base_ids JSONB NOT NULL DEFAULT '[]',
    system_prompt TEXT NOT NULL,
    temperature DOUBLE PRECISION NOT NULL,
    similarity_threshold DOUBLE PRECISION NOT NULL,
    max_tokens INTEGER NOT NULL,
    history_turns INTEGER NOT NULL,
    retrieval_limit INTEGER NOT NULL,
    retrieval_mode VARCHAR(16) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_chat_sessions_user_updated ON chat_sessions (tenant_id,user_id,updated_at DESC,id DESC);
CREATE INDEX idx_chat_sessions_model ON chat_sessions (tenant_id,model_id) WHERE model_id IS NOT NULL;
COMMENT ON TABLE chat_sessions IS '用户所属的聊天会话，配置从企业默认模板复制后独立维护';
COMMENT ON COLUMN chat_sessions.id IS '会话主键，由 Go 生成正 BIGINT';
COMMENT ON COLUMN chat_sessions.tenant_id IS '所属企业，逻辑关联 company.id，所有访问限定企业';
COMMENT ON COLUMN chat_sessions.user_id IS '会话所有者，逻辑关联本企业 users.id，管理员不自动获得其他用户会话';
COMMENT ON COLUMN chat_sessions.title IS '会话显示名称';
COMMENT ON COLUMN chat_sessions.model_id IS '最近一次生成采用的模型 ID 快照，服务端写入，不作为模型选择或删除引用；尚未生成时为 NULL';
COMMENT ON COLUMN chat_sessions.knowledge_base_ids IS '所选本企业知识库 ID 字符串数组，每次使用复核可访问性';
COMMENT ON COLUMN chat_sessions.system_prompt IS '当前会话 System Prompt，只影响后续回答';
COMMENT ON COLUMN chat_sessions.temperature IS '当前会话的生成温度';
COMMENT ON COLUMN chat_sessions.similarity_threshold IS '向量命中最低分，关键词及融合分不混用此阈值';
COMMENT ON COLUMN chat_sessions.max_tokens IS '每次回答的最大输出 token 数';
COMMENT ON COLUMN chat_sessions.history_turns IS '携带的完整历史轮数';
COMMENT ON COLUMN chat_sessions.retrieval_limit IS '本轮最多使用的知识片段数量';
COMMENT ON COLUMN chat_sessions.retrieval_mode IS '检索策略 keyword、vector 或 hybrid，必须符合所选库索引能力';
COMMENT ON COLUMN chat_sessions.created_at IS '会话创建时间，带时区';
COMMENT ON COLUMN chat_sessions.updated_at IS '最近会话配置或消息更新时间';

CREATE TABLE chat_messages (
    id BIGINT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    session_id BIGINT NOT NULL,
    role VARCHAR(16) NOT NULL,
    content TEXT NOT NULL,
    status VARCHAR(16) NOT NULL,
    sources JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_chat_messages_session_created ON chat_messages (tenant_id,user_id,session_id,created_at,id);
CREATE INDEX idx_chat_messages_generating ON chat_messages (session_id) WHERE status='generating';
COMMENT ON TABLE chat_messages IS '聊天消息与引用快照，生成中、完成、失败和取消均有明确状态';
COMMENT ON COLUMN chat_messages.id IS '消息主键，由 Go 生成正 BIGINT';
COMMENT ON COLUMN chat_messages.tenant_id IS '所属企业，逻辑关联 company.id，与会话归属一致';
COMMENT ON COLUMN chat_messages.user_id IS '消息所属用户，逻辑关联本企业 users.id，与会话所有者一致';
COMMENT ON COLUMN chat_messages.session_id IS '所属会话，逻辑关联 chat_sessions.id';
COMMENT ON COLUMN chat_messages.role IS 'user 用户提问或 assistant 助手回答';
COMMENT ON COLUMN chat_messages.content IS '消息正文；生成失败或取消时保留已产生的文本';
COMMENT ON COLUMN chat_messages.status IS 'generating、completed、failed 或 cancelled，由 Go 管理状态转换';
COMMENT ON COLUMN chat_messages.sources IS '本次回答的引用内容快照，包含知识库、资料、分块及分数，刷新可恢复';
COMMENT ON COLUMN chat_messages.created_at IS '消息创建时间，带时区';

COMMIT;
