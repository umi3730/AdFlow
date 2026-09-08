# JMeter 用户画像单接口对照

测量 `GET /v1/profiles/{userId}`：同一个生产 API 二进制，两组仅修改 `ADFLOW_PROFILE_STORE=mysql` / `mysql-redis`。保留 JWT，均经过正常 handler、JSON 编解码与 HTTP 返回，不走临时用户模拟接口。

Windows 仓库根目录：

```powershell
.\tests\load\run-jmeter-profile.ps1
```

需 WSL Ubuntu 的 MySQL root socket、redis-server、Java 8+。本轮实际使用 JMeter 5.6.3、OpenJDK 21。脚本下载官方 JMeter（若缺少）并验证 SHA512，编译 Linux API/隔离管理程序，通过非 GUI 模式运行 JMeter。所有依赖保留在忽略的 work 目录，不修改系统 Java 或业务配置。

可选参数：`-Seconds 15 -Threads 20 -Rounds 3`。它们表示**每组固定线程数、持续时间、配对轮数**，不是固定 QPS。程序为每组记录实际请求数与测量时间跨度，计算完成吞吐。

## JMX 内容

`tests/load/profile-query.jmx` 可以在 JMeter GUI 中打开查看结构；正式执行用 `-n`。

1. Test Plan 从子进程环境读取临时登录 token，不写进 JMX/JTL 或命令行。
2. CSV Data Set 从测试生成的 1000 个用户 ID 循环读取，所有线程共享文件游标。
3. Header Manager 添加 Bearer token。
4. 两个断言检查 HTTP 200，以及响应包含当前请求的 userId 与 score=88。
5. Warmup 线程组每线程 20 次请求；正式组同线程数持续指定秒数。线程组串行执行，Warmup 不进入正式分位数/吞吐。

每组先由 Go 对全部 1000 个用户进行 HTTP 预热，两种模式都执行，以便 MySQL buffer pool 也热；Redis 组随后应全部缓存命中。两组各自重新启动 API 和 JVM，采用 AB、BA、AB 的顺序。JMeter 没有思考时间，没有 GUI 监听器渲染负担。

## 指标与校验

- JTL 保留每次 sample 的 timeStamp、elapsed、HTTP code、断言成功状态等，原始毫秒计时可能使极短请求出现 0/1ms，不要伪造微秒级精度。
- 正式统计只看 `GET profile` 标签；`warmup` 仍保留在文件里。吞吐为正式 sample 数 / 从最早开始至最晚完成的时间跨度。
- p50/p95/p99 使用 nearest-rank。跨轮汇总需要合并原始 elapsed；不能平均三个 P95。
- Performance Schema `table_io_waits_summary_by_table.COUNT_FETCH` 测量用户表实际读取增量。直接 MySQL 模式应与整个 JTL 请求数（含 JMeter warmup）一致；Redis 模式应为 0。
- `events_statements_summary_by_digest.COUNT_STAR` 也记录，但本机并发预检出现少量欠计，保留为诊断计数，不强称其与真实执行数严格相等。未通过修改全局统计设置“修正”结果。
- Redis 模式同时校验 Prometheus `hit` 增量 = 全部 JMeter sample 数，且 miss/fill 为 0。两种模式的 Go 全量预热都发生在采集基线之前，不混入增量。
- 失败断言、无正式样本或读取/缓存计数不符会使管理程序非零退出。JMeter 非 GUI 进程退出码为 0 本身不代表所有断言都通过。

## 隔离与边界

只创建新的 `adflow_jmeter_*` 测试库、独立 API 及独占 Redis 进程；不读取 `.env`，不修改业务库/Redis，不产生 Kafka 流量。退出时清理隔离资源并保留 JTL/日志/环境及源码摘要。

这是热缓存单接口实验，不测冷启动、击穿、空值缓存、跨实例更新一致性或缓存故障回源，也不是数据库慢查询测试。MySQL 的热主键查询本身很快，因此必须接受“数据库读取减少，但接口时延未明显改善甚至局部变慢”的结果。

实验记录见 `docs/jmeter-profile-experiment-20260908.md`。
