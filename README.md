# AdFlow · 广告决策与投放验证平台

基于 **Go、MySQL、Redis、Kafka** 的后端实践项目，配有可操作的 React 管理台。从配置广告计划，到用户定向、内部竞价、预算预占、曝光结算，再到事件计量和异常排查，走通一条完整的广告投放链路。

重点是业务约束、并发正确性和可复查的压测记录，适合作为 Go 后端学习与面试展示项目。

[快速启动](#快速启动) · [功能说明](docs/getting-started.md) · [API 文档](docs/openapi.yaml) · [性能与验证](#性能与验证)

![投放总览](docs/images/overview.png)

*截图来自本地运行的管理台，曝光、点击、转化价值均为模拟数据。*

## 可以做什么

| 功能 | 说明 |
| --- | --- |
| 广告计划与素材 | 草稿、发布、暂停；不可变规则版本与乐观并发控制 |
| 定向与竞价 | 标签/字段的 all、any、none 规则；多个广告主的一价竞价，保留成交快照 |
| 预算与频控 | 整数“分”计价，决策时预占，曝光后确认；重复事件不重复扣费 |
| 用户与投放模拟 | 保存模拟画像，按计划生成样本，单次验证或固定并发批量运行 |
| 异步事件处理 | 结算队列、Transactional Outbox、Kafka 消费、幂等计量、死信与待核对 |
| 请求追踪 | 按 requestId 查看决策、曝光结算、发布和统计状态 |
| 权限与辅助配置 | JWT、RBAC、审计日志；Agent 生成草稿，经人工确认后发布 |

内置固定成本和三广告主竞价用例。新环境自动安装一次；用户可以编辑或删除，持久化环境重启不会恢复已删除数据。后台账号与模拟用户画像是两种不同对象。

<details>
<summary>查看竞价配置、决策结果和完整处理过程</summary>

### 竞价与定向配置

![竞价规则](docs/images/auction-rules.png)

### 单次投放验证

![单次决策结果](docs/images/decision.png)

### 决策 → 曝光结算 → 事件计量

![请求处理过程](docs/images/request-trace.png)

</details>

## 技术设计

```mermaid
flowchart LR
    UI[React 管理台 / k6] --> API[Go · Gin API]
    API --> Decision[幂等决策 · 定向 · 竞价]
    Decision --> Redis[Redis 画像缓存 / 预算与频控预占]
    Decision --> DB[(MySQL 决策与规则版本)]
    API --> Ingest[曝光 / 点击 / 转化回传]
    Ingest --> Settlement[持久化曝光结算队列]
    Settlement --> Redis
    Settlement --> Outbox[Transactional Outbox]
    Outbox --> Kafka[Kafka]
    Kafka --> Metrics[幂等消费与统计事务]
    Metrics --> DB
```

- **模块化单体**：按 campaign、decision、event、identity 等业务模块组织；领域与应用层依赖接口，MySQL、Redis、Kafka 位于适配层。
- **缓存与原子预占**：画像采用 Cache-Aside，含负缓存、并发回源合并和 generation 校验，避免旧查询重新填入过期值；Redis Lua 原子处理预算和频控。
- **并发与过载保护**：速率限制、Channel 限制执行并发、Context 超时；根据异步队列高低门槛暂停和恢复新决策，避免积压失控。
- **可靠事件链路**：先持久化受理，曝光结算确认后发布；Kafka 至少一次投递，消费端按 eventId 幂等，统计与处理记录同事务提交。
- **恢复与可解释性**：worker 租约和 owner 校验、明确回滚后的有限事务重试；保存成交版本与价格，异常进入待核对而非猜测扣费成功。

项目采用内部广告主竞价模型，不连接外部广告交易平台；广告主标识是业务归属，尚未实现广告主多租户隔离。MySQL 与 Redis 的跨存储操作不具备单事务原子性，通过预占、持久化状态和恢复流程处理失败边界。

## 性能与验证

以下是 **2026-09-08 本机实验记录**，不是生产容量承诺。测试版本、预热、负载、失败档位及原始记录保存在对应报告中；不同接口的数据不能互相替代。

| 测试对象 | 已验证结果 | 证据与边界 |
| --- | --- | --- |
| 画像查询，Redis 热缓存 | 1000 QPS，3 × 60s 通过 | [容量报告](docs/profile-capacity-20260908.md)；1500 QPS 复测未稳定通过 |
| 广告决策，含竞价与预占 | 450 QPS，3 × 60s 达标 | [决策报告](docs/decision-capacity-20260908.md)；81003 次请求有 2 次 409，不含事件回传 |
| Kafka 完整业务链路 | 45 轮/秒，3 × 120s；16202 轮、48606 条事件完成 | [完整链路](docs/kafka-capacity-followup-20260908.md)；约 180 HTTP QPS，50 轮第三组出现积压 |
| 异步积压保护 | 100 次新决策尝试/秒 × 120s，受理 6681 轮、明确拒绝 5320 次 | [保护验收](docs/async-backpressure-20260908.md)；已受理流程全部完成，统计 P95 2.875s，恢复阶段无拒绝；不代表全量接收 100 轮/秒 |
| SQL 索引对照 | 百万行 Outbox 场景，UPDATE 服务端 P95 164.52ms → 1.09ms | [独立实验](docs/sql-index-experiment-20260908.md)；同一索引 invisible/visible 对照，含回滚，非接口耗时 |

完整链路的一轮包含一次决策和曝光、点击、转化三次回传。压测同时核对受理数量、结算、消费、预算金额和重复事件；HTTP 202 或 Kafka lag=0 都不能单独代表业务已完成。

## 快速启动

需要 Go **1.27+** 和 Node.js **24**。默认使用内存适配器，可先体验功能，再接入真实基础设施。

```bash
git clone https://github.com/umi3730/AdFlow.git
cd AdFlow
go mod download
go run ./cmd/api
```

另开一个终端启动前端：

```bash
cd AdFlow/web
npm ci
npm run dev
```

打开终端显示的前端地址（默认 `http://localhost:3000`），API 默认 `http://localhost:18080`。环境变量可修改端口和适配器；程序本身不自动读取 `.env`，需要由 shell 或启动脚本加载。

首次体验建议：右上角 **功能说明 → 准备竞价演示 → 开始模拟**，先跑三轮，观察成交价和事件统计，再查看请求处理过程。各页面的用途见[第一次上手](docs/getting-started.md)。

本地认证默认关闭。开启 `ADFLOW_AUTH_ENABLED=true` 后，登录页预填 local/test 环境的演示账号 `admin / adflow-admin`，也可点击“注册账号”。本 Demo 新注册账号统一具有管理员权限，MySQL 模式下账号持久保存。详见[注册与登录说明](docs/demo-registration-20260908.md)；其他环境应自行配置账号哈希、JWT 密钥和注册开关。

### MySQL / Redis / Kafka 模式

使用 MySQL **8.0**，先应用全部迁移（当前至 `000013`）：

```bash
go run ./cmd/migrate -dir migrations
```

通过环境配置选择 MySQL 存储、Redis 预占及 Kafka 传输，并准备对应数据库和 Topic。详见 [运行配置](docs/runtime-configuration.md)、[配置示例](.env.example) 和 [原生组件脚本](ops/native/)。Agent 默认为离线 Mock，可选配置兼容模型服务；API 密钥只放运行环境。

## 测试与目录

```bash
go test ./...
go vet ./...
cd web
npm test
npm run lint
npm run build
```

GitHub Actions 运行 Go 格式检查、vet、race 测试，以及前端格式、lint、测试和构建。真实 MySQL / Redis / Kafka 集成测试需要独立测试环境并显式启用，见[基础设施验证](docs/integration-verification.md)。

```text
cmd/            API 与数据库迁移入口
internal/       业务模块、基础设施适配器、可观测性
migrations/     可重复执行并校验摘要的数据库迁移
web/            React / TypeScript 管理台
tests/          集成测试、k6 / JMeter 与独立基准实验
docs/           功能设计、故障复盘、测试报告及原始证据
ops/            本地基础设施与 API 启动脚本
```

后续重点：减少结算和同步审计的数据库写入开销，补充更长依赖中断与多实例过载测试。已有边界和计划保存在 [Roadmap](docs/roadmap.md)。
