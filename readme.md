# Game Backend 桌游后端项目
这是一个基于 Go 语言开发的多人在线桌游后端系统，目前支持两款经典桌游： Splendor（璀璨宝石） 和 Acquire（收购） 。

## 🎮 支持的游戏
### Splendor（璀璨宝石）
- 经典的宝石收集策略游戏
- 支持多人实时对战
- WebSocket 实时通信
### Acquire（并购）
- 经典的股票投资策略游戏
- 支持多人实时对战
- WebSocket 实时通信
## 🏗️ 技术架构
### 后端技术栈
- 语言 : Go 1.23.0
- Web框架 : Gin
- 数据库 : PostgreSQL
- 实时通信 : WebSocket
- 容器化 : Docker + Docker Compose
- 反向代理 : Nginx
- CI/CD : GitHub Actions

## 数据库配置

复制 `.env.example` 为本地 `.env` 后再按环境填写。生产服务统一连接阿里云
RDS 的 `gamebus_prod` 数据库，并强制启用 TLS：

```text
POSTGRES_DSN=postgresql://<user>:<password>@<instance>.pg.rds.aliyuncs.com:5432/gamebus_prod?sslmode=require
AUTH_CENTER_URL=http://pam-gateway:8080/pam-api/platform/auth
```

`.env` 不提交到 Git，生产部署前会拒绝旧 Neon、本地主机、关闭 TLS 或非
`gamebus_prod` 的连接串。
