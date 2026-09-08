# Outbox 结算恢复修复（2026-09-07）

后续状态更新：用户授权后已完成本地真实依赖验收、迁移和切换，见 [异步模式切换记录](async-switch-20260907.md)。本文下方“未执行/未重启”描述保留修复提交当时的验证范围。

## 修复的问题

旧 Kafka 入口先提交 Outbox，再分别确认 Redis 预算和频控。Redis 确认失败时，接口虽然报错，Outbox 仍可被发布、消费和计量；恢复依赖客户端再次提交。两个 Redis 确认之间也存在只成功一半的窗口。

本次增加持久化结算任务、原子结算凭证和消费校验。HTTP 202 表示已持久化受理，结算完成后才发布和计量。这是正确性修复，尚未测得新版吞吐或延迟提升。

## 处理流程

1. 入口校验决策、用户事件和有效期。同一 eventId 已受理时先比较身份，保留首次时间；即使原决策过期，合法重试也可查询到原受理结果。
2. 一个 MySQL 事务写入 event_settlements、event_receipts 和 event_outbox。任务保存中标决策快照，曝光初始状态为 SETTLING。同一 requestId 只接受一个曝光 eventId，冲突返回 409。点击、转化必须已有曝光结算任务。
3. 后台 worker 每次领取最多 10 条任务，使用数据库时间的 30 秒租约和每次新生成的 owner。空闲时每 200ms 轮询；有成功任务时继续取下一批，不额外等待一个轮询周期。
4. Redis Lua 在一次操作内验证两个预留及中标金额、确认预算和频控、写入幂等凭证。凭证绑定事件、请求、预留 token、用户、计划、素材和价格，保留 7 天。使用 Redis 预留键的剩余 TTL 判断存活，不混用应用与 Redis 的绝对时钟来判断过期。
5. 结算成功后，worker 在 MySQL 事务中将任务置为 SETTLED，同时把该请求的 SETTLING 事件转为 PENDING。回写必须匹配 owner、事件及有效租约，旧 worker 不能覆盖接管者。
6. 曝光发布成功后，Outbox 发布状态与 event_receipts 的 PUBLISHED 状态在同一个事务中提交。点击、转化只有在曝光已有发布凭证时才能被 relay 领取，避免跨 relay 发布乱序。
7. 消费前批量校验已受理载荷和 SETTLED 证明，再进入原有 eventId 幂等计量。无证明的旧消息或重放不能绕过结算。已处理的历史事件可以按原受理载荷幂等重放。

入口与结算完成都会锁定同一 requestId 的任务行。因此，点击在结算完成的同时进入，也不会永远留在 SETTLING。

## 故障处理边界

| 故障 | 处理 |
| --- | --- |
| Outbox 事务失败 | 收据、结算任务、事件一起回滚 |
| Redis 临时不可用 | 后台指数退避重试，不需要客户端再次提交 |
| Redis 已成功、返回结果丢失 | 重试读到同一凭证，不再次扣费 |
| Redis 已成功、MySQL 回写失败 | 租约到期后可由新 worker 接管，复用原凭证再完成回写 |
| worker 退出或旧 worker 迟到 | 新 owner 接管；完成和失败回写均检查租约 |
| 缺失、已过期或不匹配的预留，且无成功凭证 | RECONCILE，停止发布和计量 |
| 连续 8 次结算失败 | RECONCILE，保留最后错误供核对 |
| Kafka 已确认、发布状态回写失败 | 按原 Outbox 重试机制重新发布；消费者按 eventId 去重 |
| 曝光成为死信 | 点击和转化继续等待；曝光重放并成功发布后才解除依赖 |

原决策预留通常只有 30 秒。长时间故障导致预留失效时，本次不会重新抢占或盲目补扣预算，因为预算可能已被其他请求使用。RECONCILE 是明确暴露的待核对结果，不能宣传为“任何故障都自动恢复”。Redis 凭证丢失或超过保留时间时也不能推断已经成功。

运行态工作台和 Prometheus 增加 SETTLING / RECONCILE 数量、筛选与中文标签。待核对事件没有普通死信重放按钮。人工处理需先核验原事件、决策快照、成功凭证和账务；本次没有提供跳过核验的强制结算接口。

Kafka 配置要求 MySQL 决策库和 Redis 预留，避免重启或多个实例之间使用互不共享的内存状态。默认同步演示模式保持原有配置；上述持久化结算队列用于 Kafka 模式。

## 升级方式与已有数据

新迁移：`000010_event_settlements`，前置为已有迁移 000001–000009。升级前停掉旧版本 API、relay 和 consumer，再迁移并启动新版本，不能混跑旧的无结算校验 worker。

迁移会把尚未处理、又缺少新结算证明的历史 Outbox 事件转为 RECONCILE，包括旧版本已经发布但尚未处理的事件。它不会伪造历史结算证明。Kafka 中这些旧消息可能阻塞所在分区的重试，需要核对后制定数据处理方案；不能直接跳过 offset 当作财务问题已解决。历史已处理数据不自动重算或回滚。

回滚迁移会保留 SETTLING 事件为 RECONCILE，不会把它们直接恢复成可发布状态。生产升级及回滚应先在独立测试环境演练。

本轮未迁移用户当前数据库，也未重启 18080 / 18081 的演示 API，避免清空其内存数据。构建产物位于 `work/outbox-settlement/adflow-api-verified.exe`，现有浏览器后端仍需另行升级才会使用本次逻辑。

## 验证记录

- `go test -count=1 -json ./...`：221 个顶层测试通过，计入子测试共 279 个 pass 记录。
- 故障注入覆盖 Redis 故障、Redis 成功响应丢失、MySQL 完成回写失败：重新创建 worker 后恢复；同一事件重复消费后曝光为 1、扣费为 5 分。这里使用 miniredis 执行实际 Lua、内存队列模拟持久化状态及内存计量，属于受控回归测试。
- 32 个并发结算重试：实际 Lua 在 miniredis 中执行，扣费合计仍为 5 分，频控保留；这不是容量压测数据。
- sqlmock 覆盖受理事务回滚、曝光唯一绑定、点击依赖、结算前后状态、租约接管条件、旧 owner 完成/失败回写被拒、发布收据事务和消费证明。sqlmock 不验证 MySQL 实际锁行为和 SQL 执行计划。
- `go vet ./...` 与后端构建通过；最终 worker 调整另有定向回归日志。
- 前端 98 条测试、TypeScript 检查、修改文件的 lint 和生产构建通过。
- 本轮真实 MySQL/Redis/Kafka 联调未执行：未配置专用 `ADFLOW_IT_MYSQL_DSN`，3 项相关集成测试明确跳过。不能把它们算成通过或声称已完成真实故障切换验收。

原始记录在 [verification/outbox-settlement](verification/outbox-settlement)，包括 Go JSONL、前端输出、集成测试跳过原因和代码哈希。

新增真实依赖测试 `TestSettlementRestartFencesOldWorkerAndReleasesDependentEvents` 验证独立 worker 的租约接管、Redis 成功但未回写、旧 owner 被拒、并发受理依赖事件与完成事务、曝光发布前后的 relay 领取条件。使用专用空测试库配置 `ADFLOW_IT_MYSQL_DSN` 和测试 Redis 地址 `ADFLOW_IT_REDIS_ADDR`，再执行：

```text
go test -tags integration -count=1 ./tests/integration -run TestSettlementRestartFencesOldWorkerAndReleasesDependentEvents
```

该套基础设施测试会应用迁移，不要把专用测试 DSN 指向演示或业务库。Kafka 重复投递测试另需配置测试 broker、topic 和 dead-letter topic。

## 后续可量化工作

在独立 MySQL/Redis/Kafka 环境完成上述验收，再对比修复前后相同数据与负载下的入口延迟、结算等待时间、积压、吞吐和错误率。新链路增加持久化任务、结算回写及消费证明查询，旧版本的 100 iteration/s 数据不能直接视为新版性能。还需补结算记录与凭证的留存策略，以及人工核对后的受控恢复流程。
