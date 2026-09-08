# 画像 GET 的本机容量探索

沿用本目录的隔离库、API、独占 Redis 与画像初始化，但使用 **k6 固定到达率**寻找本机可持续速率；不是把 JMeter 线程数直接当 QPS。

默认被测路径：JWT 鉴权的 `GET /v1/profiles/{userId}`，`mysql-redis`，1000 个已保存画像，全部先预热。保持生产 API 二进制、日志、缓存超时和连接池配置不变，不经过广告决策的 256 并发门控，也不涉及竞价或事件处理。

## 运行

从仓库根目录，在独立 PowerShell 中：

```powershell
$env:GOOS = 'linux'
$env:GOARCH = 'amd64'
go build -o work/end-to-end/api-linux ./cmd/api
go build -o work/jmeter/profile-runner-linux ./tests/bench/jmeter-profile
wsl.exe -d Ubuntu -u root --cd /mnt/d/AdFlow --exec ./work/jmeter/profile-runner-linux -capacity -output work/jmeter/capacity-new
```

要求既有 WSL MySQL、redis-server，以及前一轮 k6 演练下载的 `work/k6-lab/k6-v2.2.0-linux-amd64/k6`。输出目录必须不存在。准备说明参见 `tests/load/run-k6-walkthrough.ps1`；不要为取用 k6 而重复启动无关业务压测。

默认短探测 20s，持续确认每轮 60s，可用 `-probe-seconds`、`-confirm-seconds` 调整。脚本固定起始 500/1000/2000/4000/8000/16000 QPS；遇到首个失败后，以 500 QPS 的分辨率缩小区间，然后在候选速率上连续确认三轮。如长测失败，降低 500 QPS 重新确认，最多回退四级。结果不保证一定能找到通过的上限，失败与未找到都明确记录。

## 预先定义的通过标准

- HTTP P95 < 100ms。
- HTTP 或画像内容错误率 < 0.1%；每次都检查返回用户 ID 和 score=88。
- dropped_iterations = 0。
- 实际请求数达到计划数的至少 99%，有效响应占比至少 99.9%。
- 只有连续三轮较长确认均通过，才报告“本次验证的稳定目标速率”。

如果所有初始阶梯连 16000 QPS 都通过，只能报告达到测试上界的下界，不能宣称找到了真正最大值。如果约束主要来自发压端或共享机器，则只报告该测试环境的上限。

## 控制与观测

- 每轮仅清空**本脚本拥有的独立 Redis 进程**的数据，然后读取全部画像预热。不会对业务 Redis 执行清空。
- 1000 个用户轮询，保持相同工作集。k6 每轮预分配 256 VU，上限 512 VU；这个数不是 Go 服务端 goroutine 上限。
- 每秒采样 `/proc` 中 API、k6、Redis、MySQL 的 CPU/RSS/线程数及整机 CPU；CPU=100% 表示占用一个逻辑核心，整机百分比另行按所有核心归一化。
- 每秒抓取一次 API `/metrics`，记录 `go_goroutines` 与 `GOMAXPROCS`。它是额外观察流量，不进入 k6 的 QPS。
- 记录用户表 COUNT_FETCH 增量与缓存 hit/miss/fill 等计数；压力下若出现回源会如实保存，不能默认为所有请求一直命中缓存。
- 保存 k6 每轮原始汇总及控制台输出，异常轮次保留非零退出信息；不在一轮内重试失败请求。
- API 与发压端共享 WSL 主机，因此无法直接外推独立发压机或生产部署的最大容量。

所有创建的数据库、API、独占 Redis 在结束时清理，报告保留清理结果及源码/二进制摘要。真实业务库和已运行的 API 保持原样。
