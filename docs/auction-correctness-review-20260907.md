# 竞价后续正确性审查（2026-09-07）

本轮仅审查和复现，没有修改业务实现、迁移数据库或重启服务。以下都是待修复项，不是已完成优化。

后续进展：前两项的代码修复和新增回归已完成，见 [请求执行修复](decision-execution-fix-20260907.md)。本文件保留发现问题时的记录；第三项主动结算补偿仍待处理，真实 MySQL 验证尚未完成。

上轮验证覆盖不同 requestId 的预算并发以及同 requestId 的串行重试；本轮增加并发重复请求和结算失败条件。已有正常路径测试通过不代表这些条件已覆盖。

## P1：重复执行的失败分支会释放成功决策的预占

`internal/decision/application/service.go` 先查结果，再分别预占、保存。两条相同 requestId 的请求可能同时查不到结果，复用相同的预占 token，再提交不同 ExpiresAt 的结果。一条保存成功，另一条冲突后无条件 releaseReservations，释放的是共享 token。

隔离复现使用真实内存仓储和预占实现，通过 barrier 固定两个请求同时未命中和完成预占的执行顺序：

- 同 requestId、同用户、同广告位；日预算 5 分，单次 5 分，每人频控 1 次。
- 一条返回成功、一条失败。
- 两条执行结束后，另一请求仍能成功预占完整 5 分预算和频控名额，尽管成功决策仍存在。

修复需要明确同 requestId 的执行所有权、结果复用以及失败分支对预占的释放权限。单进程 mutex/请求合并只能覆盖单实例；多实例还需要持久化协调或等效协议，不能直接宣称加锁后分布式幂等成立。

## P1：requestId 没有绑定关键入参

结果命中后直接返回 existing，没有验证用户和广告位是否与存量请求一致。

复现：先执行 `(same-id, alice, slot-a)`，再使用相同 ID 改为 bob 或 slot-b。当前仍返回 alice/slot-a 的历史结果，且无错误。

应绑定规范化后的关键请求参数或请求摘要。相同 ID、相同参数复用原结果；相同 ID、不同参数明确报冲突。临时画像随请求携带时，也应明确其参数绑定范围。

## P1：曝光计量和结算之间缺少主动恢复保障

`AsyncService.Record` 先写 Outbox，再确认 Redis 预算/频控。确认失败时接口返回错误，但 Outbox 已经持久化；生产装配中的消费端 processor 没有 reservation confirmer。

复现模拟相同装配方式：预算确认报错，Outbox 已有事件，将该事件交给无 confirmer 的消费处理器，曝光统计增加 1；没有发生成功的预算确认或客户端重试。

这证明事件可以在结算失败后继续被计量，不等同于已经测得生产环境少扣款金额。后续需要持久化结算阶段、幂等补偿/重试及对账，覆盖「Outbox 已提交、Redis 确认失败、客户端不再重试」。仅把两个操作交换顺序不能解决跨存储原子性。

## 证据与复现范围

- [原始输出](verification/auction-review/repro-results.jsonl)
- [决策复现源码](verification/auction-review/decision_repro_test.go.txt)
- [事件复现源码](verification/auction-review/event_repro_test.go.txt)
- [代码摘要](verification/auction-review/metadata.json)

源码通过 Go overlay 临时加入相应测试包，文件以 `.go.txt` 留档，不参与普通 CI。这些测试断言期望的安全行为，在当前实现上预期失败；失败是缺陷复现，不是验证通过。

测试没有连接真实 MySQL/Redis/Kafka；并发问题使用真实内存实现，事件问题使用内存计量和可控 Outbox/失败确认器。不能据此宣称已完成跨实例或真实基础设施故障测试。复现 barrier 固定当前实现的执行路径，后续修复应另建正常回归门禁，不照搬 barrier 作为新架构的测试约束。

## 建议下一步

2026-09-07 更新：请求绑定及执行权修复见 `decision-execution-fix-20260907.md`；Outbox 主动结算、消费校验与验证边界见 `outbox-settlement-fix-20260907.md`。以下保留原审查时的建议，真实基础设施验收仍待完成。

先把请求参数绑定、执行协调和失败补偿边界一起设计清楚，优先修前两项；再完善 Outbox 与结算的主动恢复机制。随后审计 MySQL 毫秒时间精度与返回值一致性、内存和持久化模式的幂等保留周期，补充真实依赖验证。
