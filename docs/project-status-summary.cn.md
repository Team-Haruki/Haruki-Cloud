# Haruki-Cloud 项目进展总结

> 最后更新：2026-03-23（v8.0）

## 📊 项目概览

**Haruki-Cloud** 是 HarukiBot 的核心后端服务，当前正在进行**多个独立服务的统一合并**工作。

## 🎯 合并项目状态一览

| 项目 | 原用途 | 状态 | 备注 |
|------|--------|------|------|
| **Service-Test** | 渲染服务（Part2） | ✅ 已完成 | 已合并进 `internal/pjsk/render` |
| **Test_Request_Construction** | Service-Test 的别名/镜像 | ✅ 已覆盖 | 实际就是 Service-Test |
| **Test_Instruction_Parser** | 指令解析（Part1） | ✅ 已完成 | 已合并进 `internal/pjsk/parser` + `handler` + `chardata` |

## ✅ Service-Test 合并（已完成）

**完成时间**：2026-03-21  
**详细文档**：`docs/service-test-merge-status.cn.md`

### 已完成内容

1. **渲染子系统完全迁移**（12 个模块）：
   - ✅ card, music, gacha, event
   - ✅ education, honor, profile, stamp
   - ✅ misc, score, deck, sk, mysekai

2. **统一路由体系**：
   - ✅ `POST /internal/pjsk/render` - 统一分发入口
   - ✅ `POST /internal/pjsk/<module>/<action>/build|render` - 模块化路由

3. **测试覆盖**：全部通过

### 临时实现（技术债）

1. **本地用户快照** - 仍使用 `user.json`、`music_metas.json`、`mysekai.json`
2. **MySekai masterdata** - 依赖本地文件，未完全转为 DB 驱动
3. **Deck 引擎** - 简化版实现，未迁入原生 CGo 引擎

## ✅ Test_Instruction_Parser 合并（已完成）

**完成时间**：2026-03-23

### 已完成内容

1. **Parser 核心迁移**（`internal/pjsk/parser/`，8 个文件）：
   - ✅ `GlobalCommandResolver` — 正则路由表，指令→模块+模式
   - ✅ `Extractor` — 角色昵称/稀有度/属性/技能/区服/年份 提取器
   - ✅ `CardParser` / `MusicParser` / `EventParser` — 类型化查询解析
   - ✅ `CommandParser` — SK/bind 命令解析
   - ✅ 测试全部通过

2. **Handler 子系统迁移**（`internal/pjsk/handler/`）：
   - ✅ Trie 树指令分发器（`handler.go`）
   - ✅ `SekaiCommandHandler` — 反射自动注册（`sekai/handler.go`）
   - ✅ 14 个功能模块 handler（card, event, chart, music, deck, gacha, education, entertainment, misc, mysekai, profile, score, sk, stamp, vlive）

3. **区域类型统一**：
   - ✅ `sekai_region.SekaiRegion` → `render/region.Value` 全量替换
   - ✅ 不再有重复的区域类型定义

4. **Chardata 加载器**（`internal/pjsk/chardata/loader.go`）：
   - ✅ 从数据库加载角色昵称映射
   - ✅ 后台定时刷新

5. **Bridge 调用桥**（`internal/pjsk/handler/bridge.go`）：
   - ✅ `Execute(ctx, resolved, app)` — ResolvedCommand 直接路由到 render Controller
   - ✅ 覆盖全部 12 个模块的所有模式
   - ✅ 零 HTTP 开销，纯 Go 函数调用

6. **配置接入**：
   - ✅ `config.PJSKParserConfig` 添加（chardata_region, refresh_interval）
   - ✅ `cmd/server/main.go` 中初始化 chardata + resolver
   - ✅ `haruki-db-configs.example.yaml` 更新

### 调用链（已实现，含 API 端点）

```
Bot → POST /internal/pjsk/command
    → parser.GlobalCommandResolver.Resolve(req.Command)
    → parser.ResolvedCommand{Module, Mode, Query, Region, Params}
    → handler.Execute(ctx, resolved, renderApp)
    → render Controller 直接调用
    → Drawing API → PNG (Content-Type: image/png)
```

## ✅ Bot 指令 API 端点（已完成）

**完成时间**：2026-03-23

### 设计范围说明

本端点仅处理 **Render 类指令**（返回 PNG 图片的查询指令），例如：
- 查卡、查活动、查卡池、谱面、sk线、我的世界资源等

**不在此端点的操作**（账号管理类指令）：
- `/绑定`、`/解绑`、`/设置主账号`、`/交换绑定`、`/注册` 等
- 这些是数据写操作，无图片输出，**应当作为独立 REST 端点实现**（待账号管理逻辑完成后添加）
- `GlobalCommandResolver` 中刻意不包含这些指令

### 鉴权说明

`/api/v2/bot/:botId/` 下的所有端点使用 **`VerifyBotSession` 中间件**（`api/bot_session_middleware.go`）。

| 请求头 | 说明 |
|--------|------|
| `X-Haruki-Bot-Id` | Bot 的数字 ID（须与 URL `:botId` 一致） |
| `X-Haruki-Bot-Session-Token` | POST `/bot/:bot_id/auth` 下发的 JWT session token |

验证流程：
```
1. 检查两个请求头存在且 X-Haruki-Bot-Id == URL :botId（不一致 → 403）
2. 验证 JWT 签名有效且未过期（失败 → 401）
3. 验证 JWT bot_id claim == X-Haruki-Bot-Id（不一致 → 403）
4. 在 Redis (hdb:bot:session:<botId>) 中查找 session token 是否存在且一致（不存在/不一致 → 401）
```

### 通用指令端点

**`POST /internal/pjsk/command`** — 接受原始文本指令，内部自动解析+路由，适合内部服务场景。使用 `VerifyAPIAuthorization` 中间件。

### 按功能分项端点

路径格式：**`GET|POST /api/v2/bot/:botId/pjsk/<module>/<mode>`**

Bot 工作流：
```
1. GET /api/v2/bot/:botId/command/manifests  →  TODO 占位端点（Manifest 结构待定）
2. Bot 本地预匹配指令到对应端点
3. GET /api/v2/bot/11451419/pjsk/card/detail?command=<Base64 OneBot JSON>
   或  POST /api/v2/bot/11451419/pjsk/card/detail  body: {"command": "<Base64>"}
4. Cloud 解码 Base64 → 提取 OneBot 消息文本 → 解析 → 验证端点匹配 → 渲染 → PNG
```

**command 参数**：Base64 编码的 OneBot v11 JSON payload（支持 `raw_message` 和 `message` 数组格式）。  
如果不是有效的 Base64+OneBot JSON，自动降级为纯文本指令处理。

端点若收到不属于该功能的指令，返回 400 并说明期望的 module/mode。

### 请求格式

GET：query params `command`、`server`、`im_platform`、`im_user_id`  
POST：JSON body `{"command":"<base64>", "server":"jp", "im_platform":"qq", "im_user_id":"12345"}`

成功响应：`200 OK`，`Content-Type: image/png`，body 为 PNG 字节流。

### 实现文件

| 文件 | 内容 |
|------|------|
| `api/bot_session_middleware.go` | `VerifyBotSession(redisClient)`、header 常量定义 |
| `api/legacy/pjsk/command_struct.go` | `CommandRequest`、`CommandErrorResponse` |
| `api/legacy/pjsk/command.go` | `RegisterPJSKCommandRoute`（通用端点） |
| `api/legacy/pjsk/command_test.go` | 5 个测试 |
| `api/bot/pjsk/struct.go` | `BotCommandRequest`、`BotCommandErrorResponse`、`ManifestEntry`、`ManifestResponse` |
| `api/bot/pjsk/route_table.go` | 41 条路由配置表（module、mode、path、prefixes） |
| `api/bot/pjsk/handler.go` | `makeBotHandler`、Base64+OneBot 解码、`RegisterPJSKBotRoutes`、manifest 占位 |
| `api/bot/pjsk/handler_test.go` | 11 个测试（含 OneBot 解码、纯文本降级、GET/POST、端点匹配、manifest） |
| `api/bot/pjsk/testhelpers_test.go` | 测试辅助：`testRenderApp`、`testResolver`、`renderEnvelope` |
| `cmd/server/main.go` | 同时注册两套路由，传入 `redisClient` |

> 注：`RegisterPJSKBotRoutes` 传入 `nil` redisClient 时跳过鉴权（用于单元测试）。

## ✅ Command Manifest 系统（已完成）

**完成时间**：2026-03-23

### 设计目标

Bot 客户端启动时下载完整 Manifest，本地预匹配用户输入的指令前缀到对应端点，避免每次指令都向服务端查询路由。

### 数据库表：`command_manifests`（bot DB）

| 字段 | 类型 | 说明 |
|------|------|------|
| `command_prefixes` | JSON `[]string` | 触发前缀列表，如 `["/查卡","/card"]` |
| `command_priority` | int (default 0) | 匹配优先级，越大越优先 |
| `command_mode` | string(16) | 请求方法，如 `"GET,POST"` |
| `command_module` | string(64) | 功能模块，如 `"pjsk"` |
| `command_path` | string(256) | 路径（无前导斜杠），如 `"card/detail"` |
| `command_additional_params` | JSON `[]string`（可空） | 额外可接受的参数名 |

唯一索引：`(command_module, command_path)`

### 启动时自动 Seed

首次启动若表为空，`SeedCommandManifests` 自动从 `botModeTable` 填充 41 条默认记录（priority=0，mode="GET,POST"）。后续启动不覆盖已有数据，手动调整的优先级得以保留。

### 实现文件

| 文件 | 内容 |
|------|------|
| `ent/bot/schema/commandmanifest.go` | Ent Schema 定义 |
| `database/bot/commandmanifest*.go` | 自动生成的 CRUD 代码 |
| `api/bot/pjsk/seed.go` | `SeedCommandManifests` — 首次空表时填充默认数据 |
| `api/bot/pjsk/struct.go` | `ManifestEntry`、`ManifestResponse` 结构体 |
| `api/bot/pjsk/handler.go` | `buildManifestHandler` — DB 查询按 priority DESC 返回 |

### API 端点

`GET /api/v2/bot/:botId/command/manifests`

- 需要 `VerifyBotSession` 鉴权
- 返回 `{"status":"ok","data":{"entries":[...]}}`，entries 按 `command_priority` 降序排列
- 无 botDB 配置时返回 501（单元测试模式）

## ✅ API 层结构整理（已完成）

**完成时间**：2026-03-23

### 变更内容

1. **公开 API 路径统一迁移至 `/api/v2/public/` 前缀**：
   - `/pjsk/alias/*` → `/api/v2/public/pjsk/alias/*`
   - `/chunithm/*` → `/api/v2/public/chunithm/*`

2. **删除 Censor 模块**：`api/censor/` 全部删除（`RegisterCensorRoutes` 已是空函数），`initCensor`/`initUsers` 从 `main.go` 移除。

3. **`api/` 目录按 public/bot/legacy 三层重新组织**：

   | 旧路径 | 新路径 | 说明 |
   |--------|--------|------|
   | `api/pjsk/alias_*.go` | `api/public/pjsk/` | 公开别名查询 |
   | `api/chunithm/` | `api/public/chunithm/` | 公开曲目查询 |
   | `api/bot/` | `api/bot/auth/` | Bot 注册/登录（package 改为 `auth`） |
   | `api/pjsk/bot_*.go` | `api/bot/pjsk/` | Bot 指令端点 |
   | `api/pjsk/render_*.go + command*.go` | `api/legacy/pjsk/` | 内部渲染路由（待发布前删除） |

4. **删除 `api/types.go`**：仅有类型重新导出，无存活代码引用。
5. **删除 `api/users/` 空目录**。

### 最终 `api/` 结构

```
api/
  struct.go / helper.go / bot_session_middleware.go   (package api — 共享层)
  public/
    pjsk/        → GET /api/v2/public/pjsk/alias/*         (无鉴权)
    chunithm/    → GET /api/v2/public/chunithm/*           (无鉴权)
  bot/
    auth/        → /bot/send-mail, /bot/register, /bot/:id/auth
    pjsk/        → /api/v2/bot/:botId/pjsk/*              (VerifyBotSession)
  legacy/
    pjsk/        → /internal/pjsk/*                       (VerifyAPIAuthorization，待删除)
```

## ✅ 外部 API 客户端整合（已完成）

**完成时间**：2026-03-23

将原来分散的三个外部 API 客户端包合并为统一的 `utils/sekai/` 包。

### 合并范围

| 原包 | 说明 | 新位置 |
|------|------|--------|
| `utils/sekaiapi/` | Sekai World API 客户端 | `utils/sekai/client_api.go` |
| `utils/tracker/` | 活动 Tracker 客户端 | `utils/sekai/client_tracker.go` |
| `utils/toolbox/` | Haruki Toolbox 客户端 | `utils/sekai/client_toolbox.go` |

三个原包目录均已删除。

### 实现文件（`utils/sekai/`）

| 文件 | 内容 |
|------|------|
| `enums.go` | `SekaiServerRegion`、`SekaiEventType`、`SekaiWorldBloomType`、`SekaiEventStatus`、`SekaiUnit` 等枚举及档线常量 |
| `errors.go` | 所有 sentinel error、自定义 error 结构体（含 toolbox 的 6 种细分错误）和 `parseMessage` 辅助函数 |
| `client_api.go` | `SekaiAPIClient`，单例，`GetSekaiAPIClient()` 获取 |
| `client_tracker.go` | `TrackerClient`，单例，`GetTrackerClient()` 获取 |
| `client_toolbox.go` | `HarukiToolboxClient`，单例，`GetToolboxClient()` 获取 |
| `types_profile.go` | `GetUserProfileResponse` 及 31 个嵌套类型（从 C# dump.cs 完整反向工程得出） |
| `types_system.go` | `GetSystemResponse`、`AppVersionInfo` |
| `types_info.go` | `GetInformationResponse`、`InformationEntry` |
| `types_tracker.go` | Tracker 响应类型（含多处类型修正） |
| `types_ranking.go` | 游戏排行榜类型（`RankingUserCard/Profile/ProfileHonor` 等，解决命名冲突） |

### Toolbox 错误细分

`GetPrivateData` 针对常见错误场景返回明确类型：

| HTTP 状态 | Message | 错误类型 |
|-----------|---------|---------|
| 404 | `account binding not found` | `ErrAccountBindingNotFound` |
| 404 | `game data not found` | `ErrGameDataNotFound` |
| 403 | `forbidden: invalid platform or platform_user_id` | `ErrInvalidPlatformUser` |
| 403 | `forbidden: account owner is banned` | `ErrAccountOwnerBanned` |
| 4xx/5xx 其他 | — | `ErrToolboxAPIError`（含状态码+消息） |

### Tracker 类型修正

原 `utils/tracker/` 中存在多处类型错误，合并时一并修复：

- `RankDataPoint.UserID` / `UserEventData.UserID`：`int64` → `string`
- `RankingUserData`：新增 `CheerfulTeamID *int` 字段
- `ScoreGrowthPoint.ScoreEarlier` → `*int`，`TimestampEarlier` → `*int64`（均为可选）
- `ScoreGrowthPoint` 新增 `TimeDiff *int64`、`Growth *int`
- 新增 `WorldBloomRankDataPoint`（嵌入 `RankDataPoint` + `CharacterID *int`）
- 新增 `WorldBloomLatestRankingResponse`、`WorldBloomTraceRankingResponse`

---

## ✅ Go 1.26 语法迁移（已完成）

**完成时间**：2026-03-23

将 `utils/sekai/` 中的 `errors.As` 替换为 Go 1.26 引入的类型安全泛型函数 `errors.AsType[T]`：

```go
// 旧写法
var netErr net.Error
if errors.As(err, &netErr) { ... }

// 新写法
if _, ok := errors.AsType[net.Error](err); ok { ... }
```

涉及三个客户端文件的重试逻辑（`client_api.go`、`client_tracker.go`、`client_toolbox.go`）。  
`database/*/ent.go` 为自动生成文件（`DO NOT EDIT`），不做改动。

---

## ✅ Logger 全局对接（已完成）

**完成时间**：2026-03-23

从另一项目引入 `utils/logger/` 包，替换原有的 `log/slog` 用法，统一日志接口。

### Logger API

| 函数 | 说明 |
|------|------|
| `NewLogger(name, level, writer)` | 创建独立 logger，显式指定 level 和 writer |
| `NewLoggerFromGlobal(name)` | 创建 logger，每次调用时动态读取全局 level/writer（支持运行时重配置） |
| `SetGlobalLogLevel(level)` | 设置全局 log level（启动时调用一次） |
| `SetGlobalFileWriter(writer)` | 设置全局文件 writer（启动时调用一次） |
| `OpenLogFile(path)` | 创建目录并以 `O_APPEND\|O_CREATE\|O_WRONLY` 打开文件 |
| `NewMultiWriter(writers...)` | 多路输出，自动过滤 nil（替代 `io.MultiWriter`） |

### 对接变更

**`cmd/server/main.go`**：
- `setupLogging()`：`os.OpenFile` → `harukiLogger.OpenLogFile`，`io.MultiWriter` → `harukiLogger.NewMultiWriter`
- 新增 `harukiLogger.SetGlobalLogLevel` + `SetGlobalFileWriter`，全局 logger 自动继承
- access log 文件同样换用 `OpenLogFile`
- `slog.Default()` → `harukiLogger.NewLoggerFromGlobal("Chardata")`
- 移除 `"log/slog"` import

**`internal/pjsk/chardata/loader.go`**：
- `*slog.Logger` → `*logger.Logger`
- `l.logger.Info("...", "key", val)` → `l.logger.Infof("... key=%v", val)`（结构化 kv 改 printf 风格）
- `l.logger.Warn(...)` → `l.logger.Warnf(...)`

**`utils/censor/censor.go`**：
- `NewLogger("...", "INFO", nil)` → `NewLoggerFromGlobal("...")`，自动跟随全局 log level

---

## ✅ Toolbox 数据类型枚举与封装（已完成）

**完成时间**：2026-03-23

`utils/sekai/enums.go` 新增 `ToolboxDataType` 枚举，明确两种私有数据类型：

```go
ToolboxDataTypeSuite   ToolboxDataType = "suite"    // 用户游戏快照 → SnapshotStore
ToolboxDataTypeMySekai ToolboxDataType = "mysekai"  // MySekai 世界快照 → MySekaiStore
```

`utils/sekai/client_toolbox.go` 新增两个语义化封装函数：

| 函数 | 说明 | 最终存储目标 |
|------|------|------------|
| `GetSuiteData(server, userID, platform, platformUserID)` | 拉取用户游戏快照（替代 user.json） | `pjsk_user_snapshots.main_snapshot_json` |
| `GetMySekaiData(server, userID, platform, platformUserID)` | 拉取 MySekai 世界快照（替代 mysekai.json） | `pjsk_user_snapshots.mysekai_snapshot_json` |

`GetPrivateData` 参数 `dataType` 从 `string` 改为 `ToolboxDataType`（编译期类型安全）。

---

## ✅ Music Meta 全局缓存（已完成）

**完成时间**：2026-03-23

新增 `internal/pjsk/meta/` 包，负责从公开远端（`sekai-data.3-3.dev`）按区服拉取并缓存 music_metas 数据，替代本地 `music_metas.json` 文件路径。

### 数据来源说明

`music_metas.json` 是**全局区服级**数据（不含任何用户状态），与 suite/mysekai 快照完全无关：

| 区服 | URL |
|------|-----|
| jp | `sekai-data.3-3.dev/music_metas.json` |
| en | `sekai-data.3-3.dev/music_metas-en.json` |
| tw | `sekai-data.3-3.dev/music_metas-tc.json`（注意 tc 后缀） |
| kr | `sekai-data.3-3.dev/music_metas-kr.json` |
| cn | `sekai-data.3-3.dev/music_metas-cn.json` |

### 实现文件（`internal/pjsk/meta/`）

| 文件 | 内容 |
|------|------|
| `urls.go` | 区服 → 远端 URL 映射，`Regions()` 返回稳定顺序 |
| `omakase.go` | `InjectOmakase(data []byte) []byte`（从 `userdata/local.go` 迁出导出） |
| `loader.go` | `Loader` 结构体，ETag + Last-Modified 条件请求，并发拉取，后台刷新 |

### Loader 核心行为

- **条件 GET**：携带 `If-None-Match` + `If-Modified-Since`
  - **304 Not Modified** → 保留缓存，零 body 传输
  - **200 OK** → `InjectOmakase` 处理后更新缓存，保存新 ETag/Last-Modified
- **并发拉取**：`LoadAll(ctx)` 对所有区服并发发起请求，单区服失败不阻断其他
- **后台刷新**：`StartBackgroundRefresh(ctx, interval)`，默认间隔 30 分钟（与 TS 原版一致）
- **安全读取**：`Get(region)` 返回缓存的独立副本，RWMutex 保护

### 集成情况

- `internal/pjsk/render/userdata/local.go` — 删除私有 `injectOmakaseMusicMeta` + helpers，改调 `meta.InjectOmakase`
- `internal/pjsk/render/app/app.go` — `Config` + `App` 新增 `MetaLoader *meta.Loader` 字段
- `config/config.go` — 新增 `MusicMetaConfig { RefreshInterval }` 及 `PJSKRenderConfig.MusicMeta`
- `cmd/server/main.go` — `initPJSKRenderIfEnabled` 中初始化 loader、首次 `LoadAll`、启动后台刷新并注入 `renderapp.Config`
- `haruki-db-configs.example.yaml` — 新增 `pjsk_render.music_meta.refresh_interval` 配置示例

---

## 📋 后续待办事项

### P1 - 重要但不紧急

1. **账号管理 REST 端点**（bind/unbind/setMain 等）
   - `sekai/profile.go` 中相关逻辑全部是 TODO stub，需先完成业务逻辑
   - 完成后作为独立 REST 端点暴露（如 `POST /internal/pjsk/binding/bind`），不走 render 指令链路

2. **正式用户快照 Provider**（技术债偿还）
   - 设计已完成：`docs/pjsk-user-snapshot-provider-design.cn.md`
   - 数据来源已确认：suite → `GetSuiteData()`，mysekai → `GetMySekaiData()`，music meta → `meta.Loader`
   - 当前仍依赖本地 `user.json`、`mysekai.json` 文件（`music_metas.json` 已由远端 loader 接管）
   - 待实现：`snapshot-schema` → `snapshot-store` → `identity-resolver` → `binding-resolver` → `snapshot-provider` → `app-wire` → `snapshot-write-api`

3. **Bot 调用方切换**
   - Haruki-ZeroBot 改调 `POST /internal/pjsk/command`
   - 详见：`docs/zerobot-render-followup.cn.md`

### P2 - 可选优化

4. **MySekai 完全 DB 驱动**（当前依赖本地 masterdata 文件）
5. **Deck 引擎收口**（如需原生 CGo 引擎）

## 📈 项目进度总览

```
总体进度: ████████████████████ 99%

Service-Test 合并:       ████████████████████ 100% ✅
Test_Request_Constr:     ████████████████████ 100% ✅
Parser 合并:             ████████████████████ 100% ✅
Bot API 层:              ████████████████████ 100% ✅  (通用端点 + 41个功能端点)
Command Manifest:        ████████████████████ 100% ✅  (DB表 + Seed + 端点)
外部API客户端整合:       ████████████████████ 100% ✅  (utils/sekai 统一包 + ToolboxDataType)
Go 1.26 语法迁移:        ████████████████████ 100% ✅  (errors.AsType)
Logger 对接:             ████████████████████ 100% ✅  (utils/logger 全局接入)
Music Meta 缓存:         ████████████████████ 100% ✅  (internal/pjsk/meta, ETag/30min刷新)
用户快照 Provider:       ░░░░░░░░░░░░░░░░░░░░   0% 📝  (8个todo待实现)
Bot 切换:                ░░░░░░░░░░░░░░░░░░░░   0% 📝
```

### 当前阶段小结（2026-03-23）

四大核心子系统（渲染 + 解析 + API 端点 + Command Manifest）全部完成。  
外部 API 客户端三包合并为 `utils/sekai/`，含完整类型定义、Toolbox 错误细分、`ToolboxDataType` 枚举及 `GetSuiteData`/`GetMySekaiData` 封装函数。  
Go 1.26 `errors.AsType[T]` 迁移完成，`utils/logger` 全局对接完成。  
`music_metas.json` 由全新的 `internal/pjsk/meta.Loader` 接管，支持 ETag/Last-Modified 条件请求和 30 分钟后台刷新。  
剩余工作：用户快照正式 Provider（suite + mysekai 数据的 DB 存储与读取，共 8 个 todo）和 ZeroBot 侧接入（外部仓库）。

## 📚 相关文档

### 已完成合并
- `docs/service-test-merge-plan.cn.md` - Service-Test 合并方案
- `docs/service-test-merge-status.cn.md` - Service-Test 合并状态

### 外部 API 客户端
- `external/dump.cs` - C# 类型 dump（用于 GetUserProfileResponse 反向工程）
- `external/sekaiapi/` - 原始 Sekai API 类型文件（参考用）
- `external/eventtracker/` - 原始 Tracker 类型文件（参考用）

### 后续事项
- `docs/pjsk-user-snapshot-provider-design.cn.md` - 用户快照 Provider 设计
- `docs/zerobot-render-followup.cn.md` - ZeroBot 接入说明

## 🔌 API 端点一览

### 鉴权方式说明

| 中间件 | 适用场景 | 验证内容 |
|--------|----------|----------|
| `VerifyAPIAuthorization` | 内部服务间调用 | `Authorization` 头 + `User-Agent` 头（匹配配置值） |
| `VerifyBotSession` | Bot 客户端直调 | `X-Haruki-Bot-Id` + `X-Haruki-Bot-Session-Token`（JWT + Redis） |
| 无鉴权 | 公开端点 | — |

### Bot 会话端点（公开）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/bot/send-mail` | 发送验证码 |
| POST | `/bot/register` | 注册 Bot |
| POST | `/bot/:bot_id/auth` | 登录，返回 `session_token` |

### Bot 指令端点（Render 类，`VerifyBotSession` 鉴权）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/internal/pjsk/command` | 通用端点：纯文本指令 → 解析 → 渲染 → PNG |
| GET | `/api/v2/bot/:botId/command/manifests` | **TODO 占位** — 返回指令前缀→端点映射表 |
| GET/POST | `/api/v2/bot/:botId/pjsk/card/detail` | 卡面详情 |
| GET/POST | `/api/v2/bot/:botId/pjsk/card/list` | 查卡列表 |
| GET/POST | `/api/v2/bot/:botId/pjsk/card/box` | 查箱/查框 |
| GET/POST | `/api/v2/bot/:botId/pjsk/gacha` | 卡池信息 |
| GET/POST | `/api/v2/bot/:botId/pjsk/music` | 歌曲详情 |
| GET/POST | `/api/v2/bot/:botId/pjsk/music/list` | 歌曲列表 |
| GET/POST | `/api/v2/bot/:botId/pjsk/music/chart` | 谱面预览 |
| GET/POST | `/api/v2/bot/:botId/pjsk/music/progress` | 打歌进度 |
| GET/POST | `/api/v2/bot/:botId/pjsk/music/rewards` | 曲目奖励 |
| GET/POST | `/api/v2/bot/:botId/pjsk/deck/event` | 活动组卡 |
| GET/POST | `/api/v2/bot/:botId/pjsk/deck/challenge` | 挑战赛组卡 |
| GET/POST | `/api/v2/bot/:botId/pjsk/deck/no-event` | 长草最强组卡 |
| GET/POST | `/api/v2/bot/:botId/pjsk/deck/bonus` | 加成组卡 |
| GET/POST | `/api/v2/bot/:botId/pjsk/deck/mysekai` | 烤森组卡 |
| GET/POST | `/api/v2/bot/:botId/pjsk/event` | 活动详情 |
| GET/POST | `/api/v2/bot/:botId/pjsk/event/list` | 活动列表 |
| GET/POST | `/api/v2/bot/:botId/pjsk/education/challenge` | 挑战赛信息 |
| GET/POST | `/api/v2/bot/:botId/pjsk/education/power` | 角色加成 |
| GET/POST | `/api/v2/bot/:botId/pjsk/education/area` | 区域道具 |
| GET/POST | `/api/v2/bot/:botId/pjsk/education/bonds` | 羁绊等级 |
| GET/POST | `/api/v2/bot/:botId/pjsk/education/leader` | 加成统计 |
| GET/POST | `/api/v2/bot/:botId/pjsk/score` | 分数查询 |
| GET/POST | `/api/v2/bot/:botId/pjsk/score/custom-room` | 自定义房间分数 |
| GET/POST | `/api/v2/bot/:botId/pjsk/score/music-meta` | 曲目 meta |
| GET/POST | `/api/v2/bot/:botId/pjsk/score/music-board` | 曲目榜 |
| GET/POST | `/api/v2/bot/:botId/pjsk/stamp` | 贴纸列表 |
| GET/POST | `/api/v2/bot/:botId/pjsk/misc/birthday` | 角色生日贺图 |
| GET/POST | `/api/v2/bot/:botId/pjsk/sk/line` | SK 榜线 |
| GET/POST | `/api/v2/bot/:botId/pjsk/sk/query` | SK 查分 |
| GET/POST | `/api/v2/bot/:botId/pjsk/sk/check-room` | 查房 |
| GET/POST | `/api/v2/bot/:botId/pjsk/sk/speed` | SK 时速 |
| GET/POST | `/api/v2/bot/:botId/pjsk/sk/player-trace` | 玩家轨迹 |
| GET/POST | `/api/v2/bot/:botId/pjsk/sk/rank-trace` | 档线轨迹 |
| GET/POST | `/api/v2/bot/:botId/pjsk/sk/winrate` | 胜率预测 |
| GET/POST | `/api/v2/bot/:botId/pjsk/mysekai/resource` | 烤森资源 |
| GET/POST | `/api/v2/bot/:botId/pjsk/mysekai/fixture-list` | 家具列表 |
| GET/POST | `/api/v2/bot/:botId/pjsk/mysekai/fixture-detail` | 家具详情 |
| GET/POST | `/api/v2/bot/:botId/pjsk/mysekai/door-upgrade` | 大门升级 |
| GET/POST | `/api/v2/bot/:botId/pjsk/mysekai/music-record` | 音乐唱片 |
| GET/POST | `/api/v2/bot/:botId/pjsk/mysekai/talk-list` | 对话列表 |
| GET/POST | `/api/v2/bot/:botId/pjsk/profile` | 个人名片 |

> `command` 参数为 Base64 编码的 OneBot JSON payload，支持 `raw_message` 和 `message[]` 格式。  
> 账号管理类指令（bind/unbind 等）为 TODO，完成后将作为独立 REST 端点添加。

### 渲染分发端点（`VerifyAPIAuthorization` 鉴权）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/internal/pjsk/render` | 统一分发入口（指定 target + operation + payload） |

### 模块化渲染端点（`VerifyAPIAuthorization` 鉴权）

每个模块提供 `build`（返回 JSON payload）和 `render`（返回 PNG）两种操作。

| 模块 | 路径前缀 | 操作 |
|------|---------|------|
| Card | `/internal/pjsk/card/` | `detail`, `list`, `box` |
| Deck | `/internal/pjsk/deck/` | `recommend`, `recommend/auto` |
| Event | `/internal/pjsk/event/` | `detail`, `list`, `record` |
| Gacha | `/internal/pjsk/gacha/` | `detail`, `list` |
| Honor | `/internal/pjsk/honor/` | （直接） |
| Profile | `/internal/pjsk/profile/` | （直接） |
| Misc | `/internal/pjsk/misc/` | `chara-birthday` |
| MySekai | `/internal/pjsk/mysekai/` | `resource`, `fixture-list`, `fixture-detail`, `door-upgrade`, `music-record`, `talk-list` |
| Music | `/internal/pjsk/music/` | `detail`, `brief-list`, `list`, `progress`, `rewards/detail`, `rewards/basic`, `chart` |
| Education | `/internal/pjsk/education/` | `challenge-live`, `power-bonus`, `area-item`, `bonds`, `leader-count` |
| SK | `/internal/pjsk/sk/` | `line`, `query`, `check-room`, `speed`, `player-trace`, `rank-trace`, `winrate` |
| Score | `/internal/pjsk/score/` | `control`, `custom-room`, `music-meta`, `music-board` |
| Stamp | `/internal/pjsk/stamp/` | `list` |

### 其他端点

| 路径前缀 | 鉴权 | 说明 |
|---------|------|------|
| `/api/v2/public/pjsk/` | 无 | PJSK 别名查询（公开） |
| `/api/v2/public/chunithm/` | 无 | Chunithm 别名 + 曲目查询（公开） |
| `/internal/bot/` | `VerifyAPIAuthorization` | Bot 会话验证（内部服务） |

### 待实现端点

| 路径 | 说明 |
|------|------|
| `POST /internal/pjsk/binding/bind` 等 | 账号管理（bind/unbind/setMain），需先完成业务逻辑 |

---

**维护者**：Haruki-Cloud Team  
**文档版本**：v8.0  
**创建日期**：2026-03-23
