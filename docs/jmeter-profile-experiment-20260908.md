# JMeter 单接口：MySQL 与 Redis 热缓存对照

## 结论

2026-09-08，使用真实 **Apache JMeter 5.6.3 非 GUI 模式**压测：

```http
GET /v1/profiles/{userId}
Authorization: Bearer <测试账号登录令牌>
```

同一 API 二进制、1000 个已保存画像，20 个线程，每组正式请求持续 15 秒，三轮配对，顺序 AB、BA、AB。两种模式都先预热，仅切换 `ADFLOW_PROFILE_STORE=mysql` / `mysql-redis`。

合并三轮原始正式样本后的结果：

| 指标 | 直接查 MySQL | Redis Cache-Aside 热缓存 |
| --- | ---: | ---: |
| 正式请求数 | 90,971 | 91,584 |
| 正式测量时间跨度之和 | 45.034s | 45.037s |
| 完成吞吐 | 2020.05 请求/s | 2033.53 请求/s |
| 平均响应时间 | 9.783ms | 9.721ms |
| P95 | 13ms | 13ms |
| P99 | 15ms | 16ms |
| HTTP/内容断言失败 | 0 | 0 |
| 用户表实际读取增量，含 JMeter 预热 | 92,171 | 0 |
| Redis 命中增量，含 JMeter 预热 | 不适用 | 92,784 |

**没有观察到稳定、明显的接口时延或吞吐收益。**缓存的明确收益是：本次已预热工作集完全命中，消除了这些请求对用户表的读取。不能据此写“接口提速数倍”或把约 0.7% 的吞吐差异包装成确定优化收益。

共 182,555 个正式请求全部正确；另有每组 400 次 JVM/HTTP 预热，六组共 2400 次，均正确。Go 的全工作集预热不计入上表的读取/命中增量。

## 为什么使用缓存后不一定更快

本次 MySQL 与 API 同处 WSL，走 Unix socket，1000 个用户均按主键读取，InnoDB 也已预热。Redis 同样需要通信、JSON 解码和后续 HTTP 处理，JMeter、API、数据库共享本机 CPU。因此“少查数据库”和“接口明显更快”是两个不同结论。

上述解释是结合环境与代码的判断，本次没有做 CPU/分配等逐项归因，也没有额外制造慢数据库延迟来放大缓存收益。缓存对慢查询、远程数据库、更大工作集或更高压力下的收益仍需另测。

## 三轮数据

| 轮次 | 模式 | 正式请求 | 吞吐 请求/s | P95 | P99 | 失败 |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| 1 | MySQL | 29,293 | 1951.44 | 13ms | 16ms | 0 |
| 1 | Redis | 28,401 | 1891.51 | 14ms | 18ms | 0 |
| 2 | Redis | 30,638 | 2041.17 | 12ms | 17ms | 0 |
| 2 | MySQL | 30,881 | 2056.81 | 12ms | 15ms | 0 |
| 3 | MySQL | 30,797 | 2051.90 | 12ms | 15ms | 0 |
| 3 | Redis | 32,545 | 2167.93 | 11ms | 15ms | 0 |

第一轮和第二轮 Redis 吞吐略低，第三轮略高，方向并不稳定。所有轮次与慢样本都保留，不只报告第三轮的有利数据。

## 方法与口径

- JMeter 5.6.3，OpenJDK 21，JVM 堆 256～512 MiB；安装包来自 Apache 官方下载并核对 SHA512。
- Intel Core i7-8700、WSL2；MySQL 8.0.46、buffer pool 128 MiB。Java/JMeter、API、Redis、MySQL 共享本机资源。
- API 使用正常 JWT 鉴权和画像 handler。画像 GET 不写审计日志。两组仍采用相同的 API 配置，仅切换画像存储适配器；不发广告决策/曝光，不启动 Kafka 负载。
- 每组重新启动 API/JVM；先通过 HTTP 读取全部 1000 个画像。JMX 内的独立预热线程组再执行每线程 20 次请求，之后进入正式线程组。
- CSV Data Set 循环读取同一批用户 ID；Header Manager 添加令牌；断言状态码 200、响应中的 userId 与请求一致且 score=88。
- 20 线程、无思考时间、HTTP keep-alive，属于固定并发的闭环模型，不是固定到达率，也不是系统最大容量证明。
- 分位数从全部正式 JTL elapsed 样本使用 nearest-rank 重算，毫秒精度；吞吐 = 正式请求数 / 各组实际采样跨度之和。没有平均三个 P95，也没有将组间重启/预热间隔计入吞吐。
- JTL 中保留预热与正式记录，统计时按 `GET profile` 标签筛选，`warmup` 不进入正式延迟。

## 回源计数的交叉验证

最初小规模预检发现：`events_statements_summary_by_digest.COUNT_STAR` 的增量略小于 JMeter 请求数。后续没有把这个差异忽略掉，而是加入第二条证据：

```sql
SELECT SUM(COUNT_FETCH)
FROM performance_schema.table_io_waits_summary_by_table
WHERE OBJECT_SCHEMA = <本次隔离库>
  AND OBJECT_NAME = 'user_profiles';
```

三组 MySQL 的 COUNT_FETCH 增量分别为 29,693、31,281、31,197，与每组 JTL 总请求数（含预热）精确一致；对应 digest COUNT_STAR 分别为 29,626、31,226、31,139。digest_lost 检查为 0，本次未进一步定位这一小幅欠计的内部原因，不能把猜测写成已确认的 MySQL 缺陷。

三组 Redis 的用户表 COUNT_FETCH 和源 SELECT digest 增量均为 0；Prometheus 缓存 hit 分别为 28,801、31,038、32,945，与各组 JTL 总数一致。正式过程没有 miss/fill。由此确认本次热缓存消除了测量阶段的回源，不能推广为整个系统永远没有回源。

## 自己复跑

PowerShell 中：

```powershell
cd D:\AdFlow
.\tests\load\run-jmeter-profile.ps1
```

可调整：

```powershell
.\tests\load\run-jmeter-profile.ps1 -Threads 20 -Seconds 15 -Rounds 3
```

脚本自动准备隔离数据库、API、独占 Redis，运行真实 JMeter，核对结果后清理，输出目录在 `work/jmeter/run-时间/`。需本项目 WSL Ubuntu 的 MySQL、redis-server 与 Java。

- [JMX 测试计划](../tests/load/profile-query.jmx)：可以在 JMeter GUI 打开查看线程组和断言。单独点击运行前仍需配置 port、users 和临时 token，最方便的执行方式是上述管理脚本。
- [运行方式与边界](../tests/bench/jmeter-profile/README.md)
- [逐组环境、计数与结果](verification/jmeter-profile/results.json)
- [独立复核及合并结果](verification/jmeter-profile/verification.json)

六份原始 JTL 以 `.jtl.gz` 保存在 `docs/verification/jmeter-profile/`，解压后可导入 JMeter，也可直接用复核脚本读取：

```sh
python tests/bench/jmeter-profile/verify.py docs/verification/jmeter-profile
```

复核器校验源码摘要、逐条成功状态、缓存/用户表读取计数，并重算分位数和吞吐。测量完成后只给清理逻辑补了“必须成功创建本库才可删除”的保护，没有改变业务代码或负载；当次被测 runner 源码另存 `tested-runner.go.txt`，复核器按其原始摘要核验，不改写旧结果。

标准 HTML 报告另在本机 `work/jmeter/full-01/`，仅供查看对应单轮图表，不能与三轮合并表混用。本次自动 HTML 分位数与该轮全量 JTL 重算有差异，未将其作为量化证据；本文数值统一以完整 JTL 的 nearest-rank 重算和 verification.json 为准。

## 结果解读

本实验使用 JMeter 对用户画像 GET 接口做了 MySQL 与 Redis 热缓存对照，固定 1000 个用户、20 线程，每组 15 秒，交替跑三轮。除了 HTTP 200，还断言返回的用户 ID 和字段。合并后两组 P95 都是 13ms，吞吐都约 2000 请求/秒，没有明显提速；但直接查询组累计读取用户表 92,171 次，Redis 组 92,784 次请求全部命中缓存，用户表读取增量为 0。因此本次验证的收益是减少数据库负载，而不是夸大接口提速。

没有测冷缓存、击穿、失效、故障降级或远程数据库场景，不将它们写作此次结果。隔离 API、Redis 和数据库均已清理，原有业务 API 健康检查通过。
