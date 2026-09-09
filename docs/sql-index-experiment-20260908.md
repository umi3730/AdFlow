# Outbox 请求联合索引：独立对照实验

后续已补充[HTTP 到统计落库的全链路对照](full-chain-experiment-20260908.md)：正常队列未观察到明显提速，静态待结算积压场景有显著收益。不要将本页单 SQL 数据直接当作接口数据。

## 结论

2026-09-08 在本地 MySQL 8.0.46 中，分别构造 10 万和 100 万条 Outbox 事件，验证已有迁移 `000011` 的 `(aggregate_key, status)` 对请求级查询和结算更新的收益。

在 **100 万条、约 5% 待结算事件、每请求 3 条事件**的合成数据下：

- 同条件查询的实际检查行数从 **49,995 降到 3**。
- 生产 Outbox 更新 SQL 的服务端中位耗时从 **126.44ms 降到 0.61ms**，P95 从 **164.52ms 降到 1.09ms**。
- 每种模式每条 SQL 采集 **60 个正式样本**，分 6 轮交替 AB/BA；并非一次执行或挑选最好结果。

这证明该索引显著减少了这条请求级 SQL 的扫描工作，不代表整个 API 或生产系统提升同样倍数。此次未修改业务源码、业务库或追加迁移；索引本来就已存在，本轮补的是独立实验和证据。

## 为什么测试这条 SQL

`internal/event/adapter/mysql/settlement.go` 的 `CompleteSettlement` 在完成曝光结算后，把该请求对应的 Outbox 事件从 SETTLING 改成 PENDING：

```sql
UPDATE event_outbox SET status = 'PENDING', attempts = 0,
  next_attempt_at = UTC_TIMESTAMP(3), last_error = NULL
  WHERE aggregate_key = ? AND status = 'SETTLING';
```

原有 `idx_event_outbox_claim(status, next_attempt_at, locked_until, created_at)` 服务于队列领取。它能定位所有 SETTLING 事件，但没有 `aggregate_key`，仍需逐条筛选目标请求。新增索引把请求和状态一起定位。本实验没有修改驱动插值、SQL 条件、表内数据和其他索引。

程序启动时核对测试 UPDATE 与业务源码 SQL 一致，并记录源码、迁移和实验程序 SHA256。

## 方法与环境

| 项目 | 设置 |
| --- | --- |
| 测试时间 | 2026-09-08，北京时间；精确 UTC 起止见原始 JSON |
| CPU | Intel Core i7-8700 @ 3.20GHz，12 个逻辑 CPU |
| 系统 | Windows 上的 WSL2 Ubuntu；Linux 6.18.33.2 |
| WSL 内存 | 约 15.5 GiB |
| MySQL | 8.0.46；InnoDB buffer pool 128 MiB |
| 执行位置 | Linux Go 二进制与 MySQL 同在 WSL，Unix socket 连接 |
| Go / 驱动 | Go 1.27.0；go-sql-driver/mysql 1.9.3，参数插值开启 |
| SQL 并发 | 1，固定单连接 |
| 事务隔离 | REPEATABLE READ |
| 数据 | 每请求曝光/点击/转化各 1 条；每种规模重新生成 |
| 状态分布 | 约 90% PUBLISHED、5% SETTLING、3% PENDING、1% PROCESSING、1% RECONCILE |
| 轮次 | 6 轮，每轮两种模式各 10 次 SELECT、10 次 UPDATE；块前 3 对预热 |
| 对照方法 | 同一物理表，目标索引不可见/可见切换；不 FORCE INDEX |
| 测量来源 | performance_schema 实际语句计数与 TIMER_WAIT；另存客户端计时 |

不可见索引仍然存在并有写入维护成本。这是**优化器是否可使用目标索引的对照**，不是物理删除与新建的所有成本对照。两种模式都保留原有队列索引，不能描述为“从无索引全表扫描优化”。

UPDATE 在显式事务中执行后回滚，始终断言影响 3 行。计时仅包含单条语句，排除 BEGIN、遥测查询、ROLLBACK 和真实提交 fsync；**不包含 CompleteSettlement 的另一条更新，也不是完整结算事务耗时**。

预热针对查询路径，不宣称全数据在内存。MySQL 为共享本地实例，其他服务未停止，存在环境噪声。没有清空全局缓冲池或修改全局参数。

## 原始结果汇总

以下均为服务端单语句耗时，单位 ms。每格对应 60 个样本。

| 事件行数 | SQL | 索引不可见：中位数 / P95 | 索引可见：中位数 / P95 | ROWS_EXAMINED 不可见 → 可见 |
| --- | --- | --- | --- | --- |
| 100,000 | SELECT | 9.273 / 11.273 | 0.306 / 0.527 | 4,995 → 3 |
| 100,000 | UPDATE | 12.525 / 14.930 | 0.647 / 1.142 | 4,998 → 6 |
| 1,000,000 | SELECT | 99.692 / 123.046 | 0.290 / 0.492 | 49,995 → 3 |
| 1,000,000 | UPDATE | 126.444 / 164.520 | 0.606 / 1.089 | 49,998 → 6 |

UPDATE 的 ROWS_EXAMINED 是 MySQL statement instrumentation 的实际统计，不直接等同于 SELECT 扫描叶子项数；每次实际更新仍是 3 行。SELECT 返回同样的 3 行且包含非覆盖索引列，未用“只查索引”偷换原有数据访问成本。

百万行可见索引 UPDATE 有一次约 **20.22ms** 的服务端抖动，最大值和全部样本均保留，没有删除离群点。中位数与 P95 按完整样本计算。

目标索引在 10 万/100 万行时的统计分配空间分别约 **5.55 MiB / 54.69 MiB**。这是索引页空间观测，不含完整表空间，也未测插入吞吐或索引维护成本。

## 执行计划解释

原始报告包含 SELECT 的 `EXPLAIN ANALYZE`、SELECT/UPDATE 的 JSON 执行计划及 SHOW INDEX。

不可见时：

```text
idx_event_outbox_claim: status = 'SETTLING'
  -> 读取同状态的大量事件
  -> 过滤 aggregate_key，保留目标请求的 3 条
```

可见时：

```text
idx_outbox_request_status: aggregate_key = ? AND status = 'SETTLING'
  -> 定位目标请求的 3 条事件
```

随着历史数据增长，状态相同的事件也增加，而每个请求仍只有少量事件，因此请求维度的选择性更适合这条 SQL。原有状态索引仍用于领取任务，不能因本实验结果将其删除。

## 验证与复现

- 先用 1000 行跑通，再执行两种正式规模。
- 所有正式测量共 **480 条单语句样本**，每次查询/更新均验证 3 行；预热另计，不混入汇总。
- 实验结束检查所有状态计数未变化，两个正式隔离库均已删除，检查无遗留 `adflow_idxbench_*` 库。
- 独立 Python 校验器重算中位数/P95，核对每轮请求配对、受影响行数、索引可见性和源码摘要。
- Go 实验程序通过编译和 vet，Linux 二进制实际执行成功。业务 API `/readyz` 复查 MySQL/Redis 均为 up。

文件：

- [实验程序与运行说明](../tests/bench/outbox-index/README.md)
- [正式原始报告](verification/sql-index/results.json)
- [小规模验证](verification/sql-index/smoke.json)
- [独立统计复核](verification/sql-index/verification.json)

复核已有报告（仓库根目录，需 Python）：

```sh
python tests/bench/outbox-index/verify.py docs/verification/sql-index/results.json
```

## 结果适用范围

核心测量结果：

> 针对曝光结算按请求更新事件时扫描大量同状态记录的问题，采用 `(aggregate_key, status)` 联合索引；在本地 100 万条合成事件、6 轮索引可见性对照中，将该 UPDATE 的服务端 P95 从 164.52ms 降至 1.09ms，同条件查询检查行数由 49,995 降至 3。

计时与环境边界：MySQL 8.0.46、单并发、回滚更新、非完整事务提交；对照保留其他索引和同一数据集，只切换目标索引可见性。效果依赖请求选择性、状态分布和环境，不能说生产接口耗时或 QPS 提升同样倍数。

尚未验证：高并发锁等待/死锁变化、真实提交与磁盘持久化开销、索引对插入吞吐的影响、其他状态分布下的收益。没有把这些写成已完成成果。
