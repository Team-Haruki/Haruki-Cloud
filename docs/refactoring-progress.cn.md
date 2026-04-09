# 数据层与 Handler 重构进度报告

> 更新日期：2026-03-31
>
> 2026-04-09 补充说明：本文是 2026-03-31 当时的阶段性进度快照，不是当前架构总览。
>
> - `api/legacy/pjsk/` 已从仓库与运行时移除
> - `internal/pjsk/render/deck/deck_cgo/` 已从仓库移除
> - PJSK 当前主链应以 `api/bot/pjsk`、`internal/pjsk/handler` 与 [项目完成度跟踪](project-completion-tracker.cn.md) 为准
> - 2026-04-09 追加稳定化：`music` / `score` / `sk` / `misc birthday` 主请求链已继续清理 `context.Background()`；服务退出路径补上了 `Redis` 与 `censor DB` 关闭
> - 2026-04-09 再次追加：`card` / `event` / `gacha` / `music` / `profile` 已支持请求级 source/provider 克隆，`DatabaseProvider` 相关 masterdata 查询开始跟随请求 `ctx`
> - 2026-04-09 本轮继续追加：`education` / `stamp` / `vlive` 也已接入请求级 source/provider 克隆；主链 render provider 的 DB 查询上下文债务已基本清完，剩余 `Background()` 主要在本地调试 helper 和 nil-ctx 兜底逻辑
> - 2026-04-09 本轮再补一层：`internal/pjsk/render/userdata` 的 `DefaultSnapshotFactory.Build(ctx, ...)` 已真正使用 `ctx`，live/local snapshot 构建入口也已补上 context-aware 版本；leader 图路径解析和 MySekai merge helper 进一步收口
> - 2026-04-09 再补脚本侧：`cmd/migrate` 已去除硬编码 Sekai DSN，改为环境变量/配置文件解析，并接入 signal-aware context
> - 2026-04-09 阶段 B 结构拆分：`internal/pjsk/render/sk/` 已继续细拆为 `controller_base.go` / `controller_line_requests.go` / `controller_query_requests.go` / `controller_speed_requests.go` / `controller_trace_requests.go` / `controller_trace.go` / `controller_validate.go` / `controller_tracker_identity.go` / `controller_tracker_name.go` / `controller_tracker_metrics.go` / `controller_meta.go` / `controller_winrate.go`；`internal/pjsk/handler/resolver.go` 已按 target/snapshot/mysekai/profile/binding 职责拆分；`executeSK()` 已收口为薄调度函数并下沉到分模式 handler
>
> 文中提到的历史 bridge 结构、legacy 路由或本地 native/deck 方案，都应视为当时阶段背景，而不是当前实现。

---

## 概述

本次重构的核心目标是消除 handler 层的"胶水代码"——原 `bridge.go`（2968 行单文件），建立统一数据访问接口，并将 handler 执行逻辑模块化。

重构分为 6 个阶段（P0–P5），目前已全部完成。

---

## 已完成的工作

### P0：公共工具提取 ✅

**提交**：`0e57af2`

将分散在 6 个模块中的重复工具函数提取为 `internal/pjsk/render/common/` 包：

| 文件 | 内容 | 消除重复行数 |
|------|------|-------------|
| `json.go` | `JSONString`、`DecodeSlice`、`DecodeMap`、`ToStringSliceFromRaw` | ~120 行 |
| `convert.go` | 5 个 `Convert*` 类型转换函数 | ~200 行 |
| `clone.go` | 13 个 `Clone*` 深拷贝函数 | ~265 行 |

**合计消除约 585 行重复代码。**

---

### P1：统一 MasterDataProvider 接口 ✅

**提交**：`ec88ae6`

#### 接口设计

新建 `internal/pjsk/render/provider/` 包，定义顶层接口：

```go
type MasterDataProvider interface {
    Cards()        CardProvider
    Events()       EventProvider
    Musics()       MusicProvider
    Characters()   CharacterProvider
    Skills()       SkillProvider
    Gachas()       GachaProvider
    Honors()       HonorProvider
    Stamps()       StampProvider
    VLives()       VLiveProvider
    Education()    EducationProvider
    PlayerFrames() PlayerFrameProvider
    MySekai()      MySekaiProvider
    Region()       renderregion.Value
}
```

12 个子接口，共计 52 个方法。

#### 两种实现

| 实现 | 文件 | 用途 |
|------|------|------|
| `DatabaseProvider` | `database.go` + 12 个 `db_*.go` | 从 SekaI DB 读取，用于生产环境 |
| `LocalProvider` | `local.go` + `local_loader.go` + `local_data.go` | 从本地 JSON 文件读取，用于测试/离线 |

#### 适配器层

9 个适配器文件将统一 `MasterDataProvider` 桥接到各模块现有的 `DataSource`/`Source` 接口：

```
provider/         →  card/adapter_provider.go    → card.DataSource
                  →  event/adapter_provider.go   → event.DataSource
                  →  music/adapter_provider.go   → music.DataSource
                  →  gacha/adapter_provider.go   → gacha.DataSource
                  →  honor/adapter_provider.go   → honor.DataSource
                  →  stamp/adapter_provider.go   → stamp.DataSource
                  →  vlive/adapter_provider.go   → vlive.Source
                  →  profile/adapter_provider.go → profile.Source
                  →  education/adapter_provider.go → education.Source
```

#### 测试

- 70 个单元测试（48 provider + 22 clone）全部通过

---

### P2：Snapshot 解析器提取 ✅

**提交**：`04096e0`

从 bridge.go 中提取 11 个函数/类型到 `internal/pjsk/handler/resolver.go`（356 行）：

- `resolveLiveSnapshot` — 绑定→Toolbox API→用户数据
- `resolveGameTarget` — 解析游戏目标用户（6 处调用）
- `buildPublicMusicProfiles` — 构建公开音乐档案（4 处调用）
- `resolveRegionFromDefaultBinding` — 区域推断
- 类型：`userQueryParams`、`resolvedGameTarget`

---

### P3：Handler 运行时上下文 ✅

**提交**：`04096e0`（与 P2 同一提交）

新建 `internal/pjsk/handler/runtime.go`（115 行），定义 `RequestContext`：

```go
type RequestContext struct {
    Ctx            context.Context
    Cmd            *parser.ResolvedCommand
    App            *renderapp.App
    Region         renderregion.Value
    RegionStr      string
    Platform       string
    PlatformUserID string
    // 延迟加载字段
}
```

特性：
- **延迟绑定解析**：通过 `sync.Once` 实现，首次调用时解析
- **延迟档案解析**：`GetDetailedProfile()` / `GetProfileCard()` 缓存结果
- **快照代理**：`ResolveSnapshot(needMySekai)` 按需获取
- **便捷方法**：`ImageMessage()` 封装图片缓存+URL 返回

---

### P4：全函数迁移至 RequestContext ✅

**提交**：`e19efeb`

将全部 17 个 `execute*` 函数统一迁移到 `*RequestContext` 签名：

```go
// 迁移前（3 种签名混用）
func executeGacha(r *parser.ResolvedCommand, app *renderapp.App)
func executeSK(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App)

// 迁移后（统一签名）
func executeGacha(rc *RequestContext)
func executeSK(rc *RequestContext)
```

`Execute()` 调度函数现在统一创建一个 `RequestContext` 并传递给所有模块。

---

### P5：bridge.go 模块拆分 ✅

**提交**：`e19efeb`（与 P4 同一提交）

将 2624 行的单体 `bridge.go` 拆分为 18 个模块文件：

| 文件 | 行数 | 内容 |
|------|------|------|
| `bridge.go` | 133 | `Execute()` 调度 + 共享工具函数 |
| `bridge_education.go` | 497 | 育成（挑战/领袖/战力/羁绊/区域道具） |
| `bridge_sk.go` | 302 | 冲榜追踪器 |
| `bridge_arrest.go` | 299 | 逮捕 + 角色查找 |
| `bridge_event.go` | 282 | 活动详情/列表/记录 |
| `bridge_deck.go` | 278 | 组卡推荐 |
| `bridge_music.go` | 175 | 音乐详情/BPM/曲绘 |
| `bridge_mysekai.go` | 157 | MySekai 功能 |
| `bridge_regtime.go` | 141 | 注册时间 |
| `bridge_checkdata.go` | 134 | 数据检查 |
| `bridge_profile.go` | 109 | 个人信息 |
| `bridge_score.go` | 97 | 分数/控制室/Meta |
| `bridge_card.go` | 62 | 卡牌 |
| `bridge_stamp.go` | 47 | 贴纸 |
| `bridge_misc.go` | 32 | 杂项（生日等） |
| `bridge_gacha.go` | 29 | 抽卡 |
| `bridge_vlive.go` | 21 | 虚拟 Live |
| `bridge_alias.go` | 19 | 别名 |

---

## 量化统计

| 指标 | 变化 |
|------|------|
| bridge.go 行数 | 2968 → 133（-95.5%） |
| 消除重复代码 | ~585 行（common 包） |
| 删除冗余代码 | ~3220 行（source_cloud.go 文件） |
| local_data.go 拆分 | 2007 行 → 12 个文件 |
| 新增 Provider 接口方法 | 52 个 |
| 新增单元测试 | 70 个 |
| 新增文件 | ~50 个 |
| Handler 文件数 | 4 → 22（bridge.go + 17 个模块文件 + resolver.go + runtime.go + test） |

---

## 追加完成的工作

### P6：删除冗余 CloudSource 实现 ✅

**提交**：`2d01179`

删除 9 个 `source_cloud.go` 文件，共计 3220 行冗余代码：

| 模块 | 删除行数 |
|------|----------|
| card/source_cloud.go | 855 |
| education/source_cloud.go | 541 |
| music/source_cloud.go | 502 |
| event/source_cloud.go | 380 |
| honor/source_cloud.go | 318 |
| profile/source_cloud.go | 218 |
| gacha/source_cloud.go | 176 |
| vlive/source_cloud.go | 102 |
| stamp/source_cloud.go | 69 |

所有模块现在统一使用 `ProviderAdapter` 桥接到 `MasterDataProvider`。

---

### P7：拆分 local_data.go ✅

**提交**：`08551b6`

将 2007 行的 `local_data.go` 拆分为 12 个模块文件：

| 文件 | 行数 |
|------|------|
| local_cards.go | 359 |
| local_musics.go | 359 |
| local_education.go | 314 |
| local_honors.go | 275 |
| local_events.go | 243 |
| local_skills.go | 104 |
| local_characters.go | 103 |
| local_gachas.go | 102 |
| local_player_frames.go | 84 |
| local_vlives.go | 76 |
| local_mysekai.go | 51 |
| local_stamps.go | 34 |

---

### P8：统一 DataSource 接口命名 ✅

**提交**：`18c6d16`

将 `Source` 重命名为 `DataSource`，统一所有模块的接口命名风格：

- education/source.go: `Source` → `DataSource`
- profile/source.go: `Source` → `DataSource`
- vlive/source.go: `Source` → `DataSource`

现在所有 9 个模块都使用 `DataSource` 作为接口名。

---

### P9：文件拆分 - drawing/models.go + deck_params.go ✅

**提交**：`f518b4b`

将两个大型单文件拆分为领域模块：

#### utils/drawing/models.go (1189 行) → 11 个文件
| 文件 | 内容 |
|------|------|
| models_music.go | 音乐相关结构 |
| models_card.go | 卡牌相关结构 |
| models_profile.go | 档案相关结构 |
| models_event.go | 活动相关结构 |
| models_education.go | 育成相关结构 |
| models_honor.go | 称号相关结构 |
| models_gacha.go | 抽卡相关结构 |
| models_sk.go | 冲榜相关结构 |
| models_score.go | 分数相关结构 |
| models_mysekai.go | MySekai相关结构 |
| models_misc.go | 杂项结构 |

#### deck_params.go (958 行) → 5 个文件
| 文件 | 内容 |
|------|------|
| deck_types.go | 类型定义 |
| deck_builder.go | 请求构建 |
| deck_extractor.go | 数据提取 |
| deck_config.go | 配置管理 |
| deck_helpers.go | 辅助函数 |

---

### P10：binding_service.go 拆分 ✅

**提交**：`6031c62`

将 1018 行的 `binding_service.go` 拆分为 4 个模块：

| 文件 | 行数 | 内容 |
|------|------|------|
| binding_types.go | 101 | 接口、类型、常量 |
| binding_defaults.go | 244 | 默认绑定管理 |
| binding_properties.go | 217 | 可见性、验证、背景 |
| binding_service.go | 517 | 核心绑定操作 |

**总计减少约 500 行分散重复代码。**

---

### P11：mysekai/controller.go 拆分 ✅

**提交**：`ae804ce`

将 2251 行的 `controller.go` 拆分为 7 个模块文件：

| 已提取文件 | 行数 | 内容 |
|-----------|------|------|
| resource_builder.go | 90 | BuildResourceRequest, RenderResource |
| map_builder.go | 390 | BuildMapRequest, RenderMap |
| fixture_builder.go | 364 | BuildFixtureListRequest, BuildFixtureDetailRequests |
| door_upgrade_builder.go | 154 | BuildDoorUpgradeRequest |
| music_record_builder.go | 189 | BuildMusicRecordRequest |
| talk_builder.go | 287 | BuildTalkListRequest |

**controller.go 从 2251 行降至 859 行**（减少 62%）

---

### P12：alias/service.go 拆分 ✅

**提交**：`b1c6129`

将 1258 行的 `service.go` 拆分：

| 已提取文件 | 行数 | 内容 |
|-----------|------|------|
| service_resolver.go | 332 | TryResolveMusicID, TryResolveCharacterID, 所有 tryResolve* 方法 |

**service.go 从 1258 行降至 944 行**（减少 25%）

---

### P13：sk/controller.go 拆分 ✅

**提交**：`d2c2f60`

将 1271 行的 `controller.go` 拆分：

| 已提取文件 | 行数 | 内容 |
|-----------|------|------|
| tracker_builder.go | 586 | validateTrackerQuery, buildRanksFromTracker, buildSingleRankFromTracker, enrichRankInfoByRank/User, buildSpeedInfosFromTracker, buildRankTraceFromTracker |

**controller.go 从 1271 行降至 700 行**（减少 45%）

---

### P14：deck/controller.go 拆分 ✅

**提交**：`ccc988f`

将 1184 行的 `controller.go` 拆分：

| 已提取文件 | 行数 | 内容 |
|-----------|------|------|
| helpers.go | 291 | optionString/Int/Float, normalizeRecommend*, resolveCharacterIconPath, resolveUnitIconPath, defaultDeckConfig*, toInterfaceSlice/Map, calculateDeckCardPower |

**controller.go 从 1184 行降至 918 行**（减少 22%）

---

### P15：music/board_request.go 拆分 ✅

**提交**：`ee2b053`

将 872 行的 `board_request.go` 拆分：

| 已提取文件 | 行数 | 内容 |
|-----------|------|------|
| board_helpers.go | 449 | weightedMusicBoardSkill, populateMusicBoardLiveMetrics, sortMusicBoardRows, loadMusicBoardMetaMap, resolveMusicBoardSpecs, boardDifficultyPriority |

**board_request.go 从 872 行降至 420 行**（减少 52%）

---

### P16：music/controller.go 拆分 ✅

**提交**：`d2737d5`

将 782 行的 `controller.go` 拆分：

| 已提取文件 | 行数 | 内容 |
|-----------|------|------|
| controller_helpers.go | 301 | currentSnapshot, profile helpers, buildPlaceholderProfile, convertDetailedProfileToCard, buildPlayResultIconMap, buildUserResults, buildUserProgressCounts, flattenProgressCounts, resolveMusicChartMeta, matchesMusicKeyword, resolveStaticIcon |

**controller.go 从 782 行降至 494 行**（减少 37%）

---

### P17：drawing/cache.go 拆分 ✅

**提交**：`4f8f5dc`

将 728 行的 `cache.go` 拆分：

| 已提取文件 | 行数 | 内容 |
|-----------|------|------|
| cache_helpers.go | 482 | policy building, endpoint parsing, payload normalization, sanitization, user ID extraction, profile detection, JSON traversal helpers (mapAt, valueAt, sliceAt等), scalar conversion, scope value, digest utilities |

**cache.go 从 728 行降至 260 行**（减少 64%）

---

### P18：userdata/local.go 拆分 ✅

**提交**：`060850e`

将 689 行的 `local.go` 拆分：

| 已提取文件 | 行数 | 内容 |
|-----------|------|------|
| local_helpers.go | 265 | mergeMySekaiJSON, resolveLeaderImagePath, fallbackLeaderImagePath, makeRelativeAsset, buildUserCardEntries, findActiveDeck, findUserCard, buildMusicResultMap, normalizePlayResult, prioritizePlayResult, convertChallenge*函数 |

**local.go 从 689 行降至 438 行**（减少 36%）

---

### P19：music/builder.go 拆分 ✅

**提交**：`a7b8af9`

将 678 行的 `builder.go` 拆分：

| 已提取文件 | 行数 | 内容 |
|-----------|------|------|
| builder_helpers.go | 253 | normalizeDifficulty, containsString, regionToLocation, formatTimestamp, selectLocalizedTitle, buildJPVocalOrderKey, normalizeVocalCaption, classifyVocalByAssetBundle, localizeVocalCaption, vocalLocalizationByRegion |

**builder.go 从 678 行降至 435 行**（减少 36%）

---

### P20：profile/controller.go 拆分 ✅

**提交**：`4a2d5e8`

将 652 行的 `controller.go` 拆分：

| 已提取文件 | 行数 | 内容 |
|-----------|------|------|
| controller_helpers.go | 369 | logProfilePayloadDebug, resolveProfileBGSettings, buildAPIUserCardEntries, buildLeaderImagePathFromSource, buildFramePaths, buildPCards, buildHonors, findEventRank, buildMusicCounts, buildCharacterRanks, buildSoloLive, buildCharaIconMap |

**controller.go 从 652 行降至 297 行**（减少 54%）

---

### P21：handler/sekai/score.go 拆分 ✅

**提交**：`9acee70`

将 596 行的 `score.go` 拆分：

| 已提取文件 | 行数 | 内容 |
|-----------|------|------|
| score_board_params.go | 462 | buildMusicBoardParams 及全部 board 参数解析函数 (extractMusicBoard*, parseMusicBoard*, resolveMusicBoard*, isMusicBoardLevelFilter) |

**score.go 从 596 行降至 143 行**（减少 76%）

---

### P22：education/snapshot_build.go 拆分 ✅

**提交**：`c3e9dbf`

将 582 行的 `snapshot_build.go` 拆分：

| 已提取文件 | 行数 | 内容 |
|-----------|------|------|
| snapshot_helpers.go | 161 | hasAreaItemFilter, areaItemMatchesFilter, areaItemTargetIcon, unitIconPath, attrIconPath, materialIconPath, normalizeUnit, normalizeAttr |

**snapshot_build.go 从 582 行降至 431 行**（减少 26%）

---

### P23：card/builder.go 拆分 ✅

**提交**：`336e0fd`

将 577 行的 `builder.go` 拆分：

| 已提取文件 | 行数 | 内容 |
|-----------|------|------|
| builder_helpers.go | 253 | BuildCardBasic, buildThumbnailInfo, calculatePower, buildDualSkillDetail, combineSkillLines, buildCardImagePaths, buildCostumeImagePaths, BuildCharacterIconPath, buildUnitLogoPath, buildSkillTypeIconPath, buildEventBannerPath, buildGachaBannerPath |

**builder.go 从 577 行降至 338 行**（减少 41%）

---

### P24：handler/sekai/sk.go 拆分 ✅

**提交**：`0efa1d2`

将 574 行的 `sk.go` 拆分：

| 已提取文件 | 行数 | 内容 |
|-----------|------|------|
| sk_params.go | 368 | buildSKTrackerParams, buildSKPlayerTraceParams, extractSKMetaArgs, parseSKWorldBloomCharacterToken, splitSKWorldBloomCharacterAndRanks, parseSKRanks, normalizeRanks, defaultSKRanksByMode |

**sk.go 从 574 行降至 212 行**（减少 63%）

---

### P25：requestbuilder/misc_birthday.go 拆分 ✅

**提交**：`59f866e`

将 562 行的 `misc_birthday.go` 拆分：

| 已提取文件 | 行数 | 内容 |
|-----------|------|------|
| birthday_helpers.go | 266 | matchBirthdayCharacterIDs, birthdayCharacterMatches/Names, loadBirthdayCards, birthdayCardImagePath, resolveBirthdayColorCode, buildBirthdayCalendar, nextBirthdayTime, birthdayRegionLocation, birthdayDaysUntil, birthdayTimeText |

**misc_birthday.go 从 562 行降至 311 行**（减少 45%）

---

### P26：api/legacy/pjsk/render_route.go 拆分 ✅

**提交**：`3c29e2d`

将 1518 行的 `render_route.go` 按域拆分为 4 个文件：

| 文件 | 行数 | 内容 |
|------|------|------|
| render_route.go | 668 | 路由注册、通用helper、card/deck/event/gacha/honor/profile/misc handler |
| render_route_mysekai.go | 182 | MySekai Build/Render handler |
| render_route_music.go | 290 | Music + Education Build/Render handler |
| render_route_sk.go | 410 | SK + Score + Stamp Build/Render handler |

**render_route.go 从 1518 行降至 668 行**（减少 56%）

---

### P27：mysekai/helpers.go 拆分 ✅

**提交**：`7dea8e4`

将 542 行的 `helpers.go` 拆分：

| 已提取文件 | 行数 | 内容 |
|-----------|------|------|
| helpers_convert.go | 126 | intNumber, floatNumber, int64Number, boolValue, stringValue, nestedList, nestedInt, parseIntTokens |

**helpers.go 从 542 行降至 423 行**（减少 22%）

### 1. 快照 Provider（snapshot-schema / store）

用户快照的本地写入/读取 Provider 已标记为 `blocked`。当前架构通过 Toolbox API 实时获取用户数据，不经过本地 DB。

### 2. 删除 bridge 命名

虽然已拆分，但文件仍以 `bridge_` 为前缀。未来可考虑重命名为更直观的名称（如 `exec_card.go`、`exec_event.go`），但这是纯审美改动。

### 3. Helper 函数签名统一

部分辅助函数仍使用 `(r *parser.ResolvedCommand, app *renderapp.App)` 签名，而非 `*RequestContext`。这些函数的调用方已正确传递 `rc.Cmd` 和 `rc.App`，功能正确，但风格不统一。

---

## 建议的后续改进

### 短期（建议优先）

1. **修复 2 个预存测试失败**
   - `TestBuildCardBoxRequestUsesOwnedCardDefaultImageEvenWhenBeforeIsSet`（card 包）
   - `TestBuildBondsRequestFromSuiteIncludesFallbackIconsAndProgress`（handler 包，颜色值不匹配）

2. **统一 Helper 函数签名**：将高频辅助函数（如 `renderMusicRewards`、`buildEventRecordFromSnapshot`）也迁移到 `*RequestContext`

### 中期

4. **错误处理标准化**：部分 execute* 函数的错误处理不一致（有的返回 nil message + error，有的返回 text message + nil error）

5. **Education 类型去重**：`provider/education.go` 中定义的 Education 类型与 `education/` 包中的类型重复，可考虑抽取为共享类型包

6. **VLive 类型去重**：同上，`provider.VLive` vs `vlive.Live`

### 长期

7. **集成测试自动化**：当前 75 个端点的集成测试依赖手动触发，建议接入 CI

8. **模块级 RequestContext**：为不同模块定义更精确的 Context 类型（如 `MusicContext` 只暴露音乐相关方法），提高类型安全性

---

## 文件目录结构（重构后）

```
internal/pjsk/handler/
├── bridge.go              # Execute 调度 + 共享工具（133 行）
├── bridge_alias.go        # 别名
├── bridge_arrest.go       # 逮捕
├── bridge_card.go         # 卡牌
├── bridge_checkdata.go    # 数据检查
├── bridge_deck.go         # 组卡
├── bridge_education.go    # 育成
├── bridge_event.go        # 活动
├── bridge_gacha.go        # 抽卡
├── bridge_misc.go         # 杂项
├── bridge_music.go        # 音乐
├── bridge_mysekai.go      # MySekai
├── bridge_profile.go      # 个人信息
├── bridge_regtime.go      # 注册时间
├── bridge_score.go        # 分数
├── bridge_sk.go           # 冲榜
├── bridge_stamp.go        # 贴纸
├── bridge_vlive.go        # 虚拟 Live
├── bridge_test.go         # 测试
├── resolver.go            # 快照/绑定/档案解析
├── runtime.go             # RequestContext
├── handler.go             # Handler 注册
└── context.go             # 请求上下文构建

internal/pjsk/render/
├── common/
│   ├── json.go            # JSON 工具
│   ├── convert.go         # 类型转换
│   └── clone.go           # 深拷贝
├── provider/
│   ├── provider.go        # MasterDataProvider 接口
│   ├── cards.go           # CardProvider 接口
│   ├── characters.go      # CharacterProvider 接口
│   ├── skills.go          # SkillProvider 接口
│   ├── events.go          # EventProvider 接口
│   ├── musics.go          # MusicProvider 接口
│   ├── gachas.go          # GachaProvider 接口
│   ├── honors.go          # HonorProvider 接口
│   ├── stamps.go          # StampProvider 接口
│   ├── vlives.go          # VLiveProvider 接口
│   ├── education.go       # EducationProvider 接口
│   ├── player_frames.go   # PlayerFrameProvider 接口
│   ├── mysekai.go         # MySekaiProvider 接口
│   ├── database.go        # DatabaseProvider 实现
│   ├── db_*.go            # 12 个 DB 子实现
│   ├── local.go           # LocalProvider 实现
│   ├── local_loader.go    # JSON 文件加载器
│   ├── local_data.go      # 12 个本地子实现
│   ├── provider_test.go   # 48 个测试
│   └── clone_test.go      # 22 个测试
├── card/adapter_provider.go
├── event/adapter_provider.go
├── music/adapter_provider.go
├── gacha/adapter_provider.go
├── honor/adapter_provider.go
├── stamp/adapter_provider.go
├── vlive/adapter_provider.go
├── profile/adapter_provider.go
└── education/adapter_provider.go
```

---

## 后续重构阶段（2026-04）

### R22：Context 传播优化 ✅

**提交**：`006faa5`

将 13 处 `context.Background()` 调用替换为 `rc.Ctx`：

| 文件 | 修改位置 |
|------|----------|
| bridge_card.go | Card 相关查询 |
| bridge_deck.go | Deck 相关查询 |
| bridge_education.go | Education 相关查询 |
| bridge_event.go | Event 相关查询 |
| bridge_music.go | Music 相关查询 |
| bridge_sk.go | SK 相关查询 |
| runtime.go | RequestContext 初始化 |
| resolver.go | 绑定解析 |

---

### R16：默认区域常量集中化 ✅

**提交**：`fb44d54`

创建 `internal/pjsk/handler/defaults.go`：

```go
const DefaultRegionStr = "jp"

func regionWithDefault(region string) string {
    if region == "" {
        return DefaultRegionStr
    }
    return region
}
```

替换 11 处散落的 `"jp"` 字符串字面量和 3 处默认区域模式。

---

### R26：死代码清理 ✅

**提交**：`eae6f68`

移除 `bridge_stamp.go` 中的不可达代码（exhaustive switch 后的重复 error return）。

---

### Test 分支合并 ✅

**提交**：`a65a86a`

合并 test 分支的 21 个提交到 refactor/test：

| 功能 | 说明 |
|------|------|
| Deck 角色昵称解析 | 角色昵称（如 "miku"、"初音未来"）直接解析为角色 ID |
| Education Bonds 过滤 | 添加角色过滤参数支持 |
| Education EX 等级 | Leader Count 支持 EX 等级显示 |
| Stamp 优化 | 单表情包直接返回资源 URL |
| SK 预测模式 | 添加 sk-predict 命令和 forecast provider |

文件变更：36 个文件，+4119/-1289 行

---

### R27：错误消息与 Context 传播 ✅

**提交**：`bdf7ed9`

创建 `internal/pjsk/handler/messages.go`：

```go
const (
    ErrMsgSuiteDataUnavailable   = "当前账号没有可用的 Suite 抓包数据"
    ErrMsgMySekaiDataUnavailable = "当前账号没有可用的 MySekai 抓包数据"
    ErrMsgSelfQueryOnly          = "%s仅支持查询自己的数据"
    ErrMsgBindingServiceUnavailable = "绑定服务未就绪"
)

func unsupportedModeError(module, mode string) error
```

替换 12 处重复的 `unsupported mode` 错误模式。

修复 `loadLeaderMissionRequirements` 的 context.Background() 调用。

---

### R28：绑定解析模式统一 ✅

**提交**：`4341efa`

创建 `resolveBindingWithFallback()` 辅助函数，封装常见的 "全局默认 → 区域回退" 绑定解析模式：

```go
type bindingResolutionOptions struct {
    RequireSuite   bool
    RequireMySekai bool
}

func resolveBindingWithFallback(
    ctx context.Context,
    bindings *accountdata.BindingService,
    platform, platformUserID, regionStr string,
    regionExplicit bool,
    opts bindingResolutionOptions,
) (int, *accountdata.ResolvedBinding, error)
```

替换 4 处重复的绑定解析逻辑：
- `resolveLiveSnapshot` (resolver.go)
- `resolveMySekaiOnly` (resolver.go)
- `executeEducation` (bridge_education.go)
- `resolveRequesterGameUID` (bridge_sk.go)

减少约 40 行重复代码。

---

## 剩余重构建议

根据代码分析，以下是优先级排序的剩余重构项：

### 高优先级

| 项目 | 影响文件数 | 预计工时 |
|------|-----------|---------|
| 服务 nil 检查模式统一 | 6 | 1-2h |
| 绑定解析模式统一 | 4 | 2-3h |

### 中优先级

| 项目 | 影响文件数 | 预计工时 |
|------|-----------|---------|
| Provider 缓存初始化优化 | 12 | 3-4h |
| Mutex 解锁模式（使用 defer）| 21+ | 2h |

### 低优先级

| 项目 | 影响文件数 | 预计工时 |
|------|-----------|---------|
| 大函数拆分（executeEducation 等）| 2-3 | 4-5h |
| 魔法数字常量化 | 多 | 1-2h |
| 命名规范统一 | 多 | 2h |

---

## 统计数据

### 重构前后对比

| 指标 | 重构前 | 重构后 | 变化 |
|------|--------|--------|------|
| bridge.go 行数 | 2968 | 133 | -95.5% |
| 最大单文件行数 | 2968 | ~700 | -76% |
| handler/*.go 文件数 | 3 | 22+ | +633% |
| Provider 测试覆盖 | 0 | 70 | +70 |
| 重复代码（估计）| ~1500 行 | ~300 行 | -80% |

### 提交统计

- P0-P5 基础重构：6 个阶段，~25 个提交
- P6-P27 优化重构：22 个阶段，~22 个提交
- Test 分支合并：1 个合并提交
- **总计**：~48 个重构相关提交

---

> 文档更新日期：2026-04-01
>
> 2026-04-10 追加：阶段 A-D 代码审计与架构优化已完成

---

## 阶段 A-D 代码审计与架构优化（2026-04-10）

在 P0-R28 基础上，对全项目进行系统性代码审计，发现 18 个改进点并按 A-D 四阶段执行。

### 阶段 A：快速修复 ✅（commit 220ed19）

- A1: 删除 bridge_event + cache_helpers 7 个未调用函数
- A2: 合并重复的 UUID 掩码函数（arrestDisplayUID → maskPJSKUID）
- A3: 修复 bridge_card.go 隐式 (nil, nil) return
- A4: 补充 honor adapter 缺失的 WithContext() 方法
- A5: checkdata 绑定解析用 resolveBindingWithFallback 替换重复闭包

### 阶段 C：模式统一 ✅（commit 3a91f48）

- C1: helper 函数签名统一为 *RequestContext（resolver.go, bridge_event/sk/card, runtime.go）
- C2: api/helper.go 错误处理清理
- C3: cmd/server 初始化模式统一（init_database.go, init_services.go, server.go）
- C4: API 缓存中间件抽象（helper.go + chunithm/pjsk 路由）

### 阶段 D：架构优化 ✅（commit 22b12df）

- **D1: ProviderAdapter 基类**：创建 `provider.ProviderAdapterBase`，嵌入 `MasterDataProvider` 并提供 `DefaultRegion()` 和 `CloneWithContext()` 方法。9 个 render 模块的 adapter_provider.go 均改为嵌入基类。
- **D2: contextual.go 泛型简化**：已分析，因 Go 泛型无法抽象不同接口方法集而标记为 blocked（需代码生成）
- **D3: lazyValue[T] 泛型**：创建 `provider/lazy.go`，用 `lazyValue[T]` 替换 11 个 local_*.go 文件中的 40+ 个 `sync.Once` + error + data 字段三元组。多值 loader 使用私有 wrapper struct（cardIndex, eventIndex, musicIndex 等）。
- **D4: Sekai 客户端基础提取**：创建 `sekai/client_base.go`，提取共享的 `newRestyClient()` 和 `isRetryable` 重试逻辑。3 个 HTTP 客户端（api, toolbox, tracker）共享同一份重试配置。

### 净效果

- 减少约 110 行重复代码
- 消除 40+ 个 sync.Once 字段三元组
- 消除 9× 重复的 DefaultRegion/WithContext 模板
- 消除 3× 重复的 resty 重试逻辑
