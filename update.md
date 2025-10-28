# 游戏后端微服务架构重构文档

## 1. 项目概述

### 1.1 当前架构

- **acquire/** - Acquire 游戏独立服务
- **splendor/** - Splendor 游戏独立服务
- **docker-compose.yml** - 容器编排配置
- **nginx/** - 反向代理配置
- **kafka-data/** - 消息队列数据

### 1.2 目标架构

采用混合微服务架构：

- **房间服务**：纯 gRPC 服务，负责房间管理
- **游戏服务**：HTTP + WebSocket 对外，gRPC 客户端内部通信
- **API 网关**：Nginx 负载均衡和路由

## 2. 架构设计

### 2.1 服务拆分

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   API Gateway   │    │  Room Service   │    │ Acquire Service │
│    (Nginx)      │    │   (gRPC Only)   │    │ (HTTP+WS+gRPC)  │
│   Port: 80      │    │   Port: 9001    │    │   Port: 8082    │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         │                       │              ┌─────────────────┐
         │                       │              │ Splendor Service│
         │                       │              │ (HTTP+WS+gRPC)  │
         │                       │              │   Port: 8083    │
         │                       │              └─────────────────┘
         │                       │                       │
         │              ┌─────────────────┐              │
         └─────────────►│     Redis       │◄─────────────┘
                        │   Port: 6379    │
                        └─────────────────┘
```

### 2.2 通信模式

| 通信类型            | 协议      | 用途         | 示例               |
| ------------------- | --------- | ------------ | ------------------ |
| 客户端 ↔ 游戏服务   | WebSocket | 实时游戏交互 | 下棋、购买股票     |
| 客户端 ↔ 游戏服务   | HTTP      | RESTful API  | 创建房间、获取状态 |
| 游戏服务 ↔ 房间服务 | gRPC      | 内部服务调用 | 房间验证、状态更新 |
| 外部 ↔ 系统         | HTTP      | API 网关     | 统一入口           |

## 3. 服务职责划分

### 3.1 房间服务 (Room Service)

**职责**：

- 房间生命周期管理（创建、销毁、状态变更）
- 房间信息存储和查询
- 房间列表维护
- 玩家房间关系管理

**接口设计**：

- `CreateRoom(gameType, aiCount) -> roomID`
- `GetRoom(roomID) -> roomInfo`
- `UpdateRoomStatus(roomID, status) -> success`
- `ListRooms(gameType) -> roomList`
- `JoinRoom(roomID, playerID) -> success`
- `LeaveRoom(roomID, playerID) -> success`

**数据存储**：

- Redis 存储房间基础信息
- 房间状态：waiting, playing, finished
- 玩家列表和 AI 配置

### 3.2 Acquire 游戏服务

**职责**：

- Acquire 游戏逻辑处理
- WebSocket 连接管理
- 游戏状态维护
- AI 玩家管理

**保留模块**：

- `ws/` - WebSocket 处理和游戏逻辑
- `controller/` - HTTP API 控制器
- `service/` - 业务逻辑服务
- `dto/` - 数据传输对象

**新增模块**：

- `grpc/client/` - 房间服务 gRPC 客户端
- `config/` - 服务配置管理

### 3.3 Splendor 游戏服务

**职责**：

- Splendor 游戏逻辑处理
- WebSocket 连接管理
- 游戏状态维护
- 卡牌和宝石管理

**保留模块**：

- `ws/` - WebSocket 处理和游戏逻辑
- `controller/` - HTTP API 控制器
- `const_data/` - 游戏常量数据
- `entities/` - 游戏实体定义

**新增模块**：

- `grpc/client/` - 房间服务 gRPC 客户端
- `config/` - 服务配置管理

## 4. 目录结构重构

### 4.1 新的项目结构

```
game-backend/
├── docker-compose.yml           # 容器编排
├── nginx/                       # API网关配置
│   └── nginx.conf
├── proto/                       # gRPC协议定义
│   ├── room.proto
│   └── generated/               # 生成的gRPC代码
├── room-service/                # 房间管理服务
│   ├── Dockerfile
│   ├── go.mod
│   ├── main.go
│   ├── server/                  # gRPC服务实现
│   ├── repository/              # 数据访问层
│   └── config/                  # 配置管理
├── acquire-service/             # Acquire游戏服务
│   ├── Dockerfile
│   ├── go.mod
│   ├── main.go
│   ├── controller/              # HTTP控制器
│   ├── ws/                      # WebSocket处理
│   ├── service/                 # 业务逻辑
│   ├── grpc/client/             # gRPC客户端
│   └── config/                  # 配置管理
└── splendor-service/            # Splendor游戏服务
    ├── Dockerfile
    ├── go.mod
    ├── main.go
    ├── controller/              # HTTP控制器
    ├── ws/                      # WebSocket处理
    ├── const_data/              # 游戏数据
    ├── grpc/client/             # gRPC客户端
    └── config/                  # 配置管理
```

### 4.2 代码迁移映射

| 原路径                        | 新路径                        | 说明                   |
| ----------------------------- | ----------------------------- | ---------------------- |
| `acquire/service/room.go`     | `room-service/server/`        | 房间逻辑迁移到独立服务 |
| `acquire/repository/redis.go` | `room-service/repository/`    | Redis 连接迁移         |
| `acquire/ws/`                 | `acquire-service/ws/`         | WebSocket 逻辑保留     |
| `acquire/controller/`         | `acquire-service/controller/` | HTTP 控制器保留        |
| `splendor/service/room.go`    | `room-service/server/`        | 房间逻辑合并           |
| `splendor/ws/`                | `splendor-service/ws/`        | WebSocket 逻辑保留     |

## 5. 实施计划

### 5.1 阶段一：基础设施准备（1-2 天）

**任务**：

1. 创建 gRPC 协议定义文件
2. 生成 gRPC 客户端和服务端代码
3. 搭建房间服务基础框架
4. 配置 Docker Compose 新的服务

**交付物**：

- `proto/room.proto` 协议定义
- `room-service/` 基础框架
- 更新的 `docker-compose.yml`

### 5.2 阶段二：房间服务实现（2-3 天）

**任务**：

1. 实现房间服务 gRPC 接口
2. 迁移 Redis 房间数据逻辑
3. 实现房间生命周期管理
4. 添加服务健康检查

**交付物**：

- 完整的房间服务实现
- 房间管理 API 测试

### 5.3 阶段三：游戏服务改造（3-4 天）

**任务**：

1. 为游戏服务添加 gRPC 客户端
2. 重构房间相关逻辑调用
3. 保持 WebSocket 接口不变
4. 更新 HTTP API 路由

**交付物**：

- 改造后的 acquire-service
- 改造后的 splendor-service
- 保持原有客户端兼容性

### 5.4 阶段四：集成测试（1-2 天）

**任务**：

1. 端到端功能测试
2. 性能基准测试
3. 错误处理验证
4. 文档更新

**交付物**：

- 完整的微服务系统
- 测试报告
- 部署文档

## 6. 技术要点

### 6.1 gRPC 配置

**服务端配置**：

- 使用 grpc-go 库
- 支持健康检查
- 配置连接池和超时
- 添加拦截器用于日志和监控

**客户端配置**：

- 连接复用和负载均衡
- 重试机制和熔断器
- 超时控制
- 错误处理

### 6.2 WebSocket 处理

**连接管理**：

- 保持现有 WebSocket 接口
- 连接池管理
- 心跳检测
- 优雅断开

**消息路由**：

- 游戏消息直接处理
- 房间操作通过 gRPC 调用
- 状态同步机制

### 6.3 数据一致性

**Redis 使用**：

- 房间服务独占房间数据
- 游戏服务处理游戏状态
- 使用 Redis 事务保证一致性

**状态同步**：

- 房间状态变更通知
- 游戏状态定期同步
- 异常恢复机制

## 7. 监控和运维

### 7.1 服务监控

**指标收集**：

- gRPC 调用延迟和成功率
- WebSocket 连接数和消息量
- Redis 连接池状态
- 服务资源使用情况

**日志管理**：

- 结构化日志输出
- 分布式链路追踪
- 错误日志聚合

### 7.2 部署策略

**滚动更新**：

- 房间服务优先更新
- 游戏服务逐个更新
- 保持服务可用性

**回滚机制**：

- 版本标记和快速回滚
- 数据库迁移回滚
- 配置回滚

## 8. 风险评估

### 8.1 技术风险

| 风险               | 影响 | 缓解措施           |
| ------------------ | ---- | ------------------ |
| gRPC 学习曲线      | 中   | 提前学习，小步迭代 |
| 服务间通信延迟     | 低   | 本地网络，性能测试 |
| 数据一致性问题     | 高   | 事务机制，补偿逻辑 |
| WebSocket 连接迁移 | 中   | 保持接口兼容       |

### 8.2 业务风险

| 风险       | 影响 | 缓解措施           |
| ---------- | ---- | ------------------ |
| 服务不可用 | 高   | 健康检查，自动重启 |
| 性能下降   | 中   | 性能测试，优化调整 |
| 功能回归   | 中   | 完整测试，灰度发布 |

## 9. 成功标准

### 9.1 功能标准

- [ ] 所有现有游戏功能正常工作
- [ ] WebSocket 连接稳定
- [ ] 房间管理功能完整
- [ ] AI 玩家正常运行

### 9.2 性能标准

- [ ] 响应时间不超过现有系统的 120%
- [ ] 并发连接数不低于现有系统
- [ ] 内存使用合理
- [ ] CPU 使用率稳定

### 9.3 可维护性标准

- [ ] 代码结构清晰
- [ ] 服务边界明确
- [ ] 文档完整
- [ ] 监控覆盖全面

这个架构既保持了现有功能的完整性，又为未来扩展奠定了良好基础。通过分阶段实施，可以降低风险并确保平稳过渡。
