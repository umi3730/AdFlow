# Outbox 请求联合索引对照

验证迁移 `000011` 的 `(aggregate_key, status)` 能否改善按请求查找/更新待结算事件。只测试这一索引的优化器访问路径，不混入驱动、连接池、缓存、队列领取或应用代码变化。

## 运行条件

- MySQL 8.0，开启 `performance_schema`，且 `events_statements_history` 消费者已启用。程序不会修改全局设置。
- Go 版本遵循仓库 `go.mod`。
- 执行账号需要创建/删除隔离库、DDL/DML、读取 `performance_schema` 与 `mysql.innodb_index_stats` 的权限。
- 从仓库根目录执行。默认连接 WSL 本机 Unix socket，通过 root 的 socket 认证；不读取项目 `.env`。
- 可通过 `ADFLOW_INDEX_BENCH_DSN` 指定专用测试实例，但 DSN **必须不包含数据库名**。不要在命令参数、日志或报告中暴露凭据。

Windows + 本项目 WSL 原生 MySQL：

```powershell
New-Item -ItemType Directory -Force work/sql-index | Out-Null
# 在独立 PowerShell 进程中编译，或在当前终端编译后恢复 GOOS/GOARCH。
$env:GOOS = 'linux'
$env:GOARCH = 'amd64'
go build -o work/sql-index/outbox-index-linux ./tests/bench/outbox-index
Remove-Item Env:GOOS
Remove-Item Env:GOARCH
wsl.exe -d Ubuntu -u root -- bash -lc 'cd /mnt/d/AdFlow && ./work/sql-index/outbox-index-linux -output work/sql-index/results.json'
```

Linux 本机：

```sh
go run ./tests/bench/outbox-index -output work/sql-index/results.json
```

默认 100000、1000000 条数据，6 轮，每种模式每轮每条 SQL 10 个正式样本，块前预热 3 对查询/更新。快速检查可用 `-rows 1000 -rounds 2 -samples 2 -warmups 1 -output work/sql-index/smoke.json`。

## 数据与实验控制

1. 每次新建 `adflow_idxbench_<UTC时间>_<规模>` 数据库，名称由程序生成，禁止指定业务库。使用 `000002` 原始建表，包括事件回执外键、主键和原有任务领取索引。
2. 确定性合成数据，每请求 3 条事件：曝光、点击、转化。约 90% PUBLISHED、5% SETTLING、3% PENDING、1% PROCESSING、1% RECONCILE；尾部不满分组的偏差保存在实际计数中。
3. 建立目标联合索引并执行 ANALYZE TABLE。**同一物理索引在不可见/可见模式切换**，`use_invisible_indexes=off` 固定不变；不加 FORCE INDEX，记录优化器自己的选择。不可见索引仍有维护成本，不代表物理删除索引，也不能用于测新增索引对 INSERT 的成本。
4. 6 轮交替 AB/BA，每轮使用相同配对请求 ID，访问不同请求；每个块先预热。数据不必全部驻留在 128 MiB 缓冲池内，不能将它描述为全内存热缓存基准。
5. SELECT 读取与更新同一条件下的相关列。UPDATE 与 `CompleteSettlement` 中的 Outbox SQL 保持一致；脚本启动会核对源码中的 SQL，变化时直接失败。
6. UPDATE 在显式事务中运行并回滚，每次断言正好影响 3 行。计时仅覆盖 SQL 执行/结果读取，排除 BEGIN、遥测查询、ROLLBACK，以及真实提交的 fsync。它不是完整结算事务测试。
7. 记录客户端单语句耗时、performance_schema 服务端 TIMER_WAIT/LOCK_TIME、实际 ROWS_EXAMINED/ROWS_AFFECTED/ROWS_SENT。UPDATE 的 ROWS_EXAMINED 可能包含更新阶段的额外记账，不要误说其与 SELECT 扫描叶子项数完全等同。
8. 保存 SELECT 的 EXPLAIN ANALYZE、SELECT/UPDATE 的 JSON 执行计划及索引可见性。UPDATE 使用普通 EXPLAIN，不通过 EXPLAIN ANALYZE 意外执行 DML。
9. 结束校验状态分布未变，删除本次新建隔离库。失败时保留隔离库和可用的部分结果，便于排查；清理时只针对报告中的精确隔离库名。

## 指标解释

- 中位数为排序后中间值（偶数取两个中间值平均），P95 采用 nearest-rank；原始逐次数据一并保存。
- 硬件、缓冲池、客户端位置、行数、状态分布、索引可见性、预热和单并发约束必须与结果一同说明。
- `LOCK_TIME` 是 statement instrumentation 提供的指标，不能当成完整 InnoDB 行锁等待时长。本实验没有并发竞争，无法推导死锁消除或高并发吞吐收益。
- 不可见对照仍可使用原有状态索引。不要将其笼统描述为“无任何索引”或“全表扫描”。
- 不代表生产 QPS、完整 API 延迟或当前真实业务数据规模。运行在共享本地 MySQL 时，其他服务负载仍可能带来噪声。

实测结果见仓库 `docs/sql-index-experiment-20260908.md`。
