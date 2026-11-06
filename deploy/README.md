# Docker 部署

部署包含 `server` 和 `web` 两个应用容器，uniapp 不参与。PostgreSQL 和 S3 兼容对象存储通过配置连接，Compose 不创建、重建或清理这些外部服务。Go 使用多阶段构建，React 由 Node 构建后交给 Nginx 提供静态文件，运行时没有 Node 或 Vite 开发服务器。

```mermaid
flowchart LR
    U["浏览器：平台或企业域名"] --> G["已有 HTTPS 网关"]
    G --> W["web：Nginx + React 静态文件"]
    W -->|"/api 原路径、原 Host"| S["server：Go API"]
    S --> P[("配置的 PostgreSQL")]
    S --> O["配置的 S3 兼容对象存储"]
```

同一份 Web 镜像服务所有企业域名，前端默认请求同源 `/api`，无需为每家企业重新构建。企业身份仍由后端根据原始 Host 查询；客户端不能通过请求头指定企业。

## 准备配置

需要 Docker Engine 及 Docker Compose v2+，使用 Linux 容器。以下命令都在仓库 `deploy` 目录执行：

```powershell
Copy-Item .env.example .env
```

编辑 `.env`，填写实际数据库连接、平台域名、S3 地址、桶及专用 AK/SK。该文件已被忽略，不进入镜像或版本控制；Compose 只把服务端配置传给 server，Web 不接收数据库或 S3 凭据。默认镜像构建上下文分别限定在 server 和 react-app，各自 `.dockerignore` 排除本地配置和构建产物。

| 参数 | 说明 |
| --- | --- |
| `DATABASE_URL` | 已存在的 PostgreSQL 16+ 数据库连接；密码中特殊字符须 URL 编码 |
| `PLATFORM_HOSTS` | 平台管理域名，逗号分隔，不包含协议、端口或路径 |
| `S3_ENDPOINT` | S3 兼容服务地址；AWS S3 可留空并配置对应 region |
| `S3_REGION`、`S3_BUCKET` | 区域和预先创建的私有桶 |
| `S3_ACCESS_KEY_ID`、`S3_SECRET_ACCESS_KEY` | 限定桶权限的服务端凭据 |
| `S3_USE_PATH_STYLE` | MinIO 通常为 true，按服务商要求配置 |
| `WEB_BIND_ADDRESS`、`WEB_PORT` | 宿主机发布地址与端口，默认只监听 `127.0.0.1:8088` |
| `IMAGE_TAG` | 本地构建镜像标签；发布时建议使用版本号或提交号 |

**容器里的 localhost 是容器自身。** 连接同机已发布端口的数据库和 MinIO 时，Docker Desktop 可使用示例中的 `host.docker.internal`；Compose 也为 Linux 配置了 `host-gateway` 映射，目标服务还须允许该地址访问。连接远程服务时直接填写其可达 DNS/IP。已有容器若通过共享网络连接，使用容器 DNS 名与容器内端口；MinIO 内部端口是 9000，本机宿主机发布端口是 9002，两者不能混用。示例没有自动加入其他 Compose 项目的私有网络。

## 初始化和启动

1. 数据库必须已经存在。空数据库先按 [数据库说明](../db/README.md) 执行一次 `db/saas.sql`；已有 SaaS 数据库不要重复导入。应用启动只检查表是否就绪，不自动建表或覆盖数据。
2. S3 模式先在对象存储服务创建私有桶及专用凭据；应用不自动建桶。当前本机已有 `agent` 桶，可使用已有专用凭据配置新容器。
3. 构建镜像：

```powershell
docker compose config --quiet
docker compose build
```

新环境没有平台管理员时，临时传入初始化凭据，复用 server 镜像执行一次初始化。PowerShell 示例通过凭据提示读取密码，避免将实际密码写进命令或 Compose 配置：

```powershell
$adminCredential = Get-Credential -Message '设置首位平台管理员'
$env:BOOTSTRAP_USERNAME = $adminCredential.UserName
$env:BOOTSTRAP_PASSWORD = $adminCredential.GetNetworkCredential().Password
try {
    docker compose run --rm --no-deps -e BOOTSTRAP_USERNAME -e BOOTSTRAP_PASSWORD server bootstrap
} finally {
    Remove-Item Env:BOOTSTRAP_USERNAME, Env:BOOTSTRAP_PASSWORD
    Remove-Variable adminCredential
}
```

已有平台管理员时跳过初始化，避免把 bootstrap 当作重置密码入口。启动服务：

```powershell
docker compose up -d --wait
docker compose ps
docker compose logs --tail=100 server web
```

本地平台入口为 `http://platform.localhost:8088/platform`（示例 PLATFORM_HOSTS）。平台开通企业后，用对应企业域名访问同一端口。生产环境将平台域名和各企业域名指向 HTTPS 网关，网关再转发至 Web。server 的 8080 不直接发布到宿主机。

server 的健康检查调用 `/api/health`，验证进程和数据库，不代表 S3 已完成读写验收；Web 的 `/healthz` 检查静态服务器。Web 首次启动等待 server 健康，两个服务均以非 root 用户运行，支持停止信号和优雅关闭。健康状态异常本身不会使 Docker 自动重启进程；`restart` 策略处理的是进程退出。

## 本地子域名与 HTTPS 网关

本地可使用 `platform.localhost` 作为平台入口，使用 `alpha.localhost`、`beta.localhost` 等不同子域名作为企业入口，它们都访问 `127.0.0.1:8088`。Chrome、Edge 等浏览器将 `.localhost` 解析到本机；系统或客户端不支持时，可在 hosts 文件添加对应域名到 `127.0.0.1` 的映射。平台中登记的企业 hostname 只填域名，不含协议和端口。

| 本地地址 | 用途 |
| --- | --- |
| `http://platform.localhost:8088/platform` | 平台管理入口 |
| `http://alpha.localhost:8088/` | 企业 alpha 的账号、成员和设置 |
| `http://beta.localhost:8088/` | 企业 beta，用于验证跨企业隔离 |

微信真实登录及原生资料能力留到线上环境验证，本地先验收 Web 的企业、账号、权限、资料和会话流程。

生产 HTTPS 证书和域名入口由现有网关维护，网关保留原始 Host。以宿主机 Nginx 为例，以下 location 放入已有 HTTPS server 块：

```nginx
location / {
    proxy_pass http://127.0.0.1:8088;
    proxy_set_header Host $http_host;
    client_max_body_size 3m;
    proxy_read_timeout 35s;
}
```

网关位于另一容器时，需提供可达的网络路径并相应调整发布地址；默认的宿主机回环绑定不会向其他主机开放。

Web 保留 `/api` 前缀及原始 Host，业务 API 不缓存，头像的 HEAD/Range 与后端缓存头正常传递。头像最大 2 MiB，代理放行 3 MiB 以容纳 multipart；普通 JSON 的 16 KiB 限制仍由后端执行。哈希静态资源长期缓存，index.html 每次重新验证，React Router 的页面刷新回退到 index.html；缺失 API 或 `/assets/` 资源不会回退成页面。

## 改用文件存储

默认部署使用 S3，不为头像挂本地卷。若选择 filesystem，叠加对应文件：

```powershell
docker compose -f compose.yaml -f compose.filesystem.yaml up -d --build --wait
```

该配置将存储设为 filesystem，并把 `/data/objects` 挂到命名卷 `objects`。后续操作需带同样两个 `-f`。切换配置不会迁移已有对象，已有头像需要先按原 key 迁入目标存储。不要删除仍在使用的对象卷或外部私有桶。

## 更新和停止

```powershell
docker compose up -d --build --wait
docker compose down
```

更新会重建应用镜像和容器，不重新导入 SQL，也不重置平台账号。停止只移除本项目应用容器和应用网络，不操作已有 PostgreSQL、MinIO。新增表结构时应另行明确升级方式，不能对已有库直接重复执行完整建表 SQL。
