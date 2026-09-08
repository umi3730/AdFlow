# 死锁恢复与结算租约修复

后续长测：本文的 50 轮/秒三组 60 秒结论仍为当时实测；延长到 120 秒后发现过载边界，45 轮/秒通过三组且队列平稳，见 [容量续测](kafka-capacity-followup-20260908.md)。

## 最终结果

修复已完成并更新本机 18080 / 18081 API，无新数据库迁移。最终真实 Kafka 复测以 **50 轮业务/秒**运行三轮各 60 秒：

| 轮次 | 正式业务轮数 | HTTP 错误 / 未派发 | 统计提交可见 P95 | 停发后排空 |
| --- | ---: | --- | ---: | ---: |
| 1 | 3001 | 0 / 0 | 1.342s | 0.68s |
| 2 | 3001 | 0 / 0 | 1.309s | 0.79s |
| 3 | 3001 | 0 / 0 | 1.436s | 1.34s |

共 **9003 次正式业务、27009 条正式事件**全部完成结算、发布、消费和统计，无 RECONCILE。含每轮 3 次预热后，每阶段账目按 3004 次曝光核对；重复事件未重复计数/扣费，Redis 预占余额为 0，Kafka 三分区最终 lag 为 0。

每秒队列采样的初段/末段平均待处理事件数分别为 114→100.6、100.6→99.6、116→124.2，没有出现之前 60 轮/秒时显著的积压增长。这个结果只验证当前本机 50 轮/秒负载，不宣称绝对容量或任意故障都能恢复。

## 问题与修复

此前 [Kafka 复测](kafka-capacity-20260908.md) 捕获 MySQL 1213 死锁。worker 在批处理中途返回，未处理任务继续持有 30 秒租约，接管时其 Redis 30 秒预占已过期，最终进入待核对状态。

### 未完成任务归还与短租约

- 新增 `ReleaseSettlements`，按 requestId、eventId、PROCESSING 状态和本次 owner 条件归还任务。
- 只处理仍归该 worker 所有的未完成任务；不修改 SETTLED、RECONCILE、已显式重试调度的 PENDING 或新 owner 的任务。
- worker 失败、取消或提前退出时，通过脱离取消信号、但限时 1 秒的清理上下文归还剩余任务。清理失败并入返回错误，不静默丢弃。
- 每批从 10 条调整为 4 条，租约从 30 秒调整为 15 秒。领取限时 2 秒，单任务 Redis 确认与数据库完成共用 2 秒期限，失败记录最多 1 秒；四条任务的有界处理预算保留租约余量。
- 清理不可用或进程中断时，更短租约为新预占的剩余有效期留出接管机会。

这不是保证任意长中断都能恢复。若曝光很晚才回传，或依赖不可用时间超过剩余预占有效期，仍可能需要 RECONCILE；没有延长或取消原有 30 秒预占保护。

### 明确回滚后的事务重试

涉及结算完成、失败调度、租约归还的事务使用 READ COMMITTED，减少原死锁中的间隙锁影响。仅对 MySQL **1213（死锁）和 1205（锁等待超时）**在明确执行整笔回滚后有限重试：最多尝试 3 次，退避 10ms / 20ms，并响应 Context 取消。

提交确认失败可能已经落库，不能直接当成未提交，因此 **Commit 错误不盲目重试**。worker 通过 owner 条件归还和既有 Redis 结算凭据恢复；已提交 SETTLED 的行不会被清理降回 PENDING。

### 复测中新发现的入箱/发布死锁

第一阶段修复后，已受理的事件都完成了，但一次 Kafka 复测出现 3 次事件入口 HTTP 500。死锁报告显示：

```text
入箱事务持有 event_receipts 行锁，等待 event_outbox 间隙
发布回执事务持有 event_outbox 锁，等待 event_receipts 行锁
```

进一步把事件入箱、单条/批量发布回执更新纳入同样的 READ COMMITTED 与有界事务重试。重试单位包含两张表的完整状态变化，不能只重试最后一条 SQL。

同时保留重复事件的回滚语义：验证为重复时，回滚本次可能暂存的旧数据兼容结算行，返回 recorded=false，不错误提交新任务。既有重复事件回归仍通过。

## 验证证据

### 先失败、后通过的 worker 回归

新用例在旧 worker 上复现：完成状态写入失败、失败状态写入失败后，三条任务仍处于 PROCESSING；清理错误也没有返回。旧代码与失败日志保留在验证目录。

修复后覆盖：

- 批中第一次/中间一次持久化失败，剩余任务立即可接管，已完成前缀不受影响。
- Redis 已确认后取消，不误记 RECONCILE；新 worker 通过结算凭据继续，不重复扣费。
- 清理失败时保留错误，新 worker 在较短租约后仍能处理有效预占。
- owner 已更换、任务已完成时，旧 worker 清理不得覆盖状态。
- 事务整体重试、尝试次数上限、取消、Commit 确认不明不重试、入箱重复回滚语义、双表发布回执原子重试。

### 真实 MySQL / Redis

- 在专用隔离库中构造两笔反向持锁事务，让 MySQL 实际产生死锁。验证被回滚事务重试后两个计数各只增加两次，没有留下部分事务结果；**重复三次通过**。
- 使用真实 MySQL/Redis 注入完成写失败，验证任务立即归还、新 owner 领取、旧 owner 不能释放新租约、重启 worker 恢复三条曝光且只扣 15 分；**重复三次通过**。
- 原有重启接管、旧 owner fencing、曝光先行的集成测试也重复三次通过。

真实死锁测试验证了重试机制；Kafka 复测验证完整链路。它们不等于证明今后永不发生死锁，重点是可识别的瞬时错误能够正确恢复。

### 最终运行检查

- Go 全量回归与 `go vet ./...` 通过；修改后的测试再次验证通过。
- 最终 Kafka 的三个阶段均通过预设门槛，完整观测覆盖全部请求。
- API 日志未发现未处理的 ERROR/WARN、HTTP 500 或待核对告警；这不意味着内部从未发生并被重试的死锁。
- 本机两个 API 已切换新二进制，MySQL/Redis 就绪，异步 Kafka 模式保持开启，前端 3001 返回 200。
- 业务库原有队列统计保持：settling=0、pending=0、processing=0、reconcile=42、published=1426。历史待核对记录未删除、未自动重放。
- 所有实验库、专用 Topic/消费组与临时 Redis 已清理。集成测试使用唯一 Redis 前缀，并只清理该前缀。

## 留档

- [旧代码与失败回归](verification/settlement-recovery-fix/before-tests.log)
- [修复后批量恢复回归](verification/settlement-recovery-fix/after-batch-tests.log)
- [真实 MySQL 死锁回归](verification/settlement-recovery-fix/real-deadlock-final.log)
- [真实存储接管回归](verification/settlement-recovery-fix/real-stores-final.log)
- [中间版本发现的入口死锁](verification/settlement-recovery-fix/intermediate-ingress-deadlock.txt)
- [最终 Kafka 原始记录](verification/settlement-recovery-fix/kafka-final/proof.json.gz)
- [最终 Kafka 独立核验](verification/settlement-recovery-fix/kafka-final/verification.json)
- [最终队列趋势](verification/settlement-recovery-fix/kafka-final/queue-trend.json)
- [源码摘要与本地更新记录](verification/settlement-recovery-fix/metadata.json)

完整复跑入口仍为：

```powershell
.\tests\load\run-k6-walkthrough.ps1 -Capacity -CapacityRate 50 -ConfirmSeconds 60
```

此次没有提高 Outbox 的批量上限或降低其轮询间隔，没有把可靠性修复包装为 Kafka 最大吞吐提升。更高负载、长时间运行和更长依赖中断需要后续独立测试。
