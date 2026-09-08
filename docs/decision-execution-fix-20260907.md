# 请求幂等与预占所有权修复（2026-09-07）

修复竞价审查中前两项问题。Outbox 结算失败的主动补偿没有在本轮实现，继续作为独立待办。

## 协议

1. requestId 绑定规范化后的 requestId、userId、slotId。临时画像入口还绑定去重标签和字段的规范化摘要；标签顺序变化不误判为不同请求。
2. 已有结果先验证参数再复用；不同参数返回 HTTP 409 / request_id_conflict。历史无指纹记录只在原用户/广告位精确一致的普通请求上兼容复用。
3. 开始执行前取得请求执行权。内存由共享 Runtime 协调；MySQL 使用 decision_requests 唯一键、短事务行锁和数据库时间租约。最多等待 250ms，受外层请求 deadline 约束；仍在执行时返回 409 / decision_in_progress，并提供 Retry-After。
4. 每次执行使用随机 owner，每个候选的预占 token 由 owner 与 campaignId 派生。不同执行和不同候选的释放路径不再使用同一个 requestId token。
5. 保存结果时再次检查 owner、指纹与未过期租约；MySQL 检查、写入结果和清除执行权处于同一事务。旧执行不能在新 owner 接管后提交或解锁新 owner。
6. 保存结果报错先回读。若已落库，保留成功结果的预占；只有明确未提交才能主动释放。无法确定的提交结果保留本次独立预占，最多等原有 30 秒 TTL 到期，不冒险撤销可能已成功的决策。

MySQL 新迁移为 000009_decision_execution，增加 decision_requests 和 decisions.request_fingerprint。新接口是 DecisionStore 的必需协议，避免生产适配器悄悄退化为进程内锁。

## 生命周期和兼容性

- 内存请求绑定及决策结果保留 5 分钟；SQL 绑定跟随持久历史保留，没有本轮自动删除策略。内存新进程不具备跨重启幂等历史。
- 预算/频控预占仍为 30 秒，结果保留不会延长曝光权限。同步与异步入口用服务器当前时间拒绝过期曝光；已接受并持久化的异步事件仍可由消费者按原事件时间处理。
- 新结果时间统一到毫秒，避免 MySQL DATETIME(3) 与返回值纳秒差异引起比较问题。
- 旧预占 token 仍能被现有确认和释放适配器处理。当前服务没有重启；部署需先迁移并统一更新参与协调的 API 实例，不能把新旧混跑当成已验证场景。
- MySQL 协调依赖共享决策库；两台各自独立 memory 决策库不会形成分布式幂等。正确性带来的额外事务成本需要后续性能复测，不能沿用旧压测数字当成新版结果。

## 证据

- [上轮失败复现](verification/auction-review/repro-results.jsonl)：一次成功、一次失败后，5 分预算与 1 次频控名额可以被再次占用。
- [新回归输出](verification/decision-execution/regression.jsonl)：两份 Service 共享仓储，同一请求只提交一次、结果一致且预占仍存在；覆盖等待方取消、提交确认丢失、不确定提交到期、旧租约 fencing、参数指纹和画像摘要。
- [独立 HTTP 输出](verification/decision-execution/http-smoke.json)：32 个相同请求并发，32 份结果一致；换用户/广告位/临时画像为 409；标签顺序变化可重放；重复曝光增量为 1。
- MySQL 单元测试验证指纹冲突、忙碌/过期租约、所有权检查和 COMMIT 不确定错误分类；它们使用 sqlmock，不等于真实数据库验收。

提供 opt-in 的真实依赖测试 TestExecutionCoordinationWithRealMySQLAndRedis：创建独立随机测试库、应用全部迁移，用两份独立连接池和同一 Redis 测试命名空间并发验证，最后清理自己创建的库与键。本机配置的 MySQL 账号认证失败（1045），本轮该项明确跳过，没有修改业务库。

运行入口：设置 ADFLOW_IDEMPOTENCY_MYSQL_DSN、ADFLOW_IDEMPOTENCY_REDIS_ADDR，必要时设置 ADFLOW_IDEMPOTENCY_REDIS_PASSWORD，再执行 `go test -tags=integration -count=1 ./internal/decision/application -run TestExecutionCoordinationWithRealMySQLAndRedis`。凭据只通过本机环境配置，不写入日志或文档。

Go 全包测试、vet 和 API 编译通过；本机未运行 race（无 CGO/C 编译器），CI 的 Linux race job 继续覆盖普通测试。此次只启动了独立的 18082 验证服务，未重启原有 18080/18081 预览后端。
