# 企业管理工作台

React 19、TypeScript、Vite 8、React Router、Zustand 5。平台管理员与企业成员使用独立入口和会话。业务范围和 API 契约见 [SaaS 说明](../docs/saas.md)。

Docker 使用 Node 构建静态文件、Nginx 提供页面与同域 API 转发，详见 [部署说明](../deploy/README.md)。

## 启动

```powershell
npm install
npm run dev
```

Vite 将 `/api` 代理到 `http://127.0.0.1:8080`，保留 Host。需要其他地址时在启动前设置：

```powershell
$env:API_PROXY_TARGET = 'http://127.0.0.1:8081'
npm run dev -- --host 0.0.0.0
```

浏览器先通过 `/api/site` 发现当前域名对应的平台或企业入口；后端根据实际 HTTP Host 识别，客户端不发送租户选择头。企业域名不能进入平台管理，未绑定域名不提供登录。本地可将 `platform.localhost` 配入后端平台域名白名单，用 `localhost` 或其他 `.localhost` 子域名开通企业；开发代理及生产代理均需保留 Host。`location.hostname` 只用于隔离当前域名的会话存储。

## 路由

| 地址 | 入口与权限 |
| --- | --- |
| `/login` | 企业成员登录 |
| `/` | 企业工作台 |
| `/account` | 企业成员个人资料、密码 |
| `/members` | 企业管理员管理成员、角色、状态与密码 |
| `/settings` | 企业管理员维护企业信息 |
| `/platform/login` | 独立平台管理员登录 |
| `/platform` | 平台工作台 |
| `/platform/tenants` | 开通、维护企业，配置小程序，重置企业管理员密码 |
| `/platform/account` | 平台账号与密码 |

BrowserRouter 以 URL 作为页面状态；导航使用 Link / NavLink，支持直接访问、刷新、前进与后退。未登录访问业务地址会进入对应登录页，登录后只恢复白名单内且属于当前身份空间的地址。普通成员直接访问管理页显示无权访问；未知地址显示页面不存在，并提供返回工作台的入口。前端路由守卫用于界面组织，不能代替服务端权限检查。

平台首位管理员需要后端显式 bootstrap，没有默认账号和密码。平台路由仅在平台域名开放，该域名的根地址进入 `/platform`。平台会话与企业会话按域名分别保存，切换身份空间会销毁旧请求与页面状态。

平台企业列表中的“小程序配置”维护对应企业的 AppID 与 AppSecret，没有额外登录开关。读取只返回 AppID 与是否已配置密钥，密钥不回填且不持久化到浏览器；留空保留原密钥，首次配置或更换 AppID 必须提供新密钥。保存后根据服务端确认刷新，重新打开时读取当前配置。

## 代码组织

- `src/main.tsx`、`src/App.tsx`：浏览器挂载、BrowserRouter 与身份空间组装。
- `src/router`：登录与受保护路由、权限入口和导航规则。
- `src/layouts`：应用布局、导航展示与布局样式；由路由层传入导航模型。
- `src/pages`：登录、工作台、账号、企业管理、成员与设置的完整页面；专属表单、弹窗、样式和测试跟随页面。
- `src/api`：按认证、账号、成员和企业划分接口及类型、解码；请求层统一使用 fetch 处理超时、错误、认证与企业上下文。
- `src/stores/session`：按身份空间创建的 Zustand 会话 Store、持久化、Hook 与身份类型。
- `src/components`、`src/hooks`、`src/utils`：跨页面组件、异步生命周期及具有明确用途的公共函数。

路由组合完整页面与布局，页面调用 API、会话和公共组件。页面之间不读取私有实现；接口类型与响应解码由相应 API 维护，跨页面使用的账号校验具有独立维护位置。请求层通过参数与失效回调接入会话，不反向导入 Store。布局只消费导航模型，不依赖导入页面的路由注册模块。

Zustand Store 拥有内存会话，`persist` 配合 `createJSONStorage` 将令牌保存到当前标签页的 sessionStorage，平台与不同企业使用独立键。持久化只白名单保存 `token`，身份资料由后端重新确认；版本号、提示、请求状态、表单与密码草稿不持久化。恢复只接纳有效的令牌字段，存储内容不能覆盖动作和运行时状态。表单和弹窗继续使用组件局部状态。

存储失败会显示提示，仍能在内存中完成登录或退出。401 或停用会清理当前会话；普通权限不足保留登录并显示拒绝原因；旧身份响应不能清理新登录。请求期限为 15 秒，包含正文读取。离开页面或切换身份会取消等待并丢弃旧响应；取消写入不代表回滚，重新进入相应页面时读取服务端确认。

个人资料统一使用 PUT，完整提交 `nickname` 和 `avatar_url`，空字符串清空。头像从文件选择器选取 JPG/PNG，最大 2MB；保存时先通过带认证的 multipart 上传，再将服务端返回的同源相对路径绑定到资料。重新读取或仅修改昵称时保留该路径。所有新密码按 UTF-8 的 10–72 字节校验。

## 验证

```powershell
npm test
npm run build
npm run lint
```

Vitest 自动发现 `src/` 和 `tests/` 下的 `*.test.{ts,tsx,mjs}`；`npm run test:watch` 可交互运行。局部测试靠近实现，跨业务客户端与应用路由流程放 `tests/integration`，迁移后的 TSX 流程测试仍纳入 TypeScript 检查；Node 测试使用 Node 工具配置检查。React Testing Library 实际挂载组件、Hook 和路由，验证输入、会话持久化与恢复、草稿、请求取消、登录恢复、权限及浏览器历史对应的路由行为。局部测试使用 HTTP 或平台替身隔离后端，不能代替产品贯通验证。

真实产品流程由 Playwright 连接 Go API、GORM、隔离 PostgreSQL、React 和 uniapp H5 执行：

```powershell
npx playwright install chromium
$env:SAAS_TEST_DATABASE_URL = 'postgres://用户:密码@localhost:测试端口/测试库?sslmode=disable'
npm run test:e2e
```

测试启动端口 8081、5175、5176，平台入口使用 `platform.localhost:5175`，企业入口使用 `localhost:5175`。按本次运行标识确认 API 实例，在随机 schema 导入 `db/saas.sql` 并初始化测试身份，结束后清理。不会使用开发环境的 DATABASE_URL。需要 Go、两端前端依赖及空闲测试端口。流程覆盖平台开通、小程序配置保存与重新读取、密钥留空保留、企业配置隔离、域名入口隔离、管理员建号、双端资料与改密、企业信息保存、停用恢复、会话撤销、路由直达与历史，以及同一标签页跨域名会话恢复、草稿清理和退出后刷新不恢复会话。Web 与 H5 的密码登录入口只认证已有账号。小程序配置流程不调用真实微信上游。失败截图与 trace 保存在被 Git 忽略的 `test-results/`。

生产将 `dist` 部署到 HTTPS 站点，业务路径回退到 `index.html`，`/api` 独立反向代理到 Go 服务且不走 SPA 回退；不要将后端秘密写入前端环境变量。
