# 公开记录脱敏说明

2026-09-08 发布检查发现 k6 的 `setup_data.token` 包含本地演示账号的短期 JWT。公开的 `k6-summary.json` 已将该字段替换为 `[REDACTED_LOCAL_DEMO_TOKEN]`；请求数量、延迟、检查结果和其他测试字段没有改动。

当前 `k6-walkthrough.js` 的汇总导出已经排除 `setup_data`，并补充回归，后续复跑不会把 setup 中的认证信息保存到汇总。原测试版本保存在 `tested-k6-walkthrough.js.txt`，便于按原始源码摘要复查。不要把真实运行 token 放进仓库。
