<p align="center">
  <img src="web/public/logo.png" width="112" alt="GrayCrow">
</p>

<h1 align="center">GrayCrow · 灰鸦</h1>

<p align="center">GitHub 公开代码敏感信息巡检与告警处置</p>

<p align="center">
  中文 · <a href="README.en.md">English</a>
</p>

GrayCrow 通过 GitHub Code Search 定期检索公开代码。当指定域名、邮箱或项目名与密码、密钥、Token 等凭据出现在同一处时，程序先在本地用正则与熵规则复核，去重后把同一批命中合并推送到规则绑定的通知渠道（飞书、企业微信、钉钉、通用 Webhook 或 SMTP）。接收人登录控制台后，可以标记误报、已处理或忽略，也可以将仓库加入白名单。

发布形态为单个二进制，前端构建产物通过 `go:embed` 内嵌其中。数据库默认 SQLite，同时支持 MySQL 与 PostgreSQL。

## 检索边界

- 仅覆盖 GitHub 已索引的公开仓库默认分支。索引延迟通常为数分钟至数小时。
- 单次查询最多返回 1000 条结果。关键词过宽时需要按日期切分查询窗口，仍可能遗漏。
- Code Search 为独立限流，认证后每分钟 10 次；其他 REST 接口的每小时 5000 次不适用于该接口。
- 只保存命中的代码片段，不保存文件全文，也不克隆仓库。
- 同一 GitHub 账号下的多个 Token 共享配额。扩充配额需要使用不同账号，多 Token 的作用是容错与分摊。

## 快速开始

环境要求：Go 1.26+、Bun 1.x；构建前端还需要 Node.js 20+，`vue-tsc` 无法在 Bun 运行时中正常工作。

```bash
cp .env.example .env             # 填写 MASTER_KEY，长度不少于 32 字符
cd web && bun install && cd ..   # 首次
make build-web                   # 首次，go:embed 要求 web/dist 存在
make dev                         # API :8080，Web :5173
```

首次启动自动建表，并创建管理员 `admin / admin123`，登录后请立即修改密码。只做前端开发可执行 `make dev-mock`，所有接口走本地 mock。

| 命令 | 说明 |
| --- | --- |
| `make dev` / `make dev-mock` | 前后端联调 / 纯 mock 前端 |
| `make dev-api` / `make dev-web` | 仅启动后端 / 仅启动前端 |
| `make test` | `go test ./...` |
| `make lint` | `go vet`、中英文案一致性检查、前端构建 |
| `make build` | 构建前端并编译 `bin/graycrow` |
| `make smoke` | SQLite 冒烟测试 |
| `make clean` | 清理 `bin/` 与 `web/dist/` |

## 部署

```bash
make build
./bin/graycrow
```

`make build` 会将前端产物一并编译进 `bin/graycrow`，可直接复制到服务器运行。切换数据库只需修改 `DB_DRIVER` 与 `DB_DSN`；SQLite 不需要外部服务。

使用反向代理并启用 HTTPS 时，设置 `COOKIE_SECURE=true`。前端也可以由外部静态服务器托管，此时通过 `STATIC_DIR` 指定构建目录。

Docker 部署：

```bash
cd deploy
MASTER_KEY=$(openssl rand -base64 32) docker compose --profile sqlite up -d
```

`deploy/docker-compose.yml` 提供 `sqlite`、`mysql`、`postgres` 三个 profile，每次启用其中一个。

## 本地演示

`cmd/fakegithub` 模拟 GitHub Code Search，返回固定的搜索结果与限流响应头，用于在没有真实 Token 的情况下走通建规则、扫描、命中、通知与处置的完整链路：

```bash
make fake-gh                                             # 终端 A
GITHUB_API_BASE=http://127.0.0.1:18081 make dev           # 终端 B
```

三库冒烟：

```bash
make smoke                                # SQLite
scripts/smoke.sh mysql '<dsn>'
scripts/smoke.sh postgres '<dsn>'
```

## 配置

配置优先级从高到低为：环境变量 > `.env` > `config.yaml` > 内置默认值。

可从 `.env.example` 或 `config.example.yaml` 复制模板。`config.yaml` 中允许写入 `master_key`，因此已被 `.gitignore` 排除。配置项名称与环境变量一致，平铺书写，DSN 无需引号转义；拼写错误会在启动时告警，修改后需重启生效。

## 通知

一次扫描产生的新命中会合并为一条消息，内容包括本次数量、仓库分布以及最多 20 条明细，其余条目仅给出计数。同一泄露重复命中只累加出现次数，不重复通知。每次发送都会生成一条通知记录，包含渠道、条数、状态、重试次数与正文；遗漏的通知可在单条告警详情页通过「重新投递」补发。

## 运维

密钥轮换：新值写入 `MASTER_KEY`，旧值写入 `MASTER_KEY_OLD` 后启动一次，日志出现「已用当前 MASTER_KEY 重加密 N 个字段」即可移除旧值并重启。密文自带密钥指纹，遗漏某次启动不会导致数据不可读，最坏情况是提示「密文对应的密钥不存在」。

两步验证：右上角头像 → 账户安全，可使用认证器应用，开启后生成 10 个一次性恢复码。管理员可在用户管理页面为丢失设备的用户重置。

干跑诊断：规则列表提供「干跑诊断」，按近 30 天的命中与误报情况给出关键词调整建议，可一键采纳。

数据保留：扫描日志保留 30 天，审计日志保留 180 天；告警与通知记录不自动清理。`data/` 目录需要纳入备份。

界面语言：右上角可切换中英文，选择保存在浏览器本地。

## 目录

```
cmd/graycrow/         入口，HTTP 服务
cmd/fakegithub/       本地假 GitHub，演示用
internal/config/      环境变量 / 配置文件加载与校验
internal/server/      Gin 引擎、路由注册、静态资源托管
internal/api/         HTTP handler
internal/middleware/  session、RBAC、CSRF 校验、访问日志脱敏、崩溃恢复、requestid
internal/model/       GORM 模型
internal/store/       建表与迁移
internal/service/     业务编排：规则、告警、渠道、密钥池、审计、保留策略
internal/engine/      调度循环：取到期规则 → 扫描 → 落告警 → 汇总通知
internal/scanner/     查询生成、GitHub 客户端、二次判定
internal/schedule/    interval 与 cron 的下次运行时间计算
internal/notify/      飞书、企业微信、钉钉、通用 Webhook、SMTP
internal/crypto/      AES-GCM 字段加密与密钥轮换
internal/httpx/       统一响应与错误码
web/                  Bun + Vite + Vue3 + Naive UI
docs/                 技术文档
deploy/               Dockerfile、docker-compose
scripts/              三库冒烟脚本
```

## 文档

| 文档 | 内容 |
| --- | --- |
| [01 架构](docs/01-architecture.md) | 模块分层、扫描链路、技术选型、环境变量、部署形态 |
| [02 数据模型](docs/02-data-model.md) | 表结构、索引、三库兼容、迁移策略 |
| [03 API](docs/03-api.md) | 响应约定、错误码、完整路由表 |
| [04 扫描器](docs/04-scanner.md) | 查询生成、密钥池调度、二次判定、去重、告警状态机 |
| [05 前端](docs/05-frontend.md) | 路由与页面、设计 token、状态管理、请求与 mock |
| [06 开发计划](docs/06-roadmap.md) | 阶段划分与验收标准 |

## 声明

仅用于检查有权检查的资产。使用时应遵守 GitHub 服务条款及当地法律法规。
