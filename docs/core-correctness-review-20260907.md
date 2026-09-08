# 核心链路代码审查（2026-09-07，后续轮次）

后续状态：四项已修复并完成回归，见 [修复与验证](core-fixes-20260907.md)。下文保留原审查结论和失败证据。

本轮审查当前决策、画像缓存、事件入口、预算确认和集成测试清理逻辑。审查未修改业务实现或重启 API；只新增隔离复现和记录。临时数据库与本轮 Redis 命名空间已经清理。

## P1：MySQL 与 Redis 对用户 ID 的语义不同

位置：`migrations/000004_profiles.up.sql:7`、`internal/decision/adapter/mysql/profile_store.go:48` / `:63`、`internal/decision/adapter/profilecache/store.go:200`、`internal/decision/application/service.go:127`。

user_profiles 使用 utf8mb4_unicode_ci，数据库会把某些大小写不同的 user_id 当成同一行。缓存、缓存代次和频控键却直接使用原始 userID，区分大小写；数据库查到画像后也使用请求中的 userID 构造返回值。

真实 MySQL/Redis 复现：

- 小写 ID 保存 score=10 并读入缓存。
- 大写同名 ID 保存 score=99，MySQL 更新同一行，但只失效大写形式的缓存键。
- 数据库小写查询返回 99，缓存小写查询仍返回 10。
- 在同一画像、同一计划、frequencyLimit=1 下，分别用小写与大写 ID 发起决策，两次都成功取得广告及资源预留。

这不是之前“同一个缓存键的旧查询回填”问题复发，而是一个数据库身份被拆成多个缓存/频控身份。当前 mysql-redis + Redis 预留模式可触发。

建议先明确 ID 是大小写敏感还是规范化标识，再统一 MySQL 主键、读取返回值、缓存、更新/删除失效、决策与频控。仅在某一个 Redis 键上转小写不足以处理排序规则的全部等价关系；已有数据也需要冲突检查。

## P1：同步入口缺少完整事件身份校验

位置：`internal/event/application/service.go:55`–`:64`、`internal/event/adapter/memory/store.go:23`–`:25`。

同步事件存储仅按 eventId 去重，发现重复后返回 false，而 Service 仍按本次输入对应的决策确认预算和频控，没有核验它是否就是原先的事件。

使用真实内存事件存储与 miniredis 执行 Redis 脚本复现：

- r1、r2 各预占 5 分；先提交 eventId=e、requestId=r1 的曝光。
- 再提交相同 eventId=e、requestId=r2，接口逻辑返回 created=false、err=nil。
- 统计只有 1 次曝光，但 Redis spent 变成 10 分；第二次决定也被确认扣费。

另一个复现：同一 requestId 使用两个不同 eventId 回传曝光，两次均计入统计，但只确认一次预留，出现 2 次曝光、5 分花费。

影响范围是仍被支持的同步模式，当前部署的 Kafka 入口已有事件身份和曝光唯一性约束，不走这条确认路径。默认纯内存同步模式同样存在一个决策重复计入曝光的问题。

建议统一两种入口的 eventId 参数绑定和每请求唯一曝光规则。重复 ID 应复用原事件，参数冲突应报错；不要只按 created=false 跳过确认，否则可能破坏“上次记录成功、确认失败”的合法恢复。

## P2：后续点击/转化被曝光预留有效期限制

位置：`internal/event/application/async_service.go:48`–`:52`；同步入口与消费校验还有相同的 occurredAt / ExpiresAt 比较。

现在所有事件类型都检查原决策 ExpiresAt，而决策预留有效期为 30 秒。隔离测试先成功受理并写入曝光，再将时间推进一分钟，点击直接得到“advertising decision has expired”。生产异步入口在查询结算状态之前就会拒绝，因此即使原曝光已经结算也不能解决。

当前自动模拟立即回传后续事件，所以普通演示容易看不出。若项目要支持曝光后较晚点击或购买，就会漏掉这类正常行为。若产品明确只支持三十秒内的整套演示，需要把这一限制明确说明，而不是将其视作真实点击/转化归因能力。

建议区分曝光预留期与点击/转化归因窗口：后续行为依据已经受理/结算的曝光及独立归因期限校验，继续绑定原决策，不重新预占或计费。

## P2：部分真实依赖测试在关闭客户端后才清理 Redis

位置：`tests/integration/settlement_recovery_test.go:21` / `:31`、`tests/integration/profile_generation_test.go:39` / `:41`、`tests/integration/infrastructure_test.go:309`。

这些测试通过 defer client.Close() 关闭 Redis 客户端，却通过 t.Cleanup 注册 deletePrefix。测试函数返回时 defer 先执行，随后 cleanup 使用已关闭的客户端；deletePrefix 遇到 Scan 错误直接返回，删除错误也被忽略，因此 Redis 清理会静默失败。

这是静态控制流确认的测试生命周期缺陷，不是本轮业务数据故障统计。带较长 TTL 的预算或幂等测试键会残留，影响长期反复运行的测试环境。应将关闭客户端也交给 t.Cleanup，并保证删除先执行，清理错误应使测试报告失败。本轮新增的独立复现使用清理后关闭的注册顺序，并另行清理了首轮临时键。

## 证据及范围

- [事件入口原始复现](verification/core-review/event-repro.jsonl)：三条安全/业务期望断言失败，分别对应多扣、重复计量、延迟点击被拒。
- [真实依赖身份复现](verification/core-review/identity-repro.jsonl)：独立 MySQL 测试库与 Redis 前缀，确认只有一个数据库用户，却出现缓存不一致和两个频控身份。
- 复现源码以 `.go.txt` 存放，使用 Go overlay 临时加入，不混入正常 CI。
- [现有普通测试结果](verification/core-review/existing-tests.jsonl) 独立保存；现有测试通过不代表新增边界已覆盖。

同步复现使用 miniredis 和内存决策/事件存储；延迟回传使用冻结时间和受控入口依赖。只有身份等价复现连接了真实 MySQL/Redis。没有向正在使用的广告计划发送这些异常事件，没有修改其预算和统计。

建议优先修复身份语义和同步幂等，再定义延迟行为窗口；测试清理可一起修复。本轮未把之前已明确记录的 UTC 日界线、缓存跨存储崩溃窗口、预算均匀消耗等设计边界重复列成新缺陷。
