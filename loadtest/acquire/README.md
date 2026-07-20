# Acquire Load Test

这套工具用于 Acquire 单机压测，默认优先本机验证，再低峰小流量打线上单机。

## 你需要准备什么

线上压测前，你需要准备：

1. 一个低峰时间窗口，建议优先选 `12:00-13:00`, `05:00-06:00`, `13:00-14:00`, `08:00-09:00`, `00:00-01:00`。
2. 一个临时压测 token，例如 `export ACQUIRE_LOADTEST_TOKEN='replace-with-random-secret'`。
3. 确认可随时停止压测进程。
4. 压测时盯住服务器公网出带宽；`3Mbps` 规格下，p95 到 `2.1Mbps` 停止加档，连续 1 分钟到 `2.4Mbps` 立即停止。
5. 如需 pprof，线上只通过 SSH tunnel 访问 `ACQUIRE_DEBUG_ADDR`，不要暴露公网。

## 启动被测服务

本机 `docker-compose.yml` 已给 acquire 配好本地默认值：

```yaml
ACQUIRE_LOADTEST_ENABLED=true
ACQUIRE_LOADTEST_TOKEN=local-test-token
ACQUIRE_DEBUG_ADDR=
```

所以本机 Docker 场景可以直接用 `local-test-token`。如果不用 Docker、直接 `go run`，再显式导出环境变量：

```bash
cd game-backend
export POSTGRES_DSN='your-postgres-dsn'
export ACQUIRE_LOADTEST_ENABLED=true
export ACQUIRE_LOADTEST_TOKEN='local-test-token'
export ACQUIRE_DEBUG_ADDR='127.0.0.1:6060'
go run ./cmd/acquire
```

生产 `docker-compose.prod.yml` 默认保持关闭：

```yaml
ACQUIRE_LOADTEST_ENABLED=false
ACQUIRE_LOADTEST_TOKEN=
ACQUIRE_DEBUG_ADDR=
```

线上压测时必须临时显式设置强随机 token，跑完再关闭。

## 创建压测房间

```bash
curl -sS -X POST 'http://127.0.0.1:8000/__loadtest/acquire/rooms' \
  -H "X-Loadtest-Token: ${ACQUIRE_LOADTEST_TOKEN:-local-test-token}" \
  -H 'Content-Type: application/json' \
  -d '{"prefix":"lt-local-001","count":10,"ownerPrefix":"lt-owner"}'
```

清理压测房间：

```bash
curl -sS -X DELETE 'http://127.0.0.1:8000/__loadtest/acquire/rooms' \
  -H "X-Loadtest-Token: ${ACQUIRE_LOADTEST_TOKEN:-local-test-token}" \
  -H 'Content-Type: application/json' \
  -d '{"prefix":"lt-local-001"}'
```

查看房间和 runtime 粗略统计：

```bash
curl -sS 'http://127.0.0.1:8000/__loadtest/acquire/stats' \
  -H "X-Loadtest-Token: ${ACQUIRE_LOADTEST_TOKEN:-local-test-token}"
```

## Go Runner

Go runner 会自己通过 guarded helper 创建房间。

只压 WebSocket 长连接：

```bash
go run ./loadtest/acquire/runner \
  -http-base http://127.0.0.1:8000 \
  -ws-base ws://127.0.0.1:8000 \
  -token "${ACQUIRE_LOADTEST_TOKEN:-local-test-token}" \
  -prefix lt-connect-001 \
  -mode connect \
  -rooms 5 \
  -duration 5m
```

压 `1 真人 + 5 AI`：

```bash
go run ./loadtest/acquire/runner \
  -http-base http://127.0.0.1:8000 \
  -ws-base ws://127.0.0.1:8000 \
  -token "${ACQUIRE_LOADTEST_TOKEN:-local-test-token}" \
  -prefix lt-ai-001 \
  -mode ai \
  -rooms 2 \
  -duration 5m
```

压 `6 真人 WebSocket`：

```bash
go run ./loadtest/acquire/runner \
  -http-base http://127.0.0.1:8000 \
  -ws-base ws://127.0.0.1:8000 \
  -token "${ACQUIRE_LOADTEST_TOKEN:-local-test-token}" \
  -prefix lt-ws6-001 \
  -mode ws6 \
  -rooms 2 \
  -duration 5m
```

线上单机示例，先从 2 房开始：

```bash
go run ./loadtest/acquire/runner \
  -http-base https://api.gamebus.online/api/acquire \
  -ws-base wss://api.gamebus.online/api/acquire \
  -token "${ACQUIRE_LOADTEST_TOKEN}" \
  -prefix lt-online-001 \
  -mode ws6 \
  -rooms 2 \
  -duration 5m
```

## k6 连接压测

先用 helper 创建房间，再把 roomID 用逗号传给 k6：

```bash
ROOMS='lt-local-001-000001,lt-local-001-000002' \
ACQUIRE_WS_BASE='ws://127.0.0.1:8000' \
VUS=30 \
DURATION=5m \
k6 run ./loadtest/acquire/k6/ws_connect.js
```

## k6 HTTP 读压测

```bash
ACQUIRE_HTTP_BASE='http://127.0.0.1:8000' \
ROOM_ID='lt-local-001-000001' \
VUS=10 \
DURATION=5m \
k6 run ./loadtest/acquire/k6/http_read.js
```

`/room/list` 和 `/room/game_status` 走鉴权。如果本机没有有效 cookie，会返回 401；这可以验证鉴权链路，但不代表 Acquire 游戏服读接口本身的性能。

## 线上停止线

线上服务器规格为 `2 核 / 2GB / 3Mbps`，压测时按以下规则停止：

- 公网出带宽 p95 >= `2.1Mbps`：当前档作为上限，不再加档。
- 公网出带宽连续 1 分钟 >= `2.4Mbps`：立即停止。
- 内存利用率 > `70%`：当前档结束后不再加档。
- 内存利用率 > `75%`：立即停止。
- CPU 连续 1 分钟 > `90%`：立即停止。
- WebSocket 异常断连率 > `0.5%`：立即停止。
