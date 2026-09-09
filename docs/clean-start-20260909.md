# 干净克隆启动验证 · 2026-09-09

## 环境与方法

从公开 GitHub 仓库重新克隆到独立目录，起始提交为 `3b88ba206937add503ade7fa41d2332dd6a2d303`。克隆中没有本机 `.env`、已有 `node_modules` 或被忽略的 `work/` 内容；重新执行 `go mod download` 与 `npm ci`。工具自身的下载缓存允许复用，没有复制旧安装目录。

默认运行内存适配器和 mock 模型，不使用已有演示数据库。为保留原服务，API 改用 18380，并通过 `NEXT_PUBLIC_ADFLOW_API_URL` 指向该端口；前端使用 README 支持的 3000 端口。

## 发现并修复的启动缺陷

初次运行能初始化内存演示数据，但 `/readyz` 返回 503：入口无论配置何种适配器，都会创建并检查 MySQL/Redis。未使用的本机 MySQL 不可用也导致内存模式无法通过健康检查。

修复为根据配置中的实际适配器初始化客户端、健康检查及数据库连接池监控。MySQL 用于计划、决策、画像、身份、审计或 Kafka 事件持久化时启用；Redis 用于预占、画像缓存、共享限流或 Kafka 模式时启用。内存模式的 pprof 可在没有 SQL 连接池时启动。

在克隆中只应用本轮 `cmd/api/main.go`、`internal/config/dependencies.go`、`internal/config/dependencies_test.go` 三个源文件修复后重启；修复文件的 SHA256 与主工作区一致，其他业务代码及依赖清单未改变。初次失败与修复后记录分别保留，不将首次失败计为成功。

## 验证结果

| 检查 | 结果 |
| --- | --- |
| Go 模块下载、全新 `npm ci` | 完成；前端按锁文件安装 553 个包 |
| 默认内存 `/readyz` | 修复后 HTTP 200，`ready: true`，依赖列表为空 |
| 必需依赖失效 | 选择 mysql-redis 画像、将 MySQL/Redis 指向不可用端口，关闭演示初始化，仅验证就绪检查；返回 HTTP 503，两项均为 down |
| 默认界面 | 直接进入 admin 工作台，4 个内置计划，初始计量为 0 |
| 内置竞价用例 | 3 轮全部命中风铃互动，成交 ¥0.05；12 次业务 HTTP 调用，曝光/点击/转化各 3 次 |
| 报表 | 消耗 ¥0.15、转化价值 ¥15，与三轮业务一致；这是功能验收，不是容量压测 |
| 前端 | 独立安装后的 125 项回归及生产构建通过 |
| Go 回归 | 主工作区完整 verify 通过；配置、健康、HTTP、演示初始化和 profiling 五个相关包的 race 检查通过 |

最初尝试的前端 3300 端口不在开发 CORS 允许列表内，浏览器连接被阻止；改用已有允许的 3000 端口后成功，未扩展 CORS 权限。

## 依赖告警（初次安装记录）

后续已完成依赖升级并将完整 audit 降为 0，见[维护记录](dependency-maintenance-20260909.md)。下文保留初次安装时的结果。

全新安装触发的 `npm audit` 报告 11 个受影响依赖条目：8 high、2 moderate、1 low，0 critical。涉及 `react-server-dom-webpack`、Vinext/Vite、Cloudflare 开发工具链及传递依赖等；这不是 11 个独立漏洞的精确计数，也没有据此证明项目中每条攻击路径均可利用。

安装还提示 esbuild、sharp、workerd 的安装脚本未被本机 allow-scripts 规则明确批准；本次没有绕过该规则，实际启动与构建仍通过。没有执行 `npm audit fix --force` 或混入框架升级。依赖告警已经列入 [Roadmap](roadmap.md)，需单独核对和回归；功能测试通过不代表安全审计通过。

## 证据

[修复前就绪检查](verification/clean-start/readiness-before.json) · [修复后就绪检查](verification/clean-start/readiness-after.json) · [必需依赖失效](verification/clean-start/required-dependencies.json) · [报表核对](verification/clean-start/report-summary.json) · [依赖审计原始结果](verification/clean-start/npm-audit.json)

历史技术测量、失败记录与适用范围保留，本轮没有新增性能结论。
