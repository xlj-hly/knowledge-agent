# SaaS、知识库与聊天服务

Go + Gin + GORM + PostgreSQL。包含独立平台管理、企业域名识别与账号、企业 AI 配置、文档知识库及流式聊天。业务与接口见 [SaaS 说明](../docs/saas.md) 和 [知识库与聊天说明](../docs/knowledge.md)。

## 代码结构

- `cmd/api/main.go`：API 进程入口、退出信号与错误退出。
- `internal/bootstrap`：加载本地 .env 和系统环境变量，组装数据库、业务用例、HTTP 及外部适配；管理过期登录会话清理、知识处理任务和单服务聊天生成恢复。各企业的微信凭据和 AI 连接由页面管理。
- `internal/domain/saas`：企业、账号与会话、纯业务规则、错误及领域存储契约，不依赖其他层，不携带 JSON 或 ORM 标签。
- `internal/app/saas/usecase`：认证、账号、企业、站点、维护与初始化用例，接受 `context.Context`，负责权限校验、事务与并发不变量。
- `internal/app/saas/dto`：用例输入与输出，通过明确映射维持现有 API 字段；领域对象不直接作为响应。
- `internal/app/saas/port`：事务协调与微信验证等用例所需契约；`internal/app/storage` 定义通用对象存储契约。
- `internal/domain/ai`、`knowledge`、`chat`：模型、企业启用状态与新建模板、知识资料与分块、用户对话与消息的规则及领域仓储契约。
- `internal/app/ai`、`knowledge`、`chat`：配置管理、解析索引、检索和生成用例，各自维护 DTO 及实际消费的外部端口；跨模块通过公开能力接入，不访问对方私有仓储。
- `internal/app/configuration`：企业配置逻辑引用的事务协调契约，避免连接删除和引用创建并发交错。
- `internal/infra/persistence/saas`：实现领域存储及应用事务契约，管理私有模型、GORM 操作及映射；所有企业用户与会话查询显式带企业范围。
- `internal/infra/database`：GORM PostgreSQL 连接与表结构就绪检查。数据库结构仅由 `db/saas.sql` 管理，不调用 AutoMigrate，不新增数据库外键。
- `internal/infra/wechat`：通用微信协议客户端及应用端口适配，负责 code2Session、等待期限与脱敏错误分类。
- `internal/infra/storage`：通用对象存储工厂与 filesystem、S3 兼容实现，不包含头像业务与 MinIO 产品逻辑。
- `internal/infra/ai`：显式模型注册、模型发现、调用测试、OpenAI 兼容与 Ollama 的 Embedding 和流式 Chat 协议。`providers` 按服务商维护类型化预设与内嵌资源，协议驱动统一处理真实调用；`presets` 包按 Prompt、切分方案和策略元数据分类维护产品预设，使用独立目录注入。
- `internal/infra/knowledge`：有界解析、WeKnora 切分、显式向量引擎注册及各引擎实际驱动；`internal/infra/persistence/{ai,knowledge,chat}` 实现相应仓储与私有 ORM 映射，`internal/infra/httpclient` 提供外部 HTTP 访问的公共实现。
- `internal/transport/http`：HTTP 解析、响应映射、路由和中间件，通过消费方能力接口调用用例，不执行 SQL、持有业务事务或组装适配器。
- `tests/integration`：标准库 `testing` / `httptest` 执行真实处理链路和随机 schema 的 PostgreSQL 流程；领域与服务局部测试随包保存。

依赖方向为 `transport → app → domain`，`infra` 实现内层定义的契约，`bootstrap` 负责装配。`bootstrap.New` 接受已打开的数据库，返回 HTTP 处理器和业务能力，不接管连接池；运行入口负责连接与关闭。集成测试通过同一装配入口执行真实 HTTP、业务、PostgreSQL 与文件存储链路，微信上游与对象存储故障分别注入替身。

写入 JSON 必须为单个对象，未知字段和显式 `null` 均拒绝。普通 SaaS 输入上限 16 KiB，AI 配置上限 64 KiB，知识与聊天输入另有有界请求体及字段限制，文件上传使用独立限制。个人资料使用 `PUT /account/profile`，昵称与头像都必须提供，空字符串表示清空；缺字段和空对象均拒绝。

## 初始化与启动

先在空业务数据库中执行一次 `db/saas.sql`。该文件创建 SaaS、AI、知识库与聊天共十二张表，执行方式及现有数据库处理见 [数据库说明](../db/README.md)。不自动迁移或清空已有数据。`company` 直接保存企业小程序配置，`users` 直接保存已验证的微信身份。

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

在 React 平台管理域名中登录，进入“企业管理 → 小程序配置”填写 AppID、AppSecret，直接保存到 `company.app_id / app_secret`；AppSecret 在数据库中为明文，不使用加密主密钥。没有微信登录开关，保存后后续登录立即读取新值。平台授权 GET/PUT 配置响应返回 `{app_id,app_secret}` 原值并回填表单；保存必须提交两项有效值，缺失或空值拒绝，不继承旧密钥。配置版本更新与登录提交前复核保留，企业换域名时配置仍归原企业所有。公共站点、用户资料和微信登录接口不返回 AppSecret 或微信临时 session_key。

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

## AI 配置、知识库与聊天

`/api/ai` 统一维护 Chat / Embedding 模型、服务商预设、模型发现与测试、企业新建模板及 Prompt/切分预设；旧 `/api/knowledge/models` 已移除。企业管理员通过无请求体的 `PUT /api/ai/models/:id/activate` 启用本企业 Chat 模型，返回完整 Settings；`ai_settings.chat_model_id` 是唯一启用状态。`GET/PUT /api/ai/settings` 响应保留该字段，保存输入不接受该字段，也不覆盖启用状态。`/api/knowledge` 管理向量连接、知识库、资料、分块、检索、无写入切分预览及整库重建。`/api/chat/sessions` 管理本用户对话，消息 POST 使用 SSE，`/:id/stop` 停止生成。精确路径与输入语义见 [知识库与聊天说明](../docs/knowledge.md)。

Prompt、生成、检索参数只在新建对话时复制；Embedder、向量连接、解析和切分参数只在新建知识库时复制，已有对象独立维护。启用 LLM 则在每次发送时读取，新旧会话的下一条消息都使用企业当前启用模型；流式请求捕获开始时的完整配置。会话输入不接受 `model_id`，输出保留最近生成模型的只读快照，快照不阻止删除旧模型；当前启用、默认 Embedder 和知识库等真实引用继续保护。知识库索引能力与查询策略分开；已有索引可切换查询策略，已有资料的模型、向量、解析和切分变更须确认重建。原件复用统一对象存储，旧索引保留原连接和处理代以恢复清理。

同一对话只能有一个生成，网络调用不占数据库长事务。回答、状态和来源快照保存在 PostgreSQL，停止或失败保留已有文本；刷新从服务端恢复。普通问题最多 16,000 字符，知识问答最多 2,000 字符。流式生成总期限 2 分钟，前端另有 150 秒无数据期限；代理应关闭 SSE 缓冲并允许覆盖生成期限的连接等待，避免把流式接口配置成普通短请求。

`bootstrap.New` 只组装依赖，运行入口负责后台任务启动、取消和等待，并在当前单服务启动时修复中断的生成状态；构造应用本身不启动后台 goroutine。模型预设不代表真实服务已部署，模型、Qdrant 或 DocReader 故障明确反馈，不伪造向量或静默降级。

模型注册由 `infra/ai.NewBuiltinRegistry` 创建，同一只读实例注入配置目录、用途校验、探测、聊天和 Embedding；向量由 `infra/knowledge.NewBuiltinVectors` 关联引擎定义与实际驱动。无效或重复注册在启动时返回错误。领域模型只做结构及业务规则校验，不维护服务商品牌到协议的映射；企业密钥只来自连接记录。

向量模块同步声明连接字段与实际校验，应用用例依赖统一 `port.Vectors`，页面读取 `fields` 动态渲染。配置采用字符串对象 `Config`，持久化为 JSONB，具体驱动解析专用类型。已有授权配置接口返回模型 `api_key` 与完整向量 config，表单按原值回填；模型和连接保存按提交完整替换，不继承旧凭据，空值按字段必填规则校验。发现和测试只使用当前表单，不按已存模型 ID 回读密钥。字段 `secret` 只声明输入展示及默认值约束，不过滤配置响应；凭据不进入静态包、未授权接口或日志。当前参考两个项目的页面选项注册 13 个远程引擎模块，同协议共享驱动。实际接口与引擎要求见 [知识库说明](../docs/knowledge.md)。

产品预设由 `infra/ai/presets.NewBuiltin` 创建，通过独立 `PresetCatalog` 注入 AI 用例，不由模型 Registry 持有。新增 Prompt 或切分参数方案分别修改 `prompts.go`、`chunking.go`，由 `catalog.go` 的 `NewBuiltin` 显式组装；构造时校验唯一 ID、模板内容、参数和策略引用。`GET /api/ai/presets` 的 `chunk_strategies` 提供已有算法的名称与说明，默认配置和知识库页面统一读取；新增参数方案不会增加执行算法。

扩展兼容服务商时，在 `infra/ai/providers` 增加定义与资源，并在 `Builtins` 显式登记；已有协议驱动无需新增品牌分支。专用协议实现 `Driver` 后在注册组装中接入。向量引擎通过 `Entry` 关联元数据、校验和 `port.Vectors` 实现。页面直接消费两个注册目录，不需要登记品牌；Chat 和知识处理用例无需随供应商增加修改。采用 Go 静态编译和明确依赖，不使用动态插件、全局 `init()` 注册或静默回退。

## 验证

```powershell
go test ./...
go vet ./...
# 显式选择可回收的测试数据库，不得指向生产库。
$env:SAAS_TEST_DATABASE_URL = 'postgres://用户:密码@localhost:端口/测试库?sslmode=disable'
go test ./... -count=1
go test ./tests/integration -count=1 -v
```

使用 Go 标准测试工具，不额外引入断言框架。真实数据库测试在随机 schema 内创建数据，结束后删除该 schema。不设置测试连接时，接口集成测试明确跳过，不能视为业务流程验收。除 SaaS 的企业、账号、微信和会话流程外，集成回归范围包括 AI 配置权限与引用、模板快照、LLM 启用与旧会话后续发送、流式请求捕获、仅剩历史快照的旧模型可删除、资料处理与重建、对话归属、引用持久化、失败、停止、并发生成和恢复。`react-app` 的 Playwright 通过 `tests/integration` 浏览器服务执行真实前后端闭环；服务只允许监听 `127.0.0.1:8081` 或 `127.0.0.1:8082`，必须携带本次 `E2E_RUN_ID` 完成实例握手和停止验证。

真实 Qdrant 生命周期另设 `TEST_QDRANT_ENDPOINT`，测试只创建并清理本次随机集合。模型协议与部分流程使用受控 HTTP 模型上游，业务 API 与 PostgreSQL 仍执行真实实现；这验证协议和编排，不等于实际模型效果或 DocReader 已通过验收。实际执行与跳过结果应分别报告。

真实 S3 回归单独设置 `TEST_S3_ENABLED=1`，并显式提供 `TEST_S3_ENDPOINT`、`TEST_S3_REGION`、`TEST_S3_BUCKET`、`TEST_S3_ACCESS_KEY_ID`、`TEST_S3_SECRET_ACCESS_KEY`、`TEST_S3_USE_PATH_STYLE`，不加载 `.env` 或使用部署凭据作为后备。`TestS3IntegrationObjectLifecycle` 验证对象读写、元数据、删除和匿名访问被拒绝；`TestSaaSAvatarS3PersistsAndEnforcesOwnership` 还需上述隔离 PostgreSQL 配置，贯通真实 S3、头像 HTTP 读取及 Range、归属校验、资料保存与会话读回。两者只清理本次创建的随机对象，不删除桶或其他对象。

## 部署约定

Docker 部署见 [部署说明](../deploy/README.md)，Compose 只运行 API 与 Web，数据库和对象存储均通过配置连接。

生产通过 HTTPS 和同域反向代理提供前端及 API，保留原始 Host；不要缓存业务 API。Bearer 不依赖 Cookie，React 保存在当前标签页 sessionStorage，uniapp 按企业域名存储。跨域部署才设置逗号分隔的 `ALLOWED_ORIGINS`。`PLATFORM_HOSTS` 只填写平台管理域名，不能绑定为企业域名。文件存储根目录或 S3 私有桶必须持久保存，丢失对象后已有头像地址无法读取。令牌有 7 天绝对有效期，数据库只存摘要，每小时清理过期会话；服务重启后会话仍然有效。

请求体读取、纯校验、密码验证与散列在写事务和业务锁之前完成；取得锁后重新核对凭据快照。常规企业请求持有企业共享锁，账号读写按对象加共享或排他锁；企业生命周期、管理员角色/状态和跨账号重置使用企业排他锁，按企业→账号顺序防止停用与最后管理员竞态。平台身份始终先于企业与账号锁；会话在锁后按 `clock_timestamp()` 再检查，等待不会延长有效期。

数据库连接必须只给服务使用，不能把凭据分发给客户端。GORM SQL 日志关闭，基础设施失败保留 500 语义并记录操作、脱敏 SQLSTATE 与受控对象存储错误类别，不记录查询参数、密码或令牌。业务拒绝伴随对象清理失败时仍保留业务响应，同时记录清理故障。

当前支持 Web 用户名密码与微信小程序登录。微信协议适配测试使用受控 HTTP 上游，数据库集成测试使用注入的微信身份验证替身，分别报告真实微信联调结果。
