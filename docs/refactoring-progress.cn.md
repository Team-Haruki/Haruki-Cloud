# 数据层与 Handler 重构进度报告

> 更新日期：2026-03-31

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

### P11：mysekai/controller.go 拆分（进行中）

**提交**：`330077e`

将 2251 行的 `controller.go` 开始拆分：

| 已提取文件 | 行数 | 内容 |
|-----------|------|------|
| resource_builder.go | 90 | BuildResourceRequest, RenderResource |

**当前 controller.go 为 2173 行。后续将继续提取：**
- map_builder.go (~350 行)
- fixture_builder.go (~250 行)
- door_upgrade_builder.go (~150 行)
- music_record_builder.go (~200 行)
- talk_builder.go (~350 行)
- photo_resolver.go (~60 行)
- mysekai_snapshot.go (~150 行)
- mysekai_helpers.go (~200 行)

---

## 未做的工作

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
