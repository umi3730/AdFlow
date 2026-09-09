# 前端依赖安全维护 · 2026-09-09

本轮处理干净安装发现的依赖告警。升级在独立 Git 工作区完成，通过版本约束、锁文件、构建和真实页面验证后再更新常用环境。没有运行 `npm audit fix --force`、忽略 peer 依赖或关闭安装脚本保护。

## 审计结果

| 阶段 | npm audit 结果 |
| --- | --- |
| 原锁文件 | 11 个受影响包：8 high、2 moderate、1 low |
| 直接依赖及工具链升级后 | 4 high，均由 Miniflare 的 Sharp 依赖传递产生 |
| 定向更新 Sharp 后 | 0 项告警；完整依赖树没有 peer 冲突 |

这些是当日 npm advisory 数据库对依赖树的结果，不是独立漏洞数量，也不代表应用不存在其他安全问题。原始审计结果保留，不将初次安装冲突或中间告警隐藏为成功。

## 版本调整

| 包 | 原版本 | 当前版本 |
| --- | --- | --- |
| react / react-dom / react-server-dom-webpack | 19.2.6 | 19.2.8 |
| vinext | 1.0.0-beta.5 | 1.0.0-beta.9 |
| @vitejs/plugin-rsc | 0.5.26 | 0.5.34 |
| vite | 8.0.13 | 8.2.2 |
| @cloudflare/vite-plugin | 1.37.1 | 1.54.6 |
| wrangler | 4.92.0 | 4.130.0 |
| @cloudflare/workers-types | 4.20260515.1 | 5.20260908.1 |
| miniflare → sharp | 升级后的 Miniflare 原本仍依赖 0.35.2 | 0.35.4 |

React 的 RSC 拒绝服务问题在 19.2.8 修复，React DOM 和 RSC 的 peer 约束也要求配套版本。[官方公告](https://github.com/advisories/GHSA-wx67-qw84-cm4g)

Vite 的 Windows 路径问题影响 8.0.0–8.0.15，本次升级至 8.2.2。公告对受影响的暴露方式和文件条件有明确限定，本轮不将所有本机开发环境一概视为可被利用。[官方公告](https://github.com/advisories/GHSA-fx2h-pf6j-xcff)

首次安装发现 Wrangler 要求 Workers 类型包 5.20260908.1，因此一起更新；没有用 `--legacy-peer-deps` 绕过冲突。Vinext 升级后仍保留当前入口及 Vite 8 兼容范围，原有 React 组件与 Go API 协议未改。

最新 Miniflare `5.20260908.0-alpha` 固定依赖 Sharp 0.35.2，仍受 libheif 告警影响。采用仅针对该 Miniflare 版本的 override，把 Sharp 更新至官方修复版 0.35.4；没有回退到 npm 自动建议的旧 Wrangler/Cloudflare 插件。后续 Miniflare 自身采用修复版时应删除此 override。[Sharp 公告](https://github.com/advisories/GHSA-rgj7-g3m4-5g8c)

## 验证与兼容处理

- `npm install` 成功后再次使用 `npm ci` 验证锁文件可复现；保留完整 audit 输出及依赖树检查结果。
- 前端格式、lint、TypeScript 检查、126 项回归及生产构建通过。
- 新增 Sharp 原生回归，生成并解码一个 8×8 的 AVIF 测试图，验证像素尺寸及通道。Windows 实测使用 Sharp 0.35.4、libheif 1.23.2、libvips 8.18.6；CI 同样执行此测试以检查 Linux 原生包。
- 在独立内存 API 与升级后的前端运行三轮竞价：曝光/点击/转化各 3 次，消耗 ¥0.15，转化价值 ¥15，报表一致。19 张素材缩略图均加载成功，明细展开可用，检查时浏览器错误日志为空。
- 使用不含敏感内容的独立 `.env.security-probe` 文件验证开发服务器：普通路径、Windows ADS 路径分别返回 403，编码变体返回 404，均未返回测试标记；测试文件已删除。这是定向回归，不是完整渗透测试。
- Vite 配置的 JSON 导入补充 `with { type: 'json' }`，适配原生配置加载器；没有关闭告警来隐藏兼容问题。
- 新 Windows 检出中 `.mjs` 会被转换为 CRLF，导致格式门禁失败；新增 `.js`/`.mjs` 的 LF 属性。历史 `docs/verification/**` 仍按原始字节保留，没有修改实验快照。
- CI 新增 `npm audit --audit-level=high`，阻止 high/critical 告警进入主分支。当前完整 audit 为零，门禁阈值不改变这次完整审计结果。

安装脚本沿用本机 npm 的 allow-scripts 规则，没有新增授权或放开脚本执行。升级不包含 Go 业务改动、数据库迁移或新的性能压测。

## 证据

[升级前审计](verification/dependency-maintenance/audit-before.json) · [中间审计](verification/dependency-maintenance/audit-intermediate.json) · [升级后审计](verification/dependency-maintenance/audit-after.json) · [文件访问回归](verification/dependency-maintenance/fs-deny.json) · [Sharp 运行检查](verification/dependency-maintenance/sharp-runtime.json)
