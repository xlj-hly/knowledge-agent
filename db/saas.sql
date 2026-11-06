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
COMMENT ON COLUMN company.app_secret IS '本企业微信小程序 AppSecret，直接保存；禁止公开响应或写入日志';
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

COMMIT;
