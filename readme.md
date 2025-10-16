# Game Backend 桌游后端项目
这是一个基于 Go 语言开发的多人在线桌游后端系统，目前支持两款经典桌游： Splendor（璀璨宝石） 和 Acquire（收购） 。

## 🎮 支持的游戏
### Splendor（璀璨宝石）
- 经典的宝石收集策略游戏
- 支持多人实时对战
- WebSocket 实时通信
### Acquire（收购）
- 经典的股票投资策略游戏
- 支持多人实时对战
- WebSocket 实时通信
## 🏗️ 技术架构
### 后端技术栈
- 语言 : Go 1.23.0
- Web框架 : Gin
- 数据库 : Redis
- 实时通信 : WebSocket
- 容器化 : Docker + Docker Compose
- 反向代理 : Nginx
- CI/CD : GitHub Actions