# 性能工具快速使用

默认不启动 pprof。若对应 API 端口已占用，应先确认实例，不要重复启动。

```powershell
.\ops\native\start-api.ps1 -Port 18081 -PprofPort 6061
.\scripts\profile.ps1 -ProfileType cpu -Duration 30 -BaseURL http://127.0.0.1:6061 -ExpectedApiPort 18081
.\scripts\profile.ps1 -ProfileType mem -BaseURL http://127.0.0.1:6061 -ExpectedApiPort 18081
```

脚本输出 `.pprof`、`.top.txt` 和 `.json`，不会启动 API 或伪造静态 HTML。交互查看单独运行：

```powershell
go tool pprof -http=127.0.0.1:8080 work/profiles/实际文件.pprof
```

block 采样默认关闭，需要启动 API 时显式设置 `ADFLOW_PPROF_BLOCK_RATE`。详情见 [pprof 指南](profiling.md)。

连接池对照实验：

```powershell
.\scripts\compare-pools.ps1 -Seconds 60
```

它使用独立 MySQL schema、Redis 和 Kafka 资源，依次跑九组测试。需要已有 WSL 原生组件和 Linux k6；不会读取业务 `.env`，不会往原业务数据库写测试数据。

目前默认连接池为 30/10。三档结果与选择理由见 [实验报告](mysql-pool-comparison-20260908.md)。不要把“最大连接数调大”直接写成 QPS 提升。

比较空闲上限用 `-Dimension idle`；只复制源码并验证编译用 `-PrepareOnly`。脚本从独立快照构建和运行，完整实验仍需要上述本地依赖。[10/20 实验记录](mysql-idle-pool-comparison-20260909.md) 保留了失败阶段、时钟处理和来源限制，默认配置暂未调整。
