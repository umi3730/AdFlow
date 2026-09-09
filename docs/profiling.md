# 按 API 实例采集 pprof

默认不启动 pprof。需要时通过 `ADFLOW_PPROF_ADDR` 显式启用，只允许 loopback 地址。每个 API 使用不同端口，例如：

```powershell
.\ops\native\start-api.ps1 -Port 18080 -PprofPort 6060
.\ops\native\start-api.ps1 -Port 18081 -PprofPort 6061
```

启动脚本要求目标 API 端口空闲，不会覆盖已有进程。pprof 端口绑定失败会明确报错，避免悄悄采到另一个实例。分析服务随 API 的 Context 关闭；采集期间重启可能使本次 profile 无效。

`GET /debug/instance` 返回 API 地址、PID、启动时间、GOMAXPROCS、block 采样配置和当前数据库连接池统计，不返回 DSN、环境变量或登录 token。

## 采集并生成文本摘要

```powershell
.\scripts\profile.ps1 -ProfileType cpu -Duration 30 `
  -BaseURL http://127.0.0.1:6061 -ExpectedApiPort 18081

.\scripts\profile.ps1 -ProfileType mem `
  -BaseURL http://127.0.0.1:6061 -ExpectedApiPort 18081
```

脚本先读取实例信息，再检查目标 API 端口。它不会自行启动 API，也不修改运行环境。每次采集生成独立文件：

- `.pprof`：Go 原始采样。
- `.top.txt`：`go tool pprof -top` 生成的文本摘要。
- `.json`：采集前后实例信息、文件路径与 SHA256。

交互查看可自行执行：

```powershell
go tool pprof -http=127.0.0.1:8080 work/profiles/实际文件.pprof
```

这会运行一个交互式 Web 服务，直到手动结束，不是静态 HTML 导出命令。脚本只打印该命令，不自动启动查看器，因此采集结束后会正常退出。

block 分析需额外启用 `ADFLOW_PPROF_BLOCK_RATE`。默认 0 不采样；设为 1 记录每次阻塞，存在运行开销。脚本发现 block 采样未启用时会拒绝生成误导性的空报告。

## 连接池参数

```env
ADFLOW_MYSQL_MAX_OPEN_CONNS=30
ADFLOW_MYSQL_MAX_IDLE_CONNS=10
ADFLOW_MYSQL_CONN_MAX_LIFETIME=3m
ADFLOW_MYSQL_CONN_MAX_IDLE_TIME=1m
```

最大连接数必须为正，空闲连接数不可大于最大连接数。调大单实例上限前，应考虑全部实例和其他客户端的总连接预算。

池统计中的 WaitCount / WaitDuration 是累计值；比较某次负载应计算结束值减开始值。它们包含后台 worker 的连接获取，不能把 WaitCount 除以 HTTP 请求数直接称为“用户请求等待率”。

对照入口：`scripts/compare-pools.ps1`。固定空闲连接数和连接寿命，只变更最大连接数，并按三种顺序重复测试。每个实验使用独立 MySQL schema、Redis 进程和 Kafka Topic，结束后自动清理。
