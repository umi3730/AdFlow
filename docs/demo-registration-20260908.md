# Demo 注册与默认登录

后续调整：Demo 默认已改为[免登录进入管理员工作台](direct-demo-20260908.md)。下文保留可选认证模式的注册实现说明；注册现为默认关闭，需显式开启。

按本项目 Demo 的使用方式，**新注册账号统一拥有管理员权限**。登录页默认填入公开演示账号 `admin / adflow-admin`，点击登录即可使用；切换注册时清空表单，注册成功后直接进入工作台。退出登录后恢复默认填充值。

不再自动绕过登录页，确保用户可以看到注册入口。旧 `NEXT_PUBLIC_ADFLOW_DEMO_AUTO_LOGIN` 不再控制该页面。密码和 token 不写入浏览器存储，只有公开的演示密码作为表单默认值。

## 后端与存储

- `GET /v1/auth/options` 提供注册开关、默认注册角色、是否可预填演示账号；不返回真实环境密码。
- `POST /v1/auth/register` 验证用户名和密码，保存 bcrypt 哈希并返回 JWT，使用正常认证与权限校验。
- 用户名 3–32 位字母、数字、下划线或短横线，以字母/数字开头，去除两端空格并统一小写。同名和大小写变体均返回 409。
- 密码至少 8 个 Unicode 码点、最多 72 个 UTF-8 字节，按 bcrypt 默认成本 10 哈希。确认密码由前端校验。
- 注册接口最多同时处理 4 次哈希/创建请求；满载返回 429 和 Retry-After=1，数据库操作共用 3 秒请求期限。
- 注册角色由服务端固定为 admin。预置 admin/operator/viewer 或 `ADFLOW_AUTH_USERS` 中配置的账号仍然优先，不能被注册覆盖。

新增迁移 **000013_auth_users**。MySQL 模式下新账号保存到 `auth_users`，用户名有唯一索引；不同 API 实例和重启后的服务都能登录。内存模式使用加锁用户表，支持注册，但进程重启后清空注册账号。

`ADFLOW_AUTH_STORE` 可设为 `memory` 或 `mysql`；未指定时，若计划、决策或审计已使用 MySQL，则认证也选择 MySQL，否则使用内存。

`ADFLOW_REGISTRATION_ENABLED` 默认关闭，可以显式设置。该开关开启时沿用本 Demo 的管理员注册策略。登录/注册界面用于 `ADFLOW_AUTH_ENABLED=true` 的环境；认证关闭时直接使用工作台。

只有 local/test 且没有自定义 `ADFLOW_AUTH_USERS` 时才预填演示账号，避免拿公开的默认密码替代自定义账号。实际新用户密码不回填到页面。

## 验证

- 注册、登录、bcrypt 保存、输入校验、种子账号保护、大小写冲突、并发同名注册回归通过。
- HTTP 测试确认注册返回 201，新账号能通过管理员发布权限检查；关闭注册返回 403。
- 真实 MySQL 测试覆盖独立连接/服务重新登录，以及两个服务并发注册同名账号，只创建一个账号。
- 浏览器验证了默认预填、空注册表单、确认密码拦截、注册后自动登录、跨实例登录、管理员审计接口访问、409 冲突、退出后恢复默认值和再次登录。
- Go 全量回归、vet、120 个前端测试、lint 和前端构建通过。本地 18080 / 18081 已更新，迁移 000013 已应用。

验证使用临时注册账号，完成后清理，不删除用户自行注册的账号。其他正在进行的性能改动予以保留，不把注册功能测试当作那些改动的性能证明。

证据：[浏览器记录](verification/auth-registration/browser.json)、[MySQL 回归](verification/auth-registration/mysql-integration.log)、[登录页截图](verification/auth-registration/login.png)、[注册页截图](verification/auth-registration/register.png)。
