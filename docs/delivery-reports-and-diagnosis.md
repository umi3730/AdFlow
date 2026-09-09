# 投放效果报表与 Agent 投放诊断

## 已实现

- 控制台新增「效果报表」，支持计划筛选、北京时间日期范围、小时/天聚合、趋势指标切换、明细分页与 CSV 导出。
- 展示曝光、点击、转化、已确认消耗、转化价值、CTR、CVR、平均点击成本和平均转化成本。
- 「Agent 助手」提供规则生成与投放诊断两个页签；报表的「Agent 分析」携带当前筛选条件进入诊断。
- 诊断读取服务端报表、当前计划状态/预算/频控/素材，填写请求 ID 后读取已保存决策和结算状态。建议关联实际证据，保留 provider/model/fallback 标记。

## 数据实现

独立 reporting 上下文按需聚合已去重的事件事实，复用现有 `processed_events` 与 `event_settlements`，不增加决策或消费链路的统计写入。MySQL 通过一条查询完成按时间聚合；内存模式读取已完成事件及 `ImpressionSubmission` 中的成交快照。

消耗只计算已确认的曝光成交价。数据库读取 `decision_snapshot.Pricing.priceFen`，不使用当前计划出价，也不将转化价值当作消耗。历史没有确认凭据的曝光数量单独返回 `unpricedImpressions`。预算预占尚未结算时不计入消耗。

使用事件 `occurred_at`，时间区间为 `[from,to)`。页面结束日期包含当天，通过下一个北京时间零点作为 `to`。按小时/天补齐空桶；总 CTR/CVR 由总计数计算，不平均各时段比率。最大查询窗口31天，查询期限3秒，CSV输出同一筛选下的全量时段（UTF-8 BOM）。

迁移 `000014_delivery_reports` 增加时间和计划/时间索引，兼容 MySQL 5.7。已有事实可直接查询历史数据，无需数据回填。未来数据规模增长时可在 Reader 接口后替换为物化聚合表。

## Agent 技术栈

Go + Gin，使用标准库 `net/http` 调用已配置的模型服务，支持 Responses 与 Chat Completions 两种协议。诊断使用独立 JSON 输出结构，复用现有 HTTP 超时/重试、熔断与本地降级机制。不是 Python 服务，无需 LangChain 或额外模型框架。

模型输入由服务端生成，客户端只提供查询条件、问题与可选请求 ID；不接收客户端伪造报表。发送聚合指标和精简检查结果，不发送用户画像、身份标识、结算 token、原始日志或密钥。模型返回的建议必须引用已存在的证据 ID，结果仅供查看，不自动修改或发布计划。

## 接口

- `GET /v1/reports/delivery?from=...&to=...&granularity=hour|day&campaignId=...`
- `GET /v1/reports/delivery/export`：同查询参数。
- `POST /v1/agent/delivery-diagnoses`：`from/to/granularity/campaignId/question/requestId`。

报表及导出允许 viewer；诊断需要 operator，记录独立审计动作 `DIAGNOSE_DELIVERY`。主模型不可用时按既有配置使用本地诊断或返回错误。

## 验证入口

```powershell
go test ./internal/reporting/... ./internal/agentassistant/... ./internal/event/adapter/memory ./internal/identity/application
go test -tags=integration -run TestRealReportQueryTimeBucketsAndSettlementSnapshot -v ./internal/reporting/adapter/mysql
node tests/delivery-report-smoke.mjs
```

真实SQL测试从 `ADFLOW_REPORT_IT_DSN` 读取连接，只在独占连接上建立临时表；覆盖北京时间分桶、筛选、区间上界、成交快照JSON路径和待核对/缺失凭据。HTTP smoke 固定使用独立18082端口，创建测试计划并验证完整投放、重复事件、报表、导出和诊断。

本轮通过 Go 全量回归与 vet、前端122项回归及类型检查、生产构建。真实 MySQL 临时表测试通过；独立 API 的完整 HTTP 联调验证了3次曝光、2次点击、1次转化、21分消耗与300分转化价值，重复回传不增加计数，小时/天汇总一致，CSV导出及附带请求证据的诊断通过。两种远程模型协议使用本地HTTP桩验证结构、证据引用和异常结果；未调用付费模型。
