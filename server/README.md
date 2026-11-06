# SaaS 服务

Go + Gin + GORM + PostgreSQL。当前包含独立平台管理、企业域名识别、账号和企业管理员。业务及 API 契约见 [SaaS 说明](../docs/saas.md)。

## 代码结构

- `cmd/api/main.go`：API 进程入口、退出信号与错误退出。
- `internal/bootstrap`：自动加载本地 .env 和系统环境变量，组装数据库、业务用例、HTTP、微信与对象存储，管理启动、关闭和过期会话清理。各企业的微信凭据由后台管理。
- `internal/domain/saas`：企业、账号与会话、纯业务规则、错误及领域存储契约，不依赖其他层，不携带 JSON 或 ORM 标签。
- `internal/app/saas/usecase`：认证、账号、企业、站点、维护与初始化用例，接受 `context.Context`，负责权限校验、事务与并发不变量。
- `internal/app/saas/dto`：用例输入与输出，通过明确映射维持现有 API 字段；领域对象不直接作为响应。
- `internal/app/saas/port`：事务协调与微信验证等用例所需契约；`internal/app/storage` 定义通用对象存储契约。
- `internal/infra/persistence/saas`：实现领域存储及应用事务契约，管理私有模型、GORM 操作及映射；所有企业用户与会话查询显式带企业范围。
- `internal/infra/database`：GORM PostgreSQL 连接与表结构就绪检查。数据库结构仅由 `db/saas.sql` 管理，不调用 AutoMigrate，不新增数据库外键。
- `internal/infra/wechat`：通用微信协议客户端及应用端口适配，负责 code2Session、等待期限与脱敏错误分类。
- `internal/infra/storage`：通用对象存储工厂与 filesystem、S3 兼容实现，不包含头像业务与 MinIO 产品逻辑。
- `internal/transport/http`：HTTP 解析、响应映射、路由和中间件，通过消费方能力接口调用用例，不执行 SQL、持有业务事务或组装适配器。
- `tests/integration`：标准库 `testing` / `httptest` 执行真实处理链路和随机 schema 的 PostgreSQL 流程；领域与服务局部测试随包保存。

依赖方向为 `transport → app → domain`，`infra` 实现内层定义的契约，`bootstrap` 负责装配。`bootstrap.New` 接受已打开的数据库，返回 HTTP 处理器和业务能力，不接管连接池；运行入口负责连接与关闭。集成测试通过同一装配入口执行真实 HTTP、业务、PostgreSQL 与文件存储链路，微信上游与对象存储故障分别注入替身。

写入 JSON 必须是对象；未知字段、字段 `null` 和超出 16 KiB 的请求均拒绝。资料使用 `PUT /account/profile`，昵称与头像两个字段都必须提供，空字符串表示清空；缺字段和空对象均拒绝且不写入。

## 初始化与启动

先在空业务数据库中执行一次 `db/saas.sql`。该文件完整创建 `company / users / super_admin / auth_sessions` 四张表；执行方式见 [数据库说明](../db/README.md)。`company` 直接保存企业小程序配置，`users` 直接保存已验证的微信身份。

在 `server` 目录从示例创建本地配置，填写实际数据库连接、平台域名与对象存储配置。`.env` 不提交到仓库：

```powershell
Copy-Item .env.example .env
```

首次初始化时通过环境变量传入平台管理员凭据（示例密码必须替换）：

```powershell
$env:BOOTSTRAP_USERNAME = 'platform.admin'
$env:BOOTSTRAP_PASSWORD = '一个独立的强密码至少10字节'
go run ./cmd/api bootstrap
Remove-Item Env:BOOTSTRAP_PASSWORD
go run ./cmd/api
```

连接密码含特殊字符时需要 URL 编码。程序保留已有系统环境变量，并按顺序加载首个存在的配置文件：当前目录 `.env`、当前目录 `server/.env`、可执行文件同目录 `.env`。文件缺失时仍可只用环境变量启动；格式错误会明确报错且不回显配置值。初始化命令只在 `super_admin` 没有平台管理员时创建账号，不会覆盖已有密码。没有默认账号和默认密码。完成本地配置与初始化后，在 `server` 目录执行 `go run ./cmd/api` 即可启动；可用 `LISTEN_ADDR` 改监听地址，默认 `:8080`。

React 通过 `GET /api/site` 识别入口：平台管理域名访问 `/platform`，企业域名访问 `/`，未知域名不提供登录。平台先创建企业及首个管理员，企业 hostname 必须与入口访问域名一致；本地可使用 `localhost` 或 `企业.localhost`，平台使用 `platform.localhost`。开发端通过 Vite 的 `/api` 代理访问本服务并保留 Host。小程序配置见 uniapp README。

Web 端只提供账号密码登录，成员账号由企业管理员创建；微信首次登录自动创建本企业普通成员。企业管理账号角色和状态，并至少保留一名有效管理员；企业或账号停用、账号降权、改密、密码重置均撤销对应会话。平台可重置现有企业管理员密码。忘记普通用户密码时由企业管理员重置。所有关系是逻辑外键，没有数据库外键约束。

## 微信小程序登录

每家企业使用自己的小程序与 API 域名；HTTPS 反向代理转发到同一 SaaS 后端并保留 Host。后端通过 Host 查找企业，再按 tenant_id 读取该企业的小程序配置。客户端不能通过自定义企业头、AppID 或 OpenID 选择身份。

在 React 平台管理域名中登录，进入“企业管理 → 小程序配置”填写 AppID、AppSecret，直接保存到 `company.app_id / app_secret`；AppSecret 在数据库中为明文，不使用加密主密钥。没有微信登录开关，保存后后续登录立即读取新值。GET/PUT 响应只返回 `{app_id,has_secret}`，不回显密钥；同一 AppID 留空保留已有密钥，更换 AppID 必须填写新密钥。企业换域名时配置仍归原企业所有。

小程序调用 `uni.login` 取得一次性 code，再向 `POST /api/auth/wechat/login` 提交 `{"code":"..."}`。后端验证微信身份后直接查找或创建本企业普通成员并签发会话。新成员的初始密码每账号独立随机生成，仅保存散列且不下发；重复登录不覆盖已设置的密码，也不凭昵称或头像合并现有账号。

小程序设置页使用 `POST /api/account/wechat/password`，提交新 code 和新密码，服务端核对该微信身份就是当前登录用户后改密并撤销其会话。用户只填写新密码及确认，无需知道随机初始密码。Web 密码修改继续要求当前密码。

头像通过微信 `chooseAvatar` 选择，再以 `POST /api/account/avatar` 上传（multipart 字段 `file`，PNG/JPEG，不超过 2 MiB），保存到所选对象存储并返回本用户受控地址。昵称使用 `type="nickname"` 填写能力；资料通过现有 PUT 接口保存，不增加首次登录资料门槛。

微信验证在数据库事务外执行，提交前复核企业归属、状态与 `company.wechat_config_version`；验证期间发生配置变更时要求重新获取 code。微信身份直接写入 `users.wechat_app_id / open_id`，首次成员和会话在同一事务内提交。同一微信身份的并发首登通过事务锁串行化。后端不保存或返回微信 session_key，不自动重试已可能消费的 code。

协议依据：[微信小程序登录](https://developers.weixin.qq.com/miniprogram/dev/framework/open-ability/login.html)、[code2Session](https://developers.weixin.qq.com/miniprogram/dev/server/API/user-login/api_code2session.html)。

## 对象存储

`STORAGE_PROVIDER` 选择 `filesystem` 或 `s3`。本地文件实现使用 `STORAGE_FILESYSTEM_ROOT`；S3 实现使用标准 S3 API，可连接 AWS S3、MinIO 及其他兼容服务。业务只依赖通用 `Put / Get / Stat / Delete` 契约，头像校验、对象 key 与企业/用户归属由应用用例负责。

| 环境变量 | 用途 |
| --- | --- |
| `STORAGE_PROVIDER` | 存储实现，缺省 `filesystem` |
| `STORAGE_FILESYSTEM_ROOT` | 文件存储持久根目录，缺省 `.local/objects` |
| `S3_ENDPOINT` | S3 兼容服务地址，AWS S3 可留空使用 SDK 区域端点 |
| `S3_REGION` | 签名区域，缺省 `us-east-1`，需与对象服务配置一致 |
| `S3_BUCKET` | 已存在的私有桶，例如 `agent` |
| `S3_ACCESS_KEY_ID`、`S3_SECRET_ACCESS_KEY` | 仅服务端使用的访问凭据 |
| `S3_USE_PATH_STYLE` | `true / false`，MinIO 等本地服务通常使用 `true` |

本机 MinIO 的 S3 端口为 `9002`，配置 `STORAGE_PROVIDER=s3`、`S3_ENDPOINT=http://127.0.0.1:9002`、`S3_REGION=us-east-1`、`S3_BUCKET=agent`、`S3_USE_PATH_STYLE=true`，并填写专用服务端凭据。当前本机已创建私有 `agent` 桶，凭据位于忽略的 `.env`；其他环境应先创建私有桶并授予服务账号所需对象读写权限，应用不自动建桶。桶保持私有，由 API 读取并返回图片，客户端无需取得 S3 凭据或公开桶权限。

头像 `avatar_url` 继续使用 `/api/avatars/:tenant/:user/:file`，数据库不保存 S3 endpoint、桶地址或有期限的预签名 URL。切换 provider 不自动搬迁已有对象，已有图片应先迁入新后端对应 key，再切换配置。旧 `AVATAR_DIR` 配置改为 `STORAGE_FILESYSTEM_ROOT`。

## 验证

```powershell
go test ./...
go vet ./...
# 显式选择可回收的测试数据库，不得指向生产库。
$env:SAAS_TEST_DATABASE_URL = 'postgres://用户:密码@localhost:端口/测试库?sslmode=disable'
go test ./... -count=1
go test ./tests/integration -count=1 -v
```

使用 Go 标准测试工具，不额外引入断言框架。真实数据库测试在随机 schema 内创建数据，结束后删除该 schema。不设置测试连接时，接口集成测试会明确跳过；此时只有快速校验与 HTTP 边界测试，不能当作业务流程验收。集成覆盖企业创建与跨企业权限、管理员建号、微信首次登录建号、资料与改密、停用恢复、最后管理员并发保护、会话锁等待过期，以及 JSON 空值、共享锁和数据库故障语义。`react-app` 的 Playwright 通过 `tests/integration` 浏览器服务执行真实前后端闭环；服务只允许监听 `127.0.0.1:8081` 或 `127.0.0.1:8082`，必须携带本次 `E2E_RUN_ID` 完成实例握手和停止验证。

真实 S3 回归单独设置 `TEST_S3_ENABLED=1`，并显式提供 `TEST_S3_ENDPOINT`、`TEST_S3_REGION`、`TEST_S3_BUCKET`、`TEST_S3_ACCESS_KEY_ID`、`TEST_S3_SECRET_ACCESS_KEY`、`TEST_S3_USE_PATH_STYLE`，不加载 `.env` 或使用部署凭据作为后备。`TestS3IntegrationObjectLifecycle` 验证对象读写、元数据、删除和匿名访问被拒绝；`TestSaaSAvatarS3PersistsAndEnforcesOwnership` 还需上述隔离 PostgreSQL 配置，贯通真实 S3、头像 HTTP 读取及 Range、归属校验、资料保存与会话读回。两者只清理本次创建的随机对象，不删除桶或其他对象。

## 部署约定

Docker 部署见 [部署说明](../deploy/README.md)，Compose 只运行 API 与 Web，数据库和对象存储均通过配置连接。

生产通过 HTTPS 和同域反向代理提供前端及 API，保留原始 Host；不要缓存业务 API。Bearer 不依赖 Cookie，React 保存在当前标签页 sessionStorage，uniapp 按企业域名存储。跨域部署才设置逗号分隔的 `ALLOWED_ORIGINS`。`PLATFORM_HOSTS` 只填写平台管理域名，不能绑定为企业域名。文件存储根目录或 S3 私有桶必须持久保存，丢失对象后已有头像地址无法读取。令牌有 7 天绝对有效期，数据库只存摘要，每小时清理过期会话；服务重启后会话仍然有效。

请求体读取、纯校验、密码验证与散列在写事务和业务锁之前完成；取得锁后重新核对凭据快照。常规企业请求持有企业共享锁，账号读写按对象加共享或排他锁；企业生命周期、管理员角色/状态和跨账号重置使用企业排他锁，按企业→账号顺序防止停用与最后管理员竞态。平台身份始终先于企业与账号锁；会话在锁后按 `clock_timestamp()` 再检查，等待不会延长有效期。

数据库连接必须只给服务使用，不能把凭据分发给客户端。GORM SQL 日志关闭，基础设施失败保留 500 语义并记录操作、脱敏 SQLSTATE 与受控对象存储错误类别，不记录查询参数、密码或令牌。业务拒绝伴随对象清理失败时仍保留业务响应，同时记录清理故障。

当前支持 Web 用户名密码与微信小程序登录。微信协议适配测试使用受控 HTTP 上游，数据库集成测试使用注入的微信身份验证替身，分别报告真实微信联调结果。
