# AdFlow k6 压测实操与面试讲法

## 本次真正测了什么

2026-09-08 使用 **k6 v2.2.0 linux/amd64** 发出真实 HTTP 流量。Go 程序只负责隔离环境和测后核账，未代替 k6 发正式负载。

API、k6、MySQL 均在本机 WSL；Redis 独占临时实例，Kafka 使用独立三分区 Topic。启用真实 JWT、MySQL 审计、Redis 画像缓存/限流/预算频控和 Kafka；新库应用全部迁移，无百万历史背景数据。

3 家广告主对同一人群出价 2/3/5 分，200 个已保存用户均满足定向，画像先经 HTTP 预热。每轮必须命中 5 分计划，随后回传曝光、点击、转化；每次转化价值固定 500 分。业务失败或 No-Ad 不能当成一次完整成功。

这是一轮入门负载演练，**不是最大 QPS、长稳压测或索引优化 A/B**。不能与之前使用百万历史数据或不同客户端的报告直接计算优化百分比。

## 你可以怎么复跑

在 PowerShell 中：

```powershell
cd D:\AdFlow
.\tests\load\run-k6-walkthrough.ps1
```

前提：本项目 WSL Ubuntu 中 MySQL、Kafka 已启动，可使用 root socket 认证，并有 redis-server。脚本使用固定版本 Linux k6，缺少时从官方 release 下载并核对 SHA256；不会安装全局包。它会编译当前 API/测试管理程序、准备独立数据库及中间件资源、执行压测、核账，再清理隔离资源，不使用业务 `.env`。

调整第二个负载阶段的压力/时长，例如：

```powershell
.\tests\load\run-k6-walkthrough.ps1 -Rate 40 -Seconds 30
```

这是后续可做的实验，本文没有把 40 轮/秒写成已测结果。每次使用新的输出目录，命令结束会打印路径，查看 `k6-console.log`、`k6-summary.json`、`proof.json`。

真正的 k6 脚本是 [tests/load/k6-walkthrough.js](../tests/load/k6-walkthrough.js)。管理程序会拼出下面这种命令（端口和输出目录为本次生成，清理后不能直接复用旧地址）：

```sh
k6 run --no-usage-report --no-color \
  -e BASE_URL=http://127.0.0.1:测试端口 \
  -e SECONDS=20 -e RATE=30 \
  -e REPORT_PATH=本次输出目录/k6-summary.json \
  tests/load/k6-walkthrough.js
```

## 看懂核心配置

```js
executor: 'constant-arrival-rate',
rate: 30,
timeUnit: '1s',
duration: '20s',
preAllocatedVUs: 30,
maxVUs: 60,
```

- `rate: 30`：每秒计划启动 30 **轮业务**。
- 一轮 = 决策 + 曝光 + 点击 + 转化，共 4 个串行 HTTP 请求，因此全成功时约 120 个业务 HTTP 请求/秒。
- `preAllocatedVUs` 是预分配执行资源；`maxVUs` 是资源上限。它们不是实际并发数，也不是请求速率。
- 当接口变慢，需要更多 VU 才能维持相同到达率；资源不足时 `dropped_iterations` 会增加。
- arrival-rate 执行器负责节奏，业务函数末尾没有额外 sleep。

三段配置：

| 阶段 | 目标业务速率 | 派发时长 | 目的 |
| --- | ---: | ---: | --- |
| smoke | 1 轮/秒 | 5s | 先确认登录、竞价与事件流程正常 |
| baseline | 10 轮/秒 | 20s | 低负载基线 |
| moderate | 30 轮/秒 | 20s | 观察增加压力后的延迟和错误 |

阶段之间有 2s 间隔，失败请求不自动重试掩盖错误。setup 中建数据、登录、预热不计入自定义业务耗时，但会进入 k6 内置 HTTP 总请求数。

## 本次结果

| 指标 | 10 轮/秒阶段 | 30 轮/秒阶段 |
| --- | ---: | ---: |
| 实际成功业务轮数 | 201 | 601 |
| 决策平均耗时 | 27.36ms | 37.22ms |
| 决策 P95 | 36.73ms | 57.28ms |
| 决策 P99 | 58.93ms | 72.04ms |
| 决策至三类事件均受理 P95 | 103ms | 156ms |

smoke 实际 6 轮，全场 **808 轮**、**2424 条事件**、**4040 个业务断言通过**；业务错误率 0%，未派发轮数 0，预先设置的所有阈值通过。实际轮数取 k6 Counter，不根据 `rate × duration` 硬编码核账；本次实际计数为 6/201/601。

**P95=57.28ms** 表示该阶段约 95% 的决策 HTTP 耗时不高于这个值。P99 更关注较慢的尾部，平均值会掩盖少量慢请求。

全场内置 `http_reqs=3642`：808×4=3232 个业务 HTTP 请求，加上 410 个登录/建数据/预热请求。内置全场平均约 68.90 HTTP 请求/秒，受到低速阶段、等待间隔和 setup 影响，不能拿它代替 30 轮/秒阶段的速率。

同理，带 phase 的 Counter 在 summary 中的 `rate` 仍可能按整场时长归一化；分析某阶段时看该阶段配置、计数和对应 Trend，不把这个字段直接当作稳态速率。

## 两个容易读错的字段

1. `business_accepted_ms=156ms` 只到 HTTP 受理。Kafka 消费尚可能未完成，不能叫“完整落库耗时”。
2. `business_errors` 是自定义 Rate，脚本失败时写入 true、成功时写入 false。JSON 中 `passes:0, fails:808, rate:0` 表示 **0 次错误、808 次非错误**；这里的 fails 是 Rate 布尔样本内部计数，不是 808 次业务失败。`checks` 的 passes/fails 才对应断言通过/失败。

## 阈值为何在运行前确定

本次脚本提前设置：

- 决策 P95 < 300ms、P99 < 1000ms。
- 一轮业务受理 P95 < 1000ms。
- 业务错误率为 0、dropped_iterations 为 0、断言全部通过。
- 每个阶段必须产生非零成功轮次，避免没有样本却看起来“错误率 0%”。

这些是本次练习的验收线，不是经过生产需求评审的 SLA。修改阈值要基于新的需求或测试目标，不能在跑失败后为通过而放宽。

## 测后核账

k6 结束后，Go 程序单独等待并检查持久化结果，本次核对：

- 808 条决策、808 个 SETTLED 曝光任务。
- 2424 条事件回执、2424 条 PUBLISHED Outbox、2424 条 processed_events。
- 曝光、点击、转化各 808；模拟转化价值 404000 分。
- 模拟预算支出 4040 分，即每次曝光 5 分；未结预占为 0。
- Kafka broker 的消费组输出中，3 个分区 CURRENT-OFFSET 等于 LOG-END-OFFSET，LAG 均为 0。

k6 结束后约 1.01s 查到全部处理完成；这是**全场结束后的等待时间**，不是每个请求的异步 P95。每个请求从发送至提交可见的分位数，应看此前独立的全链路实验。

## 面试时的说法

可以这样讲：

> 我把性能和业务正确性分开验证。使用 k6 的固定到达率模型，先做 1 轮/秒的冒烟检查，再以 10 和 30 轮/秒各运行 20 秒。每轮包含一次广告决策和曝光、点击、转化回传，并断言最高出价广告命中。在本机真实 MySQL、Redis、Kafka 环境下，30 轮/秒阶段决策 P95 为 57.28ms，业务错误和 dropped iterations 都是 0。随后通过数据库和 Redis 核账，确认全场 2424 条事件处理完成且预算扣减一致。因为每个负载阶段只有 20 秒，这只能证明该测试负载下的表现，我没有把它称为生产容量上限。

面试追问：

- **为什么不用 30 个固定 VU？** 固定 VU 下接口变慢会使发送速率下降；这次目标是维持业务到达率，所以用 constant-arrival-rate。
- **为什么看 P95/P99？** 平均值看不出少量慢请求，需要同时观察尾延迟和错误率。
- **为什么没有错仍不能说明成功？** 请求可能根本没发出去，或者都返回未命中；因此检查样本数、dropped_iterations、业务断言，再核对异步落库。
- **30 轮/秒就是 QPS 30 吗？** 决策约 30 请求/秒，整轮有 4 个 HTTP 请求，不能混用口径。
- **下一步怎么做？** 分级加压并延长稳态时间，结合 CPU、连接池等待、Outbox 积压和 Kafka lag 找到拐点；固定数据和环境做多轮复测。

## 环境问题也如实记录

首次尝试使用 Windows 版 k6 访问 WSL 随机 loopback 端口，登录请求被拒绝，未产生有效业务样本。未将零样本统计写成成功。之后使用同版本的官方 Linux k6，在 WSL 内运行。

Linux 压缩包来自 Grafana 官方 v2.2.0 release，SHA256 与官方校验文件一致：`b5a8003c86f35f5cd5ceef1490312c48e587696c94d998cefc6d7b3b4cb1597d`。成功及失败输出均保留。

资料：

- [k6 汇总](verification/k6-walkthrough/k6-summary.json)
- [终端原始输出](verification/k6-walkthrough/k6-console.log)
- [核账、环境、源码摘要与清理记录](verification/k6-walkthrough/proof.json)
- [首次环境检查失败记录](verification/k6-walkthrough/initial-environment-failure.log)
- [官方固定到达率说明](https://grafana.com/docs/k6/latest/using-k6/scenarios/executors/constant-arrival-rate/)
- [官方阈值说明](https://grafana.com/docs/k6/latest/using-k6/thresholds/)

实验结束后所有隔离数据库、API、Redis、Kafka Topic/消费组已清理，原业务 API 健康检查通过。报告中的 API 临时端口已失效，复跑请使用上面的启动脚本。
