# utils/query — 统一数据库查询客户端

> 最后更新：2026-03-23（v1.0）

---

## 1. 用途

`utils/query` 是一个**跨多个数据库的统一查询门面（Facade）**，将底层 4 个独立 Ent 客户端封装为一个单一 `Client`，供 API Handler 和集成测试使用。

**解决的问题：**
- API Handler 需要同时持有多个 DB 客户端，调用点分散、参数冗长
- 原始 Ent 查询不包含业务层输入校验，各处重复写参数检查
- 需要一套统一的哨兵错误（sentinel errors），以便 HTTP Handler 映射到标准 4xx 状态码
- 跨表联合查询（如批量曲目 + 难度联查）的逻辑需复用

---

## 2. 覆盖的数据库

| 字段 | DB 客户端 | 数据库 |
|------|-----------|--------|
| `chunithmMain` | `database/chunithm/maindb` | 用户绑定、别名、默认服务器 |
| `chunithmMusic` | `database/chunithm/music` | 曲目信息、难度、谱面数据 |
| `pjsk` | `database/pjsk` | PJSK 别名、群组别名、用户绑定、偏好设置 |
| `users` | `database/users` | 通用用户（平台 ID → Haruki ID） |

所有字段可以为 `nil`（对应服务未配置时），调用会返回 `ErrXxxNotConfigured`。

---

## 3. 构造方式

```go
qc := query.NewClient(
    chunithmMainClient,   // *chunithmMainDB.Client，可为 nil
    chunithmMusicClient,  // *chunithmMusicDB.Client，可为 nil
    pjskClient,           // *pjskDB.Client，可为 nil
    usersClient,          // *usersDB.Client，可为 nil
)
```

---

## 4. 哨兵错误

| 错误 | 触发条件 |
|------|----------|
| `ErrInvalidAlias` | 别名为空或超过最大长度（`utils.MaxAliasLength`） |
| `ErrInvalidAliasType` | `aliasType` 不是合法类型（调用 `utils.ParseAliasType`） |
| `ErrInvalidMusicID` | `musicID <= 0` |
| `ErrInvalidUserID` | `harukiUserID <= 0` 或 platform/groupID 为空 |
| `ErrAliasNotFound` | 查询无结果 |
| `ErrMusicNotFound` | 曲目不存在 |
| `ErrBindingNotFound` | 绑定/默认绑定/默认服务器不存在 |
| `ErrPreferenceNotFound` | 偏好设置不存在 |
| `ErrUserNotFound` | 用户不存在 |
| `ErrChunithmNotConfigured` | `chunithmMain` 或 `chunithmMusic` 为 `nil` |
| `ErrPJSKNotConfigured` | `pjsk` 为 `nil` |
| `ErrUsersNotConfigured` | `users` 为 `nil` |

HTTP Handler 可以通过 `errors.Is` 将这些错误映射到标准 HTTP 状态码（`ErrInvalidXxx` → 400，`ErrXxxNotFound` → 404，`ErrXxxNotConfigured` → 503）。

---

## 5. 方法一览

### 5.1 PJSK 查询（`pjsk.go`）

| 方法 | 说明 |
|------|------|
| `GetPJSKGlobalAliasToID(ctx, aliasType, alias)` | 全局别名 → ID 列表 |
| `GetPJSKGlobalAliasesByID(ctx, aliasType, id)` | ID → 全局别名列表 |
| `GetPJSKGroupAliasToID(ctx, platform, groupID, aliasType, alias)` | 群组别名 → ID 列表 |
| `GetPJSKGroupAliasesByID(ctx, platform, groupID, aliasType, id)` | ID → 群组别名列表 |
| `GetPJSKBindings(ctx, harukiUserID, server)` | 用户所有绑定（server 可为空，则返回所有服务器） |
| `GetPJSKDefaultBinding(ctx, harukiUserID, server)` | 用户默认绑定（server 默认为 "default"） |
| `GetPJSKPreferences(ctx, harukiUserID)` | 用户所有偏好设置 |
| `GetPJSKPreference(ctx, harukiUserID, option)` | 单条偏好设置 |

### 5.2 CHUNITHM 查询（`chunithm.go`）

| 方法 | 说明 |
|------|------|
| `GetChunithmMusicIDByAlias(ctx, alias)` | 别名 → 曲目 ID 列表 |
| `GetChunithmAliasesByMusicID(ctx, musicID)` | 曲目 ID → 别名列表 |
| `GetAllChunithmMusic(ctx)` | 全部已发布曲目（过滤 `release_date > now`） |
| `GetChunithmMusicBasicInfo(ctx, musicID)` | 单曲基本信息 |
| `GetChunithmMusicDifficultyInfo(ctx, musicID, version)` | 难度常数（version 为空则返回最新版本） |
| `GetChunithmChartData(ctx, musicID)` | 所有难度谱面数据（note 数、BPM 等） |
| `QueryChunithmMusicDataBatch(ctx, musicIDs, version)` | 批量查询曲目信息 + 难度常数（缺失曲目填充 Unknown 占位） |
| `GetChunithmDefaultServer(ctx, harukiUserID)` | 用户默认服务器 |
| `GetChunithmBinding(ctx, harukiUserID, server)` | 用户指定服务器的 Aime 绑定 |

**批量查询说明（`QueryChunithmMusicDataBatch`）：**
- 输入：`[]int` musicID 列表 + version 字符串
- 输出：`map[int]ChunithmMusicBatchItem`（键为 musicID，缺失条目填充 Unknown 占位而非报错）
- 内部执行两次查询（曲目 + 难度），并在内存中合并，避免 N+1 问题

### 5.3 用户查询（`users.go`）

| 方法 | 说明 |
|------|------|
| `GetUserByPlatform(ctx, platform, platformUserID)` | 平台账号 → `UserInfo`（含全套封禁状态字段） |
| `GetUserByID(ctx, harukiUserID)` | Haruki 内部 ID → `UserInfo` |

---

## 6. 返回类型

所有方法返回 `utils/types` 包中的共享类型：

| 类型 | 字段 |
|------|------|
| `AliasToIDResponse` | `MatchIDs []int` |
| `AliasListResponse` | `Aliases []string` |
| `PJSKBinding` | `ID, HarukiUserID, Server, UserID, Visible` |
| `PJSKBindingResponse` | `Binding *PJSKBinding`（单条）或 `Bindings []PJSKBinding`（多条） |
| `PJSKPreference` | `Option, Value` |
| `PJSKPreferenceResponse` | `Option *PJSKPreference`（单条）或 `Options []PJSKPreference`（多条） |
| `ChunithmMusicInfo` | `MusicID, Title, Artist, Category, Version, ReleaseDate, IsDeleted, DeletedVersion` |
| `ChunithmMusicDifficulty` | `MusicID, Version, Diff0~Diff4`（`*float64`，可为 nil） |
| `ChunithmChartData` | `Difficulty, Creator, BPM, TapCount, HoldCount, SlideCount, AirCount, FlickCount, TotalCount` |
| `ChunithmMusicBatchItem` | `Info ChunithmMusicInfo, Version, Difficulty [5]*float64` |
| `ChunithmBinding` | `UserID, Server, AimeID` |
| `ChunithmDefaultServer` | `UserID, Server` |
| `UserInfo` | `ID, Platform, UserID, BanState, BanReason`（+ 各功能维度封禁字段） |

---

## 7. 与原有 README 的关系

包内 `README.md` 记录了一个历史映射表：部分旧版"私有查询 API"（路径如 `/pjsk/user/:id/binding`、`/chunithm/user/:id/default`）的职责已被这些方法取代，路由层不再注册对应的公开端点。

---

## 8. 当前使用状态

| 调用方 | 文件 | 说明 |
|--------|------|------|
| 集成测试 | `integration/full_integration_test.go` | 完整测试各方法 |

> ⚠ 当前生产 API Handler（`api/public/pjsk/`, `api/public/chunithm/` 等）**尚未使用本包**，仍直接持有各自的原始 Ent 客户端。未来 Handler 重构时应统一切换至 `query.Client` 以复用校验和哨兵错误逻辑。

---

## 9. 测试

`client_test.go` 包含 5 个测试函数，使用 SQLite 内存数据库（`go-sqlite3`）：

| 测试 | 内容 |
|------|------|
| `TestClient_ConfigValidation` | `nil` 客户端时返回 `ErrXxxNotConfigured` |
| `TestClient_ChunithmQueries` | 所有 Chunithm 查询正常路径 |
| `TestClient_PJSKQueries` | 所有 PJSK 查询正常路径 |
| `TestClient_UserQueries` | 用户查询正常路径 |
| `TestClient_ChunithmQueryErrors` | Chunithm 全部错误路径 |
| `TestClient_PJSKQueryErrors` | PJSK 全部错误路径 |
| `TestClient_UserQueryErrors` | 用户全部错误路径 |

```bash
go test ./utils/query/...
```

---

**维护者**：Haruki-Cloud Team  
**文档版本**：v1.0  
**创建日期**：2026-03-23
