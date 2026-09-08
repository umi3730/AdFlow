# 异步积压过载保护

## 结果

已加入 Kafka 模式的新决策积压保护，并更新本机 18080 / 18081 API，无数据库迁移。正常负载仍执行原决策逻辑；后台结算或发布积压时，对尚未生成的决策返回 **503 `decision_backpressure`**，附 `Retry-After: 1`，待队列降低后自动恢复。

本次真实 k6 / MySQL / Redis / Kafka 测试：

| 阶段 | 尝试速率 / 时长 | 尝试数 | 完整业务数 | 预期 503 拒绝 | 统计完成 P95 |
| --- | --- | ---: | ---: | ---: | ---: |
| 正常负载 | 45 次/秒，20s | 901 | 901 | 0 | 1.454s |
| 过载 | 100 次/秒，120s | 12001 | 6681 | 5320 | 2.875s |
| 恢复 | 45 次/秒，30s | 1351 | 1351 | 0 | 1.270s |

**8933 个正式完整流程、26799 条正式事件**全部核账通过；无非预期业务错误、未派发、No-Ad 或新增 RECONCILE。过载期间已命中决策的 HTTP P95 为 87.74ms，四次请求受理 P95 为 256ms。正式决策尚未结算的采样峰值为 118，已受理尚未发布事件峰值为 433；过载结束后约 1.29s 排空。

每阶段另有 3 次完整预热，单独计入预算、转化金额和统计核验。重复回传未重复扣费/计数，Redis 预占最终为 0，Kafka 三分区最终 lag=0。拒绝的新决策不持久化，不产生预占和待处理事件。

**100 次/秒是新决策尝试速率，绝不是接收 100 个完整流程/秒。** 5320 次 HTTP 503 明确计为拒绝，没有隐藏在成功数中。此次是过载保护验收，不是最大 QPS 或相同条件下的性能提升对照；不能把它写成“吞吐提升到 100 QPS 且零 HTTP 错误”。

## 设计

上一轮长测暴露了持续积压超过 Redis 30 秒预占有效期的风险，见 [容量复测](kafka-capacity-followup-20260908.md)。现有速率限制和 256 个执行槽只限制正在处理的决策，无法反映已受理异步任务的压力，因此增加反馈保护。

每个 API 进程用一个 goroutine，每 200ms 查询共享数据库的活动队列；查询超时 200ms。一次 SQL 同时读取：

- `event_settlements` 中 PENDING / PROCESSING 的条数，最多读取到高门槛。
- `event_outbox` 中 SETTLING / PENDING / PROCESSING 的条数，最多读取到高门槛。

使用现有以 status 开头的索引和带 LIMIT 的派生表，不扫描全部已发布历史，不把 RECONCILE / DEAD_LETTERED 算成活动队列。计数是**门槛处截断的值**，不是精确的全量看板统计，因此无需新索引迁移。

| 配置 | 默认值 |
| --- | ---: |
| `ADFLOW_ASYNC_BACKPRESSURE` | true，仅 Kafka 模式接入 |
| `ADFLOW_SETTLEMENT_BACKLOG_HIGH` | 100 |
| `ADFLOW_SETTLEMENT_BACKLOG_LOW` | 40 |
| `ADFLOW_OUTBOX_BACKLOG_HIGH` | 600 |
| `ADFLOW_OUTBOX_BACKLOG_LOW` | 240 |

任一队列达到高门槛时暂停新决策；两者均低于或等于低门槛后恢复，中间区间保持原状态，避免开关频繁抖动。配置要求 `0 <= low < high`。

请求端只读取 `atomic.Pointer` 保存的不可变快照，不额外查询队列。未完成首次采样、采样失败、快照超过 1 秒未更新时，检查器返回 `decision_backpressure_unavailable`（503）；监控停止后也关闭放行。已有决策的 SQL 查询仍依赖数据库，数据库本身不可用时可能先返回原有查询错误。

检查放在已存在决策的幂等查询之后、执行租约和频控/预算预占之前：

- 已提交决策可按原参数重试获取，不被本次积压门槛拦截；原有速率/并发限制仍适用。
- 相同 requestId 但参数冲突继续返回 409，不把冲突伪装成过载。
- 曝光、点击、转化入口继续服务，后台 worker/relay 继续排空。
- `/v1/simulations/decisions` 复用相同决策服务，也受到保护。
- 无永久 No-Ad 或失败决策落库，调用方可稍后使用相同参数重试。

## 监控和边界

新增 Prometheus 指标：

- `adflow_decision_async_backlog{queue}`：截断后的队列采样数，只有观测可用时才有意义。
- `adflow_decision_backpressure_state{state}`：最近观测状态 open / blocked / unavailable，one-hot；请求另有 1 秒陈旧检查。
- `adflow_decision_backpressure_total{result}`：新决策检查的 allowed / blocked / unavailable 次数，已完成决策重放不进入此检查。

这是采样反馈控制，**不是严格的全局在途数量上限**。多个 API 的采样延迟、检查后已经在途的请求、晚到曝光均可能使真实队列短暂超过门槛；此次未结算采样峰值 118 就高于 100。消费者已收到 Kafka 消息后的处理积压不在这两个队列计数中，后续可单独扩展消费端保护。

依赖长时间中断、突发流量超过保护余量、曝光接近预占到期才回传等场景仍可能需要 RECONCILE。未延长预占期限，未提高结算并行度或降低数据库持久化等级，也未优化同步审计写入；这些需要独立实验。

## 验证与交付

- 单元回归覆盖高低门槛、防抖状态、启动/失联/陈旧快照、取消、监控退出关闭放行、拒绝前无租约/决策写入、已提交重放和参数冲突。
- HTTP 回归确认两个积压错误为 503 且有 Retry-After；配置回归验证默认、覆盖和非法门槛。
- 真实 MySQL 测试重复 3 次通过：活动状态计数、LIMIT 截断、100 条历史 RECONCILE 不触发积压、处理完成后计数清零。
- `go test ./... -count=1` 与 `go vet ./...` 通过。没有声称本机运行过 `-race`。
- 最初集成测试漏建 Outbox 对应的 receipt，修正测试夹具后重测通过；没有关闭外键。另将上轮归档的 `.go` 源码快照改为 `.go.txt`，避免被全量测试当作代码编译，内容摘要保持不变。
- 本机 API 已更新，两个实例的队列采样均为 0、保护状态 open，前端 3001 返回 200。原有 42 条历史 RECONCILE、1426 条 PUBLISHED 保留。
- 压测和集成测试使用独立数据库、Redis / Kafka 资源，完成后清理，无新迁移。

复跑过载验收：

```powershell
.\tests\load\run-k6-walkthrough.ps1 -Backpressure -ConfirmSeconds 120
```

该模式使用独立的 `kafka-backpressure.js`，仅将指定错误码且包含 Retry-After 的 503 作为预期拒绝；其他错误仍导致失败。原容量模式仍把拒绝视为容量未达标，阈值没有放宽。报告核验同时核对 `admitted + rejected == attempts`、已受理业务延迟、最终账目和恢复阶段无拒绝。

证据：[原始压测](verification/async-backpressure/proof.json.gz)、[独立核验](verification/async-backpressure/verification.json)、[队列峰值](verification/async-backpressure/queue-peaks.json)、[真实数据库测试](verification/async-backpressure/integration.log)、[全量回归](verification/async-backpressure/go-test.log)、[本机部署检查](verification/async-backpressure/local-deployment.json)。源码快照及 SHA256 随证据保存，不包含数据库密码或登录 token。
