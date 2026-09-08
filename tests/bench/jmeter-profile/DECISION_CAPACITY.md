# 广告决策接口的稳定吞吐探索

复用本目录的隔离资源管理器，实际流量由 `tests/load/decision-capacity.js` 的 k6 固定到达率执行器产生。

## 运行

仓库根目录、独立 PowerShell：

```powershell
$env:GOOS = 'linux'
$env:GOARCH = 'amd64'
go build -o work/end-to-end/api-linux ./cmd/api
go build -o work/jmeter/profile-runner-linux ./tests/bench/jmeter-profile
wsl.exe -d Ubuntu -u root --cd /mnt/d/AdFlow --exec ./work/jmeter/profile-runner-linux -decision-capacity -output work/jmeter/decision-capacity-new
```

需要既有 WSL MySQL root socket、redis-server、已下载的 Linux k6 v2.2.0。输出目录须不存在。`-probe-seconds` 默认 20，`-confirm-seconds` 默认 60。

## 请求路径及边界

使用正常 `/v1/decisions`，包含 JWT、Redis 滑动窗口限流、256 名额的单实例决策准入、已保存画像的缓存读取、定向、一价竞价、频控/预算预占、MySQL 决策执行租约与结果持久化、正常日志和审计。

不调用临时用户模拟接口，不重复 requestId，不回传曝光，不测 Kafka、结算或统计。因此结果是**决策受理吞吐**，不是完整投放链路吞吐，也不宣称预算实际扣费。

1000 个真实已保存画像、同广告位 3 个广告主、相同定向，出价 2/3/5 分，必须返回最高出价计划。每个新请求有独立 ID，避免把幂等结果读取混入正常决策。

客户端校验 requestId、素材、获胜计划、一价模式、5 分价格、3 个广告主。响应本身没有 userId 字段，用户身份通过持久化决策记录与请求序号映射核对。

## 保持预算与频控有效

- 日预算 10,000,000 分，频控 100，均未关闭或改动后端实现。
- 预占期仍是后端 30 秒。用户轮询；测试上界 2000 QPS 时，每个用户在 30 秒内约 60 次预占，低于频控上限。
- 没有曝光，所以实际 spent 必须为 0；reserved 与预占金额 Hash 的和必须一致且不超过预算。
- Redis 的过期预占依赖正常后续预占请求惰性清理，因此阶段结束的预占快照可能仍含刚到期、尚未清理的条目，不将其当成实际扣费。
- 只在阶段之间清空本实验独占 Redis 并重新预热；测试中途不重置计数。不会清空业务 Redis。
- 每阶段先全量预热画像，再执行 3 次真实决策预热，均不计入 k6 正式耗时；核账包含这 3 次。

## 搜索与标准

阶梯 25/50/100/200/400/800/1600/2000 QPS，首次失败后以 25 QPS 分辨率缩小区间。候选速率需三轮各 60 秒通过，失败则回退 25 QPS，最多回退五级。没有通过或达到搜索上界时明确报告，不把测试边界冒充绝对上限。

预先定义：P95 < 300ms，HTTP 或业务错误率 < 0.1%，dropped_iterations=0，没有 No-Ad 或错误竞价结果，实际请求数不低于计划的 99%，持久化和预占核对通过。阈值不在看到结果后修改。

原始结果保存所有失败阶段、HTTP/业务计数、SQL 落库数、Redis 预占快照及每秒资源/在途决策/goroutine 采样。HTTP 异常时可能已产生持久化结果，分别记录客户端成功数与数据库结果，不能默认为异常请求从未提交。

API、k6、MySQL、Redis 共机，保留日志和原有连接池设置。它只证明当前环境、工作集及验收标准下的已验证档位，不能外推到生产容量。
