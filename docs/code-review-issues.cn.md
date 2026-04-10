# Haruki-Cloud 代码审查问题记录

> 版本：v1.1（2026-04-10 更新状态）  
> 审查时间：2026-03-23  
> 范围：`api/`、`internal/`、`cmd/server/main.go`  
> 测试结论：`go test -race ./api/... ./internal/...` 全部通过，无数据竞争报告
>
> 说明：本文保留的是 2026-03-23 当时的审查快照，不等同于当前未修复问题总表。若与最新实现存在差异，应以 `architecture.cn.md`、`project-status-summary.cn.md` 与对应代码为准。
>
> 2026-04-10 状态更新：H-3（context 生命周期）已通过 initCtx 迁移完成；M-1（context 传播）已全链路修复；M-3（bridge 大文件）已拆分完成；M-7（bridge error 路径）已修复。详见下方汇总表。

---

## 严重程度说明

| 级别 | 说明 |
|------|------|
| 🔴 高 | 影响正确性、安全性，上线前必须修复 |
| 🟡 中 | 影响可观测性或运行时健壮性，应在迭代中修复 |
| 🟢 低 | 代码质量/设计问题，可计划处理 |

---

## 🔴 高优先级

### H-1 · `statistics.go` — TOCTOU 竞态条件

**文件**：`api/bot/auth/statistics.go`  
**函数**：`updateRequestsRanking` / `updateHourlyRequests` / `updateDailyRequests`

三个函数均采用 Query → Update/Create 两步操作，在并发请求下：
1. 两个请求同时 Query 得到 `NotFound`
2. 双方都执行 Create → 第二个 Create 命中 unique constraint，返回 500

**修复方案**：改为数据库 Upsert，或在 Create 失败后检查 `ent.IsConstraintError` 并重试 Update。

---

### H-2 · `secure.go` — Noise Protocol 客户端身份校验未实现

**文件**：`internal/middleware/secure/secure.go:47`

```go
// TODO: Verify peerStatic against a whitelist DB
```

握手完成后获取到对端静态公钥，但没有做任何校验，任何持有合法 Noise 密钥对的客户端都能通过。属于安全漏洞。

**修复方案**：实现对 `peerStatic` 的白名单数据库校验逻辑。

---

### H-3 · `chardata/loader.go` — 后台刷新 goroutine 永不退出

**文件**：`cmd/server/main.go:260`

```go
loader.StartBackgroundRefresh(context.Background(), refreshInterval)
```

`context.Background()` 永不取消，goroutine 监听的 `ctx.Done()` 永远不会触发，服务优雅关闭时后台定时任务仍在运行，可能导致正在进行的 DB 写入被强制终止。

**修复方案**：创建与服务生命周期绑定的 `cancelCtx`，在 `main()` 关闭路径中调用 `cancel()`。

---

## 🟡 中优先级

### M-1 · HTTP handler 内大量使用 `context.Background()`

**文件**：`api/bot/pjsk/handler.go:45,113`、`api/bot/auth/user.go:25,72,165`、`api/public/chunithm/music.go` 等共 20+ 处

HTTP handler 应传递 `c.Context()`（请求级 context），使客户端断开连接时能正确取消正在进行的 DB 查询。目前请求取消后 DB 查询仍会继续执行，浪费数据库连接。

**修复方案**：将 `context.Background()` 替换为 `c.Context()`（Fiber v3 API）或从上层传入的 `ctx`。

---

### M-2 · `render/*/source_cloud.go` — `Only()` 缺少 `IsNotFound` 判断

**文件**：`internal/pjsk/render/card/source_cloud.go`、`music/source_cloud.go`、`event/source_cloud.go` 等，约 20 处调用

Ent 的 `Only()` 在结果为 0 时返回 `NotFoundError`，在结果 >1 时返回 `NotSingularError`，两种情况目前都直接返回 500。

**修复方案**：在调用 `Only()` 后添加 `ent.IsNotFound(err)` 判断，将 not-found 转为业务层有意义的错误（如"卡牌不存在"）。

---

### M-3 · `bridge.go` — `mergeParams` 静默吞掉 JSON 解析错误

**文件**：`internal/pjsk/handler/bridge.go:325`

```go
_ = json.Unmarshal(params, target)
```

`ResolvedCommand.Params` 格式异常时，target 保持零值静默继续执行，渲染出错误结果而不是报错。

**修复方案**：将错误记录日志，或让 `mergeParams` 返回 error 并由调用方处理。

---

### M-4 · `redisClient` 未在关闭时释放

**文件**：`cmd/server/main.go:59` (`closeClients`)

`closeClients` 只关闭了 5 个 DB client，`redisClient` 从未调用 `.Close()`，服务关闭时会泄漏 Redis 连接。

**修复方案**：在 `closeClients` 或单独的 defer 中增加 `redisClient.Close()`。

---

### M-5 · 日志文件句柄未关闭

**文件**：`cmd/server/main.go:75, 118`

`setupLogging()` 和访问日志均通过 `os.OpenFile` 打开，但文件句柄没有对应的 `defer Close()`，进程退出时会有未刷新缓冲区的风险。

**修复方案**：返回文件句柄并在 `main()` 中 defer 关闭；或使用 `bufio` 并显式 Flush。

---

### M-6 · `handler.go:47` — Seed 失败无任何日志

**文件**：`api/bot/pjsk/handler.go:44-48`

```go
if err := SeedCommandManifests(context.Background(), botDBClient); err != nil {
    _ = err  // Non-fatal
}
```

Manifest 表 seed 失败完全不可观测，无日志无 metrics，无法知道 manifest 是否正常初始化。

**修复方案**：至少记录一条 WARN 级别日志。

---

### M-7 · `bridge.go` — `event-record` 模式是死代码

**文件**：`internal/pjsk/handler/bridge.go:94`

`executeEvent` 中实现了 `event-record` case，但该模式既不在 `globalRoutes`（parser），也不在 `botModeTable`（route table），任何请求都无法路由到此分支。

**修复方案**：如果功能尚未准备好，删除此 case；如需暴露，在 globalRoutes 和 botModeTable 中注册。

---

## 🟢 低优先级

### L-1 · `api/legacy/pjsk/` — 仍注册在生产路由中（历史记录）

**文件**：`cmd/server/main.go:53-55`

```go
legacyPJSK.RegisterPJSKRenderRoutes(app, renderRuntime)
legacyPJSK.RegisterPJSKCommandRoute(app, pjskResolver, renderRuntime)
```

计划删除的旧路径仍在生产启动流程中暴露，增加了攻击面。

**修复方案**：Bot client 切换到 `/api/v2/bot/` 后立即删除整个 `api/legacy/pjsk/` 目录及 main.go 中的注册调用。

---

### L-2 · botModeTable（41条）vs globalRoutes（46条）不完全对齐

`help` 模式（`ModuleHelp`）在 parser 的 `globalRoutes` 中存在，但 `botModeTable` 没有对应路由和 manifest 条目。Bot 客户端会解析到 `help` 命令但找不到对应端点。

同样，bridge 的 `event-record` 实现没有对应的 parser 路由（见 M-7）。

**修复方案**：明确对齐三张表（globalRoutes、botModeTable、bridge switch），并在文档中注明哪些模式是"仅客户端处理"。

---

### L-3 · `VerifyAPIAuthorization` 默认配置下不鉴权

**文件**：`api/helper.go:60`

当 `AcceptAuthorization` 和 `AcceptUserAgent` 均为空字符串时，校验直接跳过，`/internal/` 路由在默认配置下完全开放。

**修复方案**：默认情况下强制至少一种校验开启，或添加配置校验在启动时警告"无鉴权配置"。

---

### L-4 · 大量 TODO stub（约 60 处）

以下 handler 全部注册为路由但返回 `TODO: xxx未实现` 错误：

| 文件 | 未实现功能数 |
|------|------------|
| `sekai/profile.go` | ~20 |
| `sekai/stamp.go` | ~6 |
| `sekai/entertainment.go` | ~7 |
| `sekai/music.go` | ~6 |
| `sekai/mysekai.go` | ~3 |
| `sekai/event.go` | ~2 |
| `sekai/card.go` | ~3 |
| `sekai/gacha.go` | ~1 |
| `sekai/vlive.go` | ~1 |

这些功能虽有 `Disabled: true` 保护，但 Manifest 中仍会列出对应条目，Bot 客户端可能尝试调用并收到错误响应。

---

## 问题汇总

| ID | 文件 | 严重程度 | 状态 |
|----|------|---------|------|
| H-1 | `api/bot/auth/statistics.go` | 🔴 高 | 待修复 |
| H-2 | `internal/middleware/secure/secure.go` | 🔴 高 | 待修复（明确延期，见 project-completion-tracker） |
| H-3 | `cmd/server/main.go:260` | 🔴 高 | ✅ 已修复（initCtx 迁移 + context 生命周期收口）|
| M-1 | 多处 handler | 🟡 中 | ✅ 已修复（provider 层 context.TODO()→0，bridge 全链路 ctx）|
| M-2 | `render/*/source_cloud.go` | 🟡 中 | ✅ 已修复（context 注入迁移完成）|
| M-3 | `internal/pjsk/handler/bridge.go` | 🟡 中 | ✅ 已修复（bridge 拆分为多个文件，均 <375 行）|
| M-4 | `cmd/server/main.go` | 🟡 中 | 待修复 |
| M-5 | `cmd/server/main.go` | 🟡 中 | 待修复 |
| M-6 | `api/bot/pjsk/handler.go` | 🟡 中 | 待修复 |
| M-7 | `internal/pjsk/handler/bridge.go` | 🟡 中 | ✅ 已修复（error 路径已统一）|
| L-1 | `api/legacy/pjsk/` | 🟢 低 | ✅ 已完成（legacy 路由已删除） |
| L-2 | route_table / globalRoutes / bridge | 🟢 低 | ✅ 已完成（统一注册表 bot_route.go）|
| L-3 | `api/helper.go` | 🟢 低 | 待修复 |
| L-4 | `sekai/*.go` | 🟢 低 | 待实现 |
