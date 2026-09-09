# 测试补跑记录 · 2026-09-09

针对工程审查中未完成的竞态检测、真实依赖集成测试，以及前端合并后的整体验证进行补跑。结果针对本次未提交工作区，不是远程 CI 或生产环境验收，也不形成新的性能结论。

## 最终结果

| 检查 | 结果 |
| --- | --- |
| `scripts/verify.ps1` | Go 格式、vet、正式包测试，前端完整 lint 与 125 项回归全部通过 |
| 前端 `format --check` | 142 个匹配文件通过 |
| Go race detector | WSL Linux、Go 1.27.0、CGO_ENABLED=1，47 个有测试的包通过，退出码 0，未报告数据竞争 |
| MySQL / Redis / Kafka 集成 | 15 个顶层用例通过，0 跳过；包括并发预算/频控、缓存代次、Outbox 租约、消息重投和消费者重启、结算归还/接管 |
| 报表真实 SQL | 1 个用例通过，0 跳过；北京时间分桶、筛选、区间边界、成交快照及缺失凭据 |
| integration 标签静态检查 | 对集成包及报表 SQL 包执行 vet，通过 |
| 报表 HTTP smoke | 独立内存 API 上通过：3 次曝光、2 次点击、1 次转化，21 分消耗、300 分转化价值；重复事件、CSV、诊断证据及不存在请求检查通过 |
| 原运行环境 | 18080/18081 健康接口、3001 前端均返回 200；临时 18082 API 已停止 |

Go 竞态检测范围是 `./cmd/... ./internal/... ./tests/...`，使用 `-race -count=1 -timeout=5m`，不包含 `work/` 临时实验，也没有启用 integration 标签。真实基础设施用例另外执行，不能把两者合称为“真实 Kafka 链路已跑 race”。前端类型检查和生产构建已在此前最后一次明细改动后通过，本次没有改前端。

## 首轮失败与处理

首轮真实依赖检查有两处失败，原始记录保存在 `first-attempt/`，没有覆盖：

1. **背压用例的隔离库命名要求。** 用例要求 `adflow_backpressure_it_` 前缀，首轮创建的是 `adflow_check_`。调整临时运行器的建库前缀后使用全新隔离库重跑，保留原来的保护检查，没有放宽用例。
2. **旧缓存测试假设写操作直接回填缓存。** 原用例写入后立即删除源数据，再要求缓存命中；当前实现是 Cache-Aside：写入后失效、读取时回填。测试改为先读回填，更新后验证不会读到旧值，再删除独立夹具源行验证 Redis 命中。业务缓存代码未修改。复跑通过。

首次进度消息曾将失败误指向 Kafka；读取完整日志后已更正。实际两项 Kafka 恢复用例首轮和复跑均通过。

## 环境与隔离

- WSL 原来没有 C 编译器，本次安装 GCC 13.3、libc6-dev 和 Go 引导工具，由 Go 自动获取 1.27.0 Linux 工具链后执行 race。Windows 默认的 `CGO_ENABLED=0` 没有改变；PowerShell 的 `-Race` 入口仍需要 Windows 编译器，当前可在 WSL 运行。
- MySQL 通过本机 WSL root socket 创建专用随机后缀数据库。首次和复跑数据库都已删除，并再次查询确认不存在。
- 每轮使用独立 Redis 进程与三个随机 Kafka Topic；只删除归属于本轮 Topic 的新消费组。两轮临时进程、Topic 和消费组均清理成功。
- 报表 HTTP smoke 使用独立 18082 端口、内存存储和 mock 模型，没有向当前演示库生成数据，也未调用付费模型。进程完成后接收中断并记录正常关闭。
- 未重跑 10/20 空闲连接参数对照或最大 QPS 搜索；它们属于另行控制条件的性能实验，本次补的是功能、集成和竞态检查。

## 证据

[完整工程检查](verification/test-completion/verify.log) · [竞态检测](verification/test-completion/race.log) · [真实依赖及清理](verification/test-completion/integration-result.json) · [HTTP 联调](verification/test-completion/report-http-smoke.log) · [服务检查](verification/test-completion/services.json)

复现基础检查：`./scripts/verify.ps1`。WSL 竞态命令为 `CGO_ENABLED=1 go test -race -count=1 -timeout=5m ./cmd/... ./internal/... ./tests/...`；本轮使用已有 Windows 模块缓存加速构建，完整命令和运行器保留在证据目录。真实集成测试需先准备符合用例要求的隔离资源，不能将 `ADFLOW_IT_MYSQL_DSN` 指向演示库。
