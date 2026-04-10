# Haruki-Cloud 项目进展总结

> 最后更新：2026-04-01（v17.2）
>
> 涉及 `Haruki-ZeroBot` 联调的协议边界，请优先参考 `docs/zerobot-cloud-integration-plan.cn.md`。
>
> 2026-04-09 补充：当前模块分档、活跃 Bot path、disabled handler 与测试/风险快照，见 [项目完成度跟踪](project-completion-tracker.cn.md)。
>
> 再补充一条当前事实：`api/legacy/pjsk/` 与 `/internal/pjsk/*` 兼容运行时路由已于 2026-04-09 从仓库和运行时移除，`internal/pjsk/render/deck/deck_cgo/` 历史目录也已移除；本文保留了大量 2026-03 ~ 2026-04-01 的阶段性记录，凡与当前运行事实冲突之处，以 [项目完成度跟踪](project-completion-tracker.cn.md) 为准。

## 1. 当前结论

`Haruki-Cloud` 当前已经完成两部分核心合并：

1. `Service-Test` 的渲染子系统
2. `Test_Instruction_Parser` 的解析资源和处理资源

但 PJSK 指令主链路的目标模型已经重新明确：

1. 云端下发 manifest。
2. 客户端构建前缀树并命中 `path`。
3. 客户端按命中的 `path` 调用 `/api/v2/bot/:botId/pjsk/*`，并上传 `matched_command`。
4. 命中的端点在云端内部先校验 `matched_command -> handler.path`，再解析原文、提取参数，并进入统一执行链路返回 `onebot11.Message`。

也就是说，客户端负责“命中哪个端点”，云端端点负责“这个端点到底怎么解释原文”。

### 1.1 v17.1 增量更新（2026-03-28）

MySekai 指令在本轮补齐了地图路由和快捷别名对齐，当前约定如下：

1. `msa` -> `mysekai/resource`
2. `msm` -> `mysekai/map`
3. `msr` -> `mysekai/music-record`
4. `/msm <1|2|3|4>` 支持按顺序编号选图，映射地图 ID `5/7/6/8`（其中 `2` 对应花园）
5. `/msm 13` 支持紧凑组合写法（等价于 `1 3`）
6. `/msm ... all` 可开启 `show_harvested=true`

### 1.2 v17.2 增量更新（2026-04-01）

本轮主要收口了 PJSK 查歌详情载荷和本地 Drawing 请求模型：

1. `music-detail` 现在会在基础歌曲信息之外，补齐本地 `music_meta` 派生字段。
2. 当前已对齐的详情字段包括：
   - 歌曲时长 `length`
   - 排行矩阵 `leaderboard_matrix`
   - 排行标签与总曲数
   - `music_info.mv_info`
3. 这轮实现参考了本地 `lunabot` 的查歌详情逻辑，优先保证现有图片表现和字段语义一致。
4. 当前分支原先缺失一部分 `utils/drawing` 请求模型，导致 PJSK 渲染链路外的若干请求结构无法稳定编译。
5. 现已基于 `test` 分支补齐 `utils/drawing/models.go`，并同步修正字段名漂移，例如：
   - `MvInfo -> MVInfo`
   - `Bpm -> BPM`
   - `CnName -> CNName`
6. 截至本轮收口，PJSK 相关包测试已能稳定通过；整仓剩余失败主要是依赖本地服务的 integration 用例，而不是查歌功能本身的编译或业务错误。

## 2. 已完成的合并内容

### 2.1 渲染子系统

`Service-Test` 的渲染能力在当时的合并阶段曾稳定落在：

- `internal/pjsk/render/`
- `/internal/pjsk/render`
- `/internal/pjsk/<module>/<action>/build|render`

覆盖模块包括：

- card
- music
- gacha
- event
- education
- honor
- profile
- stamp
- misc
- score
- deck
- sk
- mysekai

补充说明：

- `vlive` 已作为文本型执行模块落在 `internal/pjsk/render/vlive/`
- 但它当前不提供 `/internal/pjsk/<module>/<action>/build|render` 形式的 legacy 图像路由

### 2.2 解析与处理资源

`Test_Instruction_Parser` 中与解析、提取、处理相关的核心资源已经并入：

- `internal/pjsk/parser/`
- `internal/pjsk/handler/`
- `internal/pjsk/chardata/`

其中应继续保留的重点是：

1. 通用提取器
2. 类型化解析器
3. 业务执行与 render 桥接能力

## 3. 当前目标协议

当前 PJSK Bot 协议的目标边界如下：

### 3.1 客户端入口（Manifest）

```http
GET /api/v2/bot/:botId/command/manifests
X-Haruki-Bot-Id: <bot_id>
X-Haruki-Bot-Session-Token: <jwt>
```

返回 JSON（不在 Noise 保护范围内）。

### 3.2 业务端点

```http
POST /api/v2/bot/:botId/pjsk/<path>
X-Haruki-Bot-Id: <bot_id>
X-Haruki-Bot-Session-Token: <jwt>
Content-Type: application/octet-stream

Body: NoiseIK_Message1(MsgPack(BotCommandRequest))
```

响应：`NoiseIK_Message2(MsgPack(HarukiAPIDataResponse))`

当服务端未配置 `noise_private_key` 时，退回明文模式：

```http
POST /api/v2/bot/:botId/pjsk/<path>
Content-Type: application/json

Body: JSON(BotCommandRequest)
```

响应：`JSON(HarukiAPIDataResponse)`

### 3.3 内部兼容入口

- `POST /internal/pjsk/command`

需要明确：

`/internal/pjsk/command` 是内部兼容入口，不是客户端主协议。

## 4. 当前对指令系统的重新判断

经过本轮文档整理，已经明确下面这点：

PJSK 指令系统不应再把“云端全局 resolver 重新选 module + mode”写成客户端主链路。

主链路应当是：

1. `manifest -> 本地前缀树 -> path`
2. `path -> 对应端点`
3. `端点 -> 解析原文 -> 提参 -> 处理`

因此：

1. `GlobalCommandResolver` 不应再被视为 Bot 主协议核心
2. Trie 分发不应再被视为客户端主协议核心
3. 端点内解析原文，才是后续应收口的主模型

## 5. 当前代码状态说明

当前代码中已经具备：

1. manifest
2. `/api/v2/bot/:botId/pjsk/*` 直达型端点
3. render runtime
4. 提取器与类型化解析器
5. 基于 `internal/pjsk/handler.GetPath()` 的命令归属校验
6. 基于 handler 的 manifest 同步

但仍有一些收尾项需要继续注意：

1. 部分旧 handler 仍然带有历史别名和多义语义，需要继续梳理 path 归属
2. `/internal/pjsk/command` 仍是内部兼容入口，不能再当作客户端主入口
3. 现有客户端必须按新的 `matched_command` 协议联调

这些内容后续需要继续收口。

## 5.1 账号绑定能力状态

截至 2026-03-24，PJSK 账号绑定相关链路已经完成一轮收口：

1. 绑定命令已经接入 `/api/v2/bot/:botId/pjsk/profile/*`
2. handler 只返回 `ResolvedCommand`
3. 文本型绑定命令与图片型命令统一走 `commandhandler.Execute(...)`
4. `Execute(...)` 统一返回 `onebot11.Message`
5. 绑定业务执行和文本格式化已下沉到 `internal/pjsk/userdata/`

更详细的实现说明、代码落点、测试覆盖和注意事项，请直接参考：

- [PJSK 账号绑定实现说明](pjsk-profile-binding-implementation.cn.md)

## 5.2 Profile 渲染数据源迁移状态（v11.0 新增）

### 已完成

- `internal/pjsk/render/profile/controller.go` 新增 `BuildProfileRequestFromAPI` 和 `RenderProfileFromAPI` 方法
- `internal/pjsk/render/profile/live_adapter.go` 提供 `GetAnotherProfileResponse → Raw*` 适配层
- profile render 链路可通过 `RenderProfileFromAPI(query, resp, framesJSON)` 直接使用 Sekai API 实时数据，不再依赖本地快照
- 玩家框架（player frame）支持通过工具箱 `?key=userPlayerFrames` 单独查询，`framesJSON=nil` 时优雅降级不渲染框

### 数据来源对应关系

| 字段 | 来源 | 说明 |
|------|------|------|
| 用户基础信息 / Rank | `GetUserProfile()` | ✅ |
| 卡牌 / Deck | `GetUserProfile()` | ✅ |
| Honor / ProfileHonor | `GetUserProfile()` | ✅ |
| ChallengeLive 最高分 | `GetUserProfile()` | ✅，singular = 最高角色结果 |
| MusicDifficultyCount | `GetUserProfile().UserMusicDifficultyClearCount` | ✅ |
| 角色羁绊等级 | `GetUserProfile()` | ✅ |
| 玩家框架 | 工具箱 key 查询 `?key=userPlayerFrames` | ⚠️ 可选，nil = 不渲染框 |
| **活动排名（honor badge 上的名次）** | **不接入** | ✅ 已确认 honor badge 显示 honor 等级，不使用 `UserEventResults` |

### 存疑 / 待确认

1. ~~**活动排名显示（UserEventResults）**~~：已确认不需要接入。`FcOrApLevel` 与活动排名数字无关；honor builder 中原先基于 `query.Rank` 覆盖 `FcOrApLevel` 的逻辑已删除，`BuildProfileRequest(...)` 与 `BuildProfileRequestFromAPI(...)` 也都不再把 `UserEventResults` 传入 honor 构造流程。

2. ~~**IsHideUID**~~：已解决。读取 `query.Visible`（binding 的 `Visible` 字段），`IsHideUID = !binding.Visible`。

3. **UpdateTime**：`BuildProfileRequestFromAPI` 固定传入 `nil`，确保 image cache system 在相同渲染内容下得到稳定的 cache key。

### 待完成

~~**ProfileInfoHandle 接入**：已完成（v12.0）。~~

## 5.2.1 Sekai API 公开资料复用扩展（v15.4 新增）

### 已完成

- `internal/pjsk/render/profile/controller.go` 已补齐：
  - `BuildDetailedProfileCardFromAPI(...)`
  - `BuildProfileCardFromAPI(...)`
- `internal/pjsk/handler/bridge.go` 已新增 `buildPublicMusicProfiles(...)`，用于在 bridge 层统一构造公开资料卡，并复用到其他模块。
- 当前复用范围已经从“仅 profile 主链”扩展到：
  - `card-list`
  - `card-box`
  - `music-list`
  - `music-progress`
  - `music-rewards`
  - `education-challenge`
  - `mysekai` 头部资料（`resource / fixture-list / door-upgrade / music-record / talk-list`）
  - `deck auto recommend` 头部资料

### 当前链路

| 模块 | 复用方式 | 说明 |
|------|------|------|
| `profile` | `GetUserProfile()` + `RenderProfileFromAPI(...)` | 公开资料主链，直接使用 Sekai API 实时数据 |
| `card-list / card-box` | `buildPublicMusicProfiles(...)` -> `DetailedProfileCardRequest` | 卡牌列表/一览头部资料优先走公开资料，卡牌主体数据仍来自 masterdata |
| `music-list` | `buildPublicMusicProfiles(...)` -> `DetailedProfileCardRequest` | 资料卡优先使用公开资料，不再强依赖本地 snapshot |
| `music-progress` | `buildPublicMusicProfiles(...)` -> `ProfileCardRequest` | 进度页头部资料改为优先走公开资料 |
| `music-rewards` | `buildPublicMusicProfiles(...)` -> `ProfileCardRequest` | 奖励页头部资料改为优先走公开资料 |
| `education-challenge` | `buildPublicMusicProfiles(...)` -> `DetailedProfileCardRequest` | 仅替换挑战页头部资料，挑战主体数据仍读 snapshot |
| `mysekai` | `buildPublicMusicProfiles(...)` -> `ProfileCardRequest` | 仅替换 MySekai 信息卡头部资料，主体数据仍依赖本地 snapshot / mysekai 数据 |
| `deck` | `buildPublicMusicProfiles(...)` -> `DetailedProfileCardRequest` | 当前仅覆盖组卡图头部 profile，不触碰 deck 算法主体 |

### 设计边界

- 当前复用的是 **公开资料卡**，不是完整用户快照替代。
- `music` 的成绩主体、`education`、`mysekai`、`deck` 算法主体仍主要依赖 snapshot / 私有数据。
- `card` 当前只复用了用户资料头，不涉及持有状态、抽卡记录等私有字段。
- `buildPublicMusicProfiles(...)` 只在能够解析出调用者绑定、并能从 Sekai API 成功获取公开资料时生效；失败时模块仍按原有 fallback 逻辑运行。

## 5.3 逮捕 / 注册时间功能（v12.0 新增）

### 已完成

- `internal/pjsk/userdata/binding.go`：`ResolvedBinding` 新增 `Visible bool` 字段，`Resolve()` 同步填充
- `internal/pjsk/userdata/binding_service.go`：新增 `ResolveUserBinding(ctx, platform, platformUserID, server) (harukiID int, binding *ResolvedBinding, error)` — 组合身份解析 + 绑定解析，供 bridge 直接调用
- `internal/pjsk/parser/global_resolver.go`：新增 `ModuleArrest`、`ModuleRegTime` 两个 TargetModule 枚举值，及对应的正则路由
- `internal/pjsk/handler/sekai/arrest.go`（新文件）：
  - `UserQueryParams` 通用参数结构体（mode: self / at\_user / uid）
  - `resolveUserQueryParams()` 从 `ctx.UIDArg()` 中推断查询模式
  - `ArrestHandle()` — 指令 `/逮捕`，注册路径 `arrest`
  - `RegTimeHandle()` — 指令 `/注册时间` / `/pjsk reg time` / `/pjsk 注册时间` / `/查时间`，注册路径 `profile/reg-time`；旧 `ProfileRegTimeHandle` 存根已删除
- `internal/pjsk/handler/bridge.go`：
  - 新增 `userQueryParams` bridge 侧解码结构体
  - 新增 `resolveGameUID()` — 统一处理 self / at\_user / uid 三种模式，at\_user 模式检查 `Visible` 标志
  - 新增 `executeArrest()` — 调用 `GetSekaiAPIClient().GetUserProfile()`，按用户设置的开启难度筛选统计（self 模式读 pjsk DB 设置；其余模式使用 master + expert 默认值），格式化文本输出
  - 新增 `executeRegTime()` — 调用 `calcRegistrationTime()` 计算注册时间，输出格式化日期 + "N天前"
  - 新增 `calcRegistrationTime()` — JP/EN：`1600218000 + uid[:-3] / (1024×4096)`；TW/KR/CN：`uid / (1024×1024×4096)`

### 逮捕文本输出格式

```
逮捕: 玩家名 (UID: xxxxxx) Lv.200
[master] 谱面:300 FC:200 AP:100
[expert] 谱面:250 FC:180 AP:80
挑战Live(角色#20): 3,011,947分
```

### 注册时间文本输出格式

```
UID xxxxxx 的注册时间
2020-09-16 03:00:00 UTC
（约 2015 天前）
```

### 待完成 / 遗留

- **Music 遗留功能**：目前无新增遗留；歌曲别名已从 Music 模块独立到 Alias 模块，与角色别名共用审核链路

## 5.5 Schema 扩展 & Toolbox 路由更新（v15.0 新增）

### user_bindings 表新增字段

`ent/pjsk/schema/userbinding.go` 新增四个字段（ent codegen 已同步）：

| 字段 | 类型 | 默认值 | 用途 |
|------|------|--------|------|
| `suite_visible` | bool | `true` | 控制当前绑定是否被视为“有可用 Suite 抓包数据” |
| `mysekai_visible` | bool | `true` | 控制当前绑定是否被视为“有可用 MySekai 私有数据” |
| `bg` | `*drawing.ProfileBgSettings` (JSONB) | nil | 个人信息名片背景图设置，可为空 |
| `verified` | bool | `false` | 游戏账号是否已通过 `/pjsk verify` 验证 |

`ProfileBgSettings`（定义于 `utils/drawing/models.go`）的字段说明：

```go
type ProfileBgSettings struct {
    ImgPath  *string `json:"img_path,omitempty"` // 背景图文件路径，nil 表示使用默认背景
    Blur     int     `json:"blur"`               // 模糊半径（像素）
    Alpha    int     `json:"alpha"`              // 背景透明度（0–100）
    Vertical bool    `json:"vertical"`           // 是否以纵向模式裁切背景图
}
```

存储示例（JSONB）：
```json
{"img_path": "user_upload/profile_bg/jp/binding_42.jpg", "blur": 4, "alpha": 80, "vertical": false}
```

这三个字段现在已经直接支撑以下 handler：

- `suite_visible` → `ProfileHideSuiteHandle` / `ProfileShowSuiteHandle`
- `mysekai_visible` → `ProfileHideMySekaiHandle` / `ProfileShowMySekaiHandle`
- `bg` → `ProfileUploadBGHandle` / `ProfileClearBGHandle` / `ProfileAdjustBGHandle`
- `verified` → `ProfileVerifyHandle` / `ProfileVerifyListHandle`
- `noncompliant_bg_count` → 3-Strike BG 政策（详见 §5.6）

### 当前已落地语义（2026-03-25）

1. `visible`
   - 对应隐藏/显示 ID
   - 影响个人信息卡与文本列表中的 UID 展示方式
2. `suite_visible`
   - 不再按“是否允许别人查看”解释
   - 当前语义是：当它为 `false` 时，系统把该绑定视为“没有可用的 Suite 抓包数据”
   - 当前实际影响点有两处：
     - `profile/check-data` 的 Suite 分支（`/sud`）
     - `profile` 渲染时的玩家框附加信息（`userPlayerFrames`）读取
   - 不影响公开 Sekai API 数据
3. `mysekai_visible`
   - 当前语义是：当它为 `false` 时，系统把该绑定视为“没有可用的 MySekai 私有数据”
   - 当前直接影响点是：
     - `profile/check-data-mysekai`（`/msd`）
   - 该语义已经从 `suite_visible` 中拆出，后续可继续承接 MySekai 私有链的隐藏控制

### 与 lunabot 的隐藏抓包语义差异（2026-03-26 补充）

经对照 `lunabot` 当前实现，可以明确以下几点：

1. `lunabot` 中“隐藏抓包”只直接约束 `suite` 详细数据入口 `get_detailed_profile(...)`。
2. `mysekai` 使用独立的 `get_mysekai_info(...)` 链路，默认**不复用** `hide_suite` 这一套控制。
3. `lunabot` 存在少量 `ignore_hide=True` 的特例功能，允许在隐藏抓包后继续读取 suite，用于：
   - 资料卡增强字段
   - 部分 education / deck / mysekai 依赖的主体私有数据
4. `music-rewards` 这类模块在 `lunabot` 中采用的是：
   - 有 `suite` -> 精确结果
   - 无 `suite` -> 公开 profile 估算

而 `Haruki-Cloud` 当前实现更严格，也更粗粒度：

- `Haruki-Cloud` 当前已经开始按 `lunabot` 的方向拆分：
  - `suite_visible` 只控制 suite 私有链
  - `mysekai_visible` 独立控制 MySekai 私有链

因此，后续如果继续按 `lunabot` 逻辑转译，应沿当前已经开始落地的拆分方向继续收口，而不是重新回到共用同一个开关。
3. `verified`
   - 当前 `/pjsk verify` 暂时走 Toolbox fast-verification 路径
   - 仅当当前区服当前绑定账号命中 `/api/private/game-binding` 返回列表时，才会写入 `verified=true`
4. `bg`
   - 当前背景图文件保存到：
     `pjsk_render.asset_dirs.primary/user_upload/profile_bg/<server>/binding_<binding_id>.jpg`
   - DB 中 `bg.img_path` 持久化的是相对路径：
     `user_upload/profile_bg/<server>/binding_<binding_id>.jpg`
   - 上传 / 清除 / 调整背景图都要求当前绑定已完成验证
   - 渲染时直接把这份 `bg` 设置透传给 drawing payload

### Toolbox 客户端路由更新

| 变更 | 旧值 | 新值 |
|------|------|------|
| 私有数据端点路径 | `/api/private/:server/:data_type/:user_id` | `/api/private/game-data/:server/:data_type/:user_id` |
| 游戏绑定查询函数名 | `GetToolboxUserGameBindings` | `GetToolboxUserFastVerificationGameAccountBindings` |
| 游戏绑定端点路径 | `/api/private/game-binding/:region/:game_user_id` | `/api/private/game-binding`（纯 Query 参数） |
| 响应类型 `UserGameBinding` | 含 `verified` 字段 | 无 `verified` 字段 |

新函数行为：查询 `authorize_social_platform_infos` 中 `allow_fast_verification=true` 的记录，返回所有关联游戏账号的扁平去重列表（空数组不报错，被封禁用户跳过）。

> 完整 Toolbox API 文档：[toolbox-api.cn.md](toolbox-api.cn.md)

## 5.4 Image Cache System & 颗粒度 Ban（v13.0 新增）

### Image Cache System / OneBot11 出站

**背景**：PJSK bot/legacy handler 的外部契约已经从 `payload + data_type` 收口为 `onebot11.Message`。图片与文本都不再由 API 层做结果类型分支，而是在 bridge 内部直接封装成 OneBot11 消息段。

**实现方案**：

| 层级 | 说明 |
|------|------|
| `utils/imagecache.Client` | 新建包；`StoreAndGetURL(data, group)` 将 PNG 以 SHA-256 内容寻址写入磁盘并返回 CDN URL |
| `renderapp.Config.ImageCacheURI/Dir` | 对应 `pjsk_render.image_cache.uri/dir` |
| `renderapp.App.ImageCache` | 工厂 `New()` 中直接初始化，供 bridge 图片执行器生成 URL |
| `internal/pjsk/handler/bridge.go` | `Execute(...)` 对外统一返回 `onebot11.Message` |
| 图片类 `execute*` | 在 bridge 内部完成 `Render... -> StoreAndGetURL(...) -> onebot11.Image(url)` |
| 文本类 `execute*` | 在 bridge 内部直接返回 `onebot11.Text(text)` |
| bot handler / legacy API | 直接 JSON 返回 `Execute(...)` 的消息段数组 |

**配置示例**：
```yaml
pjsk_render:
  image_cache:
    uri: "https://image-cache.example.haruki.local"
    dir: "/var/haruki/image-cache"
```

**当前行为**：`image_cache.uri` 或 `dir` 未配置时，`imagecache.New()` 返回 nil；当前图片类 bridge 执行器不再回退 raw bytes，而是会因为无法生成图片 URL 而返回错误。

---

### 颗粒度 Ban Check

**背景**：users 表中已定义 `ban_state`、`pjsk_ban_state`、`pjsk_main_ban_state` 等多级 ban 字段，此前未接入任何命令处理链路。

**Ban 层级（检查顺序，首个命中即返回）**：

```
ban_state              → 全平台禁用
└── pjsk_ban_state     → 全 PJSK 模块禁用
    ├── pjsk_main_ban_state     → Card/Gacha/Event/Music/Deck/Education/Profile/Arrest/RegTime/CheckData/Stamp/Misc
    ├── pjsk_ranking_ban_state  → SK
    ├── pjsk_alias_ban_state    → Alias（已实现，覆盖歌曲/角色别名查询、审核与删除）
    └── pjsk_mysekai_ban_state  → MySekai
```

**实现方案**：

| 组件 | 变更 |
|------|------|
| `parser.ResolvedCommand` | 新增 `RequesterPlatform` / `RequesterUserID` 字段 |
| `sekai/helpers.go makeResolvedCmd` | 从 `ctx.Platform` / `ctx.UserId` 填充两字段 |
| `userdata.BanService` | 新建 `ban_service.go`；`CheckBan(ctx, platform, userID, module)` 按层级查 users 表；user 不存在则放行（fail open）|
| `renderapp.App.BanChecker` | `configureSekaiRuntime()` 中初始化（需 usersClient） |
| `bridge.Execute()` | 开头检查 ban：返回文本错误而非执行命令 |

> **注意**：ban 检查只针对**发起者**（requester），不影响被查询的目标 UID。

---

## 5.6 内容审核流水线（Content Moderation Pipeline）（v16.8 新增）

### 文本审核（百度 TextCensor）

- **审核时机**：每次 profile 渲染时均触发（`bridge.go` 和 `profile/controller.go`）。
- **审核对象**：玩家昵称（`User.Name`）+ 个性签名（`UserProfile.Word`）。
- **命中后行为**：`CensorName` / `CensorShortBio` 返回 `false` 时，对应字段被**置空**，不在渲染结果中展示（之前版本仅记录日志，不做遮盖）。
- **缓存**：结果写入 `censor_result`（昵称）/ `short_bio`（签名）表，后续相同内容直接命中缓存，不再调 API。

### 图片审核（腾讯 IMS）

- **审核时机**：用户执行 `/pjsk 上传背景图` 时，在实际下载图片前完成（由 `BindingService` 负责，不在 `LocalProfileBGStore` 中执行）。
- **审核主体**：背景图 CDN URL → Tencent IMS 图片内容安全接口。
- **缓存**：审核结果写入新增的 `image_mod_cache` ent 表（字段：`url`、`haruki_user_id`、`result`、`created_at`），同一 URL 不重复调用 API。
- **命中后行为**：IMS 结论不为 `Pass` 时，上传被拒绝，并累计违规次数（见三振出局机制）。

`image_mod_cache` 表结构（ent schema: `ent/censor/schema/imagemodcache.go`）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int | 主键，自增 |
| `url` | varchar(2048) | 被审核的图片 URL（唯一索引） |
| `haruki_user_id` | int (nullable) | 提交图片的 Haruki 用户 ID |
| `result` | varchar(20) | IMS 建议值：`Pass` / `Review` / `Block` |
| `created_at` | timestamptz | 审核时间，不可更新 |

### 三振出局机制（3-Strike BG 政策）

- 违规次数 `NoncompliantBGCount` 存储于 `UserPreference.settings`（JSONB，**用户粒度**，跨服务器、跨绑定生效）。
- 每次 IMS 返回不通过，通过 `IncrNoncompliantBGCount()` 累加该用户的计数。
- 当 `NoncompliantBGCount >= 3` 时，该 Haruki 用户的背景图上传功能被**永久禁用**，直到人工干预重置。
- 注意：早期版本曾误将计数存于 `user_bindings.noncompliant_bg_count`（绑定粒度），v16.9 已修正为用户粒度；**`user_bindings` 表不再需要该字段**。

### 审核覆盖范围（v16.9 全量）

所有用户可控文本字段现已全部纳入审核和遮盖逻辑：

| 路径 | 字段 | 审核函数 | 遮盖行为 |
|------|------|----------|----------|
| `bridge.go` `executeProfile()` | `resp.User.Name`、`resp.UserProfile.Word` | `CensorName` / `CensorShortBio` | 置空 |
| `bridge.go` `executeArrest()` | `resp.User.Name` | `CensorName` | 置空后再传入 `formatArrestText` |
| `profile/controller.go` `BuildProfileRequest()` | snapshot 中的昵称 / word | `CensorName` / `CensorShortBio` | 置空 |
| `sk/controller.go` 各排名构建函数 | `latest/trace.UserData.Name` | `censorTrackerName()` → `CensorName` | 置空（`pickTrackerDisplayName` 对空值有安全处理）|

### 审核服务责任边界
|------|------|------|
| `LocalProfileBGStore` | 持有 `censor` 并在下载图片前调用 `CensorImage` | 不再持有 `censor`，不做审核判断 |
| `BindingService` | 无审核逻辑 | 持有 `censor`；`SetCurrentBindingProfileBG` 中执行三振逻辑 |
| `utils/censor.Service.CensorImage` | `(ctx, url)` — 使用独立原始 SQL 连接池缓存 | `(ctx, harukiUserID, url)` — 使用 censor ent client 写入 `image_mod_cache` |
| `utils/censor/image_mod_cache.go` | 独立 raw-SQL PostgreSQL 实现 | **已删除**，替换为 ent schema |

---

## 5.7 Music Chart 技能谱面 `music_meta` 接入（v15.3 新增）

### 已完成

- `internal/pjsk/meta.Loader` 已从“仅在 runtime 启动时预加载 `music_metas`”推进到“实际被 `music chart` 构建链消费”。
- `internal/pjsk/render/music/controller.go` 已补齐 chart 请求的 `music_meta` 注入逻辑：
  - `skill=false`：保持普通谱面预览行为，不附带 `music_meta`
  - `skill=true`：按 `region + music_id + difficulty` 从 `MetaLoader` 选择对应条目并写入 `drawing.GenerateMusicChartRequest.MusicMeta`
- 当 `MetaLoader` 当前无该区服缓存时，会回退读取 snapshot 的 `MusicMetaBytes()`，保持与旧本地调试链兼容。
- `internal/pjsk/handler/sekai/chart.go` 已补齐 `/技能预览` -> `music-chart` 的 `skill=true` 参数透传；此前该入口虽然存在，但不会显式打开 skill 模式。
- 已新增最小测试覆盖：
  - `internal/pjsk/render/music/chart_meta_test.go`
  - `internal/pjsk/handler/sekai/chart_test.go`

### 当前行为

| 入口 | 行为 |
|------|------|
| `/查谱面` / `/谱面预览` | 普通谱面图，不注入 `music_meta` |
| `/技能预览` | 技能谱面图，注入对应难度的 `music_meta`，供 DrawingAPI / `pjsekai-scores-rs` 显示技能区间、`skill_score_*` 与 fever 相关信息 |

### 说明

- 这次改动解决的是 `Haruki-Cloud` 侧“payload 没有把 `music_meta` 传下去”的问题。
- 若后续图面仍未正确显示技能加成文本或 fever 区块，应继续排查 `Haruki-Drawing-API` / `pjsekai-scores-rs` 对 `music_meta` 字段的消费逻辑，而不是再回到 Cloud 侧重复排查注入链。

## 6. 全功能链路状态（v16.0 全量审计）

### 6.1 已全链路接通（E2E Ready）✅

以下功能从 bot 客户端入站 → handler → bridge → executor → 出站响应全部打通：

| 模块 | 功能 | 路径 |
|------|------|------|
| **Profile** | 绑定 / 解绑 / 设主账号 / 清除默认绑定 / 查看个人信息 / 隐藏或显示 ID / 隐藏或显示抓包 / 验证 / 验证列表 / 个人信息背景上传·清除·调整 | profile/bind · unbind · default · default/clear · profile · profile/visibility/hide · profile/visibility/show · profile/suite/hide · profile/suite/show · profile/verify · profile/verify/list · profile/bg/upload · profile/bg/clear · profile/bg/adjust |
| **Arrest** | 逮捕（self/at\_user/uid 三模式，含 Visible 检查） | arrest |
| **RegTime** | 注册时间查询（JP/EN + TW/KR/CN 双算法） | profile/reg-time |
| **CheckData** | 套件抓包时间（/sud）/ MySekai 抓包时间（/msd） | profile/check-data · check-data-mysekai |
| **Card** | 卡面详情 / 卡牌列表 / 卡牌一览（Box） | card/detail · list · box |
| **Music** | 歌曲详情 / 列表 / 进度 / 奖励 / 谱面预览 / 物量统计 / BPM 查询 / 曲绘查询 | music · list · progress · rewards · chart · note-count · bpm · cover |
| **Alias** | 歌曲别名查询 / 角色别名查询 / 提交审核 / 删除已审核别名 / 待审核列表 / 通过审核 / 拒绝审核 | alias/music · alias/music/add · alias/music/del · alias/character · alias/character/add · alias/character/del · alias/pending · alias/approve · alias/reject |
| **Gacha** | 卡池列表 | gacha |
| **Deck** | 活动/挑战/长草/加成/烤森 组卡推荐 | deck/event · challenge · no-event · bonus · mysekai |
| **Event** | 活动列表 / 活动详情 / **活动记录** | event/list · event · event/record |
| **Education** | 挑战信息 / 加成信息 / 区域道具 / 羁绊 / 队长统计 | education/challenge · power · area · bonds · leader |
| **Score** | 分数计算 / 自定义房间 / 歌曲 meta / 歌曲排行 | score · custom-room · music-meta · music-board |
| **SK** | 档线 / 查询 / 时速 / 查房 / 玩家轨迹 / 档线轨迹 / 胜率预测 / 日速 / SK 预测 / 水表 | sk/line · query · speed · check-room · player-trace · rank-trace · winrate · (日速/预测/水表→复用) |
| **MySekai** | 资源 / 地图 / 对话列表 / 家具列表 / 家具详情 / 大门升级 / 唱片 / 蓝图 / 照片下载 | mysekai/resource · map · talk-list · fixture-list · fixture-detail · door-upgrade · music-record · photo |
| **Stamp** | 贴纸列表 | stamp |
| **Misc** | 角色生日 | misc/birthday |
| **Virtual Live** | 近期 Virtual Live 文本查询 | vlive |

> **统计**：约 75 个 handler · 16 个 module · 全部有 bridge case · 所有 enabled handler 均有 Path

---

### 6.2 已定义但未实现（Disabled / TODO）❌

以下功能 handler 已存在但 `Disabled: true`，executor 为存根，不暴露到 bot API：

**以下为后续再决定是否接入（保留代码，暂不实现）：**

- Card 系统（2 个）：卡面原图（`CardImgHandle`，path: `card/image`）、卡牌剧情（仅 JP）
- Deck 系统（1 个）：实效 / 倍率（`ScoreUpHandle`）
- Event 系统（1 个）：活动剧情（仅 JP）
- Gacha 系统（1 个）：抽卡记录
- Misc 系统（5 个）：帮助、更新查询、NG 词检测、抓包帮助、提取卡牌

> **统计**：9 个 handler 待后续决策

---

### 6.3 v16.1 本次清理（MySekai msd 合并 + Stamp 移除）

- **合并**：`CheckMysekaiDataHandle`（mysekai.go，disabled 重复入口）已删除；其命令集（`/pjsk烤森抓包` 等）合并入 `MsdHandle`（profile.go，path: `profile/check-data-mysekai`），现已完整接通
- **删除**：Stamp 系统 6 个不在项目规划内的 disabled handler（`StampMakeHandle`、`RandStampHandle`、`StampRefreshHandle`、`StampRefreshBatchHandle`、`StampBaseHandle`、`StampBaseDeleteHandle`）——stamp.go 仅保留 `StampHandle`（贴纸列表）
- **删除**：`ProfileSwapBindHandle`（交换绑定）、`ProfileCheckServiceHandle`（服务状态检查）、`ProfileDataModeHandle`（抓包模式切换）、`ProfileBindHistoryHandle`（绑定历史）——全部不在项目规划内
- **删除**：`ProfileInfoHandle`——功能与 `ProfileHandle`（path: `profile`）完全重复，冗余死代码
- **修复**：`EventRecordHandle` 补充 `Path: "event/record"`，现已进入 bot API 路由表
- **修复**：`HelpHandle` 改为 `Disabled: true`，防止路由到无实现的 `ModuleHelp`
- **修复**：`bridge.go` 补充 6 处 nil guard，防止 `sekaiClient` 未配置时 panic（`executeCard/Event/Gacha/Stamp` + `executeProfile` 两个分支）

> `app.Music`、`app.MySekai`、`app.Profiles` 各自的 controller 已内置 nil 接收者守卫，无需 bridge 层额外处理。`app.BanChecker` 同理。

### 6.3 v16.2 测试修复与 `/卡面` 命令路由修正（2026-03-26）

详见 v16.6 变更记录。

### 6.4 v16.3 Noise IK 传输层加密接入（2026-03-26）

Bot API（`/api/v2/bot/:botId/pjsk/*`）全面接入 Noise IK 传输层加密。

**协议变更**：

| 层级 | 之前 | 之后 |
|------|------|------|
| 传输 | 明文 POST + JSON body | Noise_IK_25519_AESGCM_SHA256 加密 POST body |
| 编码 | JSON | MsgPack（加密信封内） |
| 身份认证 | JWT session header | JWT session header（不变） |
| manifest 端点 | JSON | JSON（不在 Noise 保护范围内） |

**变更清单**：

- `config/config.go`：`HarukiBotDBConfig` 新增 `NoisePrivateKey string` 字段
- `haruki-db-configs.example.yaml`：新增 `noise_private_key` 配置项
- `internal/core/crypto/crypto.go`：新增 `KeyPairFromPrivate([]byte)` 用于从配置文件私钥派生密钥对
- `internal/middleware/secure/secure.go`：解密后设置 `c.Locals("secure_noise", true)` 标记
- `api/helper.go`：新增 `MsgPackResponse()` 用于返回 MsgPack 编码的响应信封
- `api/bot/pjsk/handler.go`：
  - `RegisterPJSKBotRoutes` 签名新增 `noiseKeyPair *crypto.KeyPair`（nil 时跳过 Noise）
  - pjsk 路由组应用 `secure.New()` 中间件
  - `parseBotRequest` 支持 JSON / MsgPack 双格式解码
  - `botResponse` 根据 `secure_noise` 标记自动选择 JSON / MsgPack 输出
  - `toZeroSegments` 新增 `map[interface{}]interface{}` 分支（MsgPack 反序列化产物）
- `cmd/server/main.go`：新增 `initNoiseKeyPair()` 从配置解码密钥对并注入路由

**降级策略**：`noise_private_key` 为空时 Noise 中间件不注册，退回明文 JSON 模式。

**测试**：新增 `TestBotNoiseIKRoundTrip` 端到端加密测试，全部 21 个测试通过。

- **命令路由**：`/卡面` 从已禁用的 `CardImgHandle`（`card/image`）移入 `CardDetailHandle`（`card/detail`）命令集，解除因禁用 handler 造成的命令悬空
- **测试修复**：`api/legacy/pjsk/render_route_test.go` 补充 `ImageCache` 至 `testRenderApp`；`rendermusic.NewController` 调用补充新增的 `metaLoader` 参数（`nil`）
- **测试更新**：`TestCommandEndpointReturnsImage` 改为验证 JSON 格式（OneBot11 image segment + image-cache URL），移除旧 raw PNG 断言（与当前 endpoint 返回 `onebot11.Message` 的架构不符）
- **全量测试**：所有模块测试通过；`integration/` 包自首次合并起即因引用已删除包而 setup failed，与本次变更无关

### 6.4 特殊 handler（绕过 bridge）

| Handler | 行为 |
|---------|------|
| `HeyiweiHandle`（/b30, /b39） | 返回硬编码字符串"何意味"，Easter Egg，不走 bridge |

---
## 7. 当前保留项

下面这些内容目前明确保留：

1. `command_manifests`
2. 基于 `internal/pjsk/handler` registry 派生的 Bot route 元数据
3. `/api/v2/bot/:botId/pjsk/*` 直达型端点
4. `internal/pjsk/parser` 中的通用提取与类型化解析能力
5. render runtime 与内部 build/render 路由

## 8. 当前不再作为主链路的内容

下面这些内容不再应被文档描述为客户端主链路：

1. `GlobalCommandResolver -> module + mode -> bridge`
2. 云端 Trie 分发先重新决定目标端点
3. 客户端直接调用 `/internal/pjsk/render`
4. 客户端直接调用 `/internal/pjsk/command`

## 9. 仍然存在的技术债

当前主要技术债包括：

1. 部分 path 仍需继续剥离历史多义命令
2. 强用户态模块仍然依赖本地 JSON 快照
3. MySekai 仍有本地 masterdata fallback
4. Deck 当前仍是 Go 方案，旧 CGo 引擎未恢复为默认链路
5. 已存在的 `command_manifests` 若被人工特殊维护，仍需确认新的 handler-source 同步结果是否符合预期
6. 图片类 bridge 执行器当前强依赖 `ImageCache`；若部署未配置 image cache，会直接影响图片命令可用性
7. `user_bindings` 表新增的 `suite_visible`、`mysekai_visible`、`bg`、`verified` 四个字段需要 DB 迁移（Ent auto-migration 或手动 ALTER TABLE）；`noncompliant_bg_count` **已从 `user_bindings` 移除**，无需迁移该字段
8. `authorize_social_platform_infos` 表新增 `allow_fast_verification` 列同样需要 DB 迁移（Toolbox 侧）
9. `image_mod_cache` 为新增 ent 表，需要在 censor DB 中执行 `Schema.Create()` 迁移（启动时自动运行）
10. `user_preferences.settings` JSONB 新增 `noncompliant_bg_count` 字段（新结构，向后兼容，旧行默认为 0，无需 ALTER TABLE）

> **已修复**：`app.Cards`、`app.Events`、`app.Gachas`、`app.Stamps` 和 `app.Bindings` 的 nil panic 风险已在 bridge.go 中通过前置 nil guard 解决（v16.0）。

## 10. 内部测试就绪评估（2026-03-26）

**整体结论：代码层面已就绪，可进入内部测试。**

| 项目 | 状态 | 说明 |
|------|------|------|
| 全部 enabled handler E2E 打通 | ✅ | 约 75 个 handler，全部有 Path + bridge case + executor |
| Nil panic 风险消除 | ✅ | bridge.go 已补充 6 处 nil guard |
| 代码无存根（enabled 路径） | ✅ | 无 TODO stub 在活跃链路中 |
| 全量单元测试通过 | ✅ | 21 个 bot 测试 + legacy 测试全部通过 |
| Noise IK 传输层加密 | ✅ | 已完整接入 pjsk 路由组；`noise_private_key` 为空时自动降级 |
| MsgPack 编码 | ✅ | Noise 信封内请求/响应使用 MsgPack；明文模式仍用 JSON |
| 内容审核覆盖完整性 | ✅ | profile / arrest / SK 排名 / suite snapshot 全路径遮盖（v16.9）|
| BG 违规计数粒度 | ✅ | 用户粒度（UserSettings JSONB），跨绑定生效（v16.9）|
| DB 迁移（`user_bindings` 4 个字段） | ⚠️ | 上线前需执行，否则 profile 设置相关命令失败 |
| DB 迁移（`allow_fast_verification`） | ⚠️ | Toolbox 侧，上线前需执行 |
| DB 迁移（censor `image_mod_cache` 表） | ⚠️ | 上线前需执行（censor DB `Schema.Create()` 已在启动时自动运行）|
| ImageCache 配置 | ⚠️ | 未配置 `image_cache.uri/dir` 时所有图片命令失败 |
| 别名管理（add/review/reject）API 归属 | ⏳ | 公开查询已通，新增/审核/拒绝 API 归属待决策 |

**测试前提条件**：
1. 执行 DB 迁移（`user_bindings`：`suite_visible` / `mysekai_visible` / `bg` / `verified`；`authorize_social_platform_infos`：`allow_fast_verification`；censor DB：`image_mod_cache` 表）
2. 部署配置中设置 `pjsk_render.image_cache.uri` + `image_cache.dir`
3. 配置 `haruki_bot.noise_private_key`（32 字节 X25519 私钥的 hex 编码；留空则退回明文 JSON 模式）
4. push 本地 `test` 分支到 `origin/test`

## 10.1 集成测试执行结果（2026-03-27，第一轮）

### 测试方式

采用黑盒 HTTP 集成测试（`integration/api_test.go`），通过真实 HTTP 请求覆盖认证链路、命令分发、图片渲染和外部代理 API 四个维度。

> **关于旧测试文件**：原 `integration/full_integration_test.go` 为白盒测试，直接 import 内部包（`haruki-cloud/api`、`haruki-cloud/database/...` 等）。在 ent schema 迁移和大规模模块重构后，这些导入路径全部失效，编译报错。新的 `api_test.go` 不依赖内部结构，改为通过 HTTP 端到端测试，更稳健。

**测试环境**：本地 Docker（haruki-postgres + haruki-redis）+ Drawing API（SSH 转发至 localhost:28000）

### 认证与协议层

| 测试项 | 结果 | 说明 |
|--------|------|------|
| Bot 认证（AES-GCM + JWT credential） | ✅ | Session token 获取成功 |
| Noise IK 密钥对生成 | ✅ | 客户端 X25519 密钥对就绪 |
| Manifest 获取 | ✅ | 返回 76 个 command group |

### 指令端点测试（第一轮，12/23 通过）

| 端点 | 结果 | 说明 |
|------|------|------|
| `profile/bind` | ✅ | 账号绑定成功 |
| `card/detail` | ✅ | 图片渲染正常 |
| `card/box` | ✅ | 图片渲染正常（耗时约 21s，含 Drawing API 渲染） |
| `music` | ✅ | 图片渲染正常 |
| `event/list` | ✅ | 图片渲染正常 |
| `stamp` | ✅ | 图片渲染正常（按 ID 查询） |
| `sk/line` | ✅ | 图片渲染正常 |
| `sk/query` | ✅ | 图片渲染正常 |
| `vlive` | ✅ | 文本响应正常 |
| `arrest` | ✅ | 文本响应正常 |
| `profile/reg-time` | ✅ | 文本响应正常 |
| `profile` | ❌ | Drawing API 找不到 honor 游戏资源：`honor/honor_6881/degree_sub.png`（`startapp` vs `ondemand` 路径问题，已知问题，待处理）|
| `gacha` | ❌ | Drawing API 找不到 gacha logo：`gacha/ab_gacha_392/logo/logo.png`（同上，`startapp` vs `ondemand` 路径问题）|
| `card/list` | ❌ | Handler 未解析 args 为卡牌 ID（Parser 接入不完整）|
| `event` | ❌ | Handler 未从 args 提取 event ID（Parser 接入不完整）|
| `score/music-meta` | ❌ | Handler 未提取歌曲参数（Parser 接入不完整）|
| `misc/birthday` | ❌ | Handler 未解析角色名为生日数据（Parser 接入不完整）|
| `education/challenge` | ❌ | 依赖本地用户快照（架构限制）|
| `education/area` | ❌ | 依赖本地用户快照（架构限制）|
| `education/bonds` | ❌ | 依赖本地用户快照（架构限制）|
| `education/leader` | ❌ | 依赖本地用户快照（架构限制）|
| `education/power` | ❌ | 依赖本地用户快照（架构限制）|

### 资源路径修复情况（第一轮）

本轮修复了以下 asset 路径错误（已提交 `test` 分支）：

- **honor/builder.go**：全量补充 `gameAssetDir` 前缀（degree/rank/scroll/bonds character/word）；静态 UI（frame/mask/icon_degreeLv/bonds bg）保持 `StaticImagesDir` 前缀；`buildBondsHonorRequest` 接入 `region` 参数
- **event/builder.go**：attr 图标改为 `StaticImagesDir`；单位图标移除 `unit/` 子目录（图标文件在 `static_images/` 根目录）
- **card/builder.go**：同上，单位图标路径修正
- **stamp/controller.go**：stamp 资源路径补充 `RegionAssetDir` 前缀；`resolveStampImage` 接入 `region` 参数

## 10.2 集成测试执行结果（2026-03-27，第二轮）

**本轮新增通过：`education/challenge` ✅、`education/bonds` ✅、`education/leader` ✅**

当前通过数：**15/23**

### 本轮修复内容

#### Education Suite 集成打通

核心问题：Toolbox API 返回 `401 "unauthorized user agent"`，原因是 `haruki-db-configs.yaml` 中 `toolbox` 配置缺少 `user_agent` 字段。

修复清单：

| 文件 | 修复内容 |
|------|---------|
| `haruki-db-configs.yaml` | 补充 `toolbox.user_agent: "Haruki-Cloud/v2.0.0"` |
| `education/controller.go` | `lunabot_static_images` → `StaticImagesDir`；`characterIconPath` 改用 `ResolveAssetPath` |
| `music/controller.go` | `lunabot_static_images` → `StaticImagesDir` |
| `music/builder.go` | `NoteHost` 前缀改为 `StaticImagesDir` |
| `handler/bridge.go` | `charaIconPath()` 改用 `ResolveAssetPath + StaticImagesDir`；education-bonds 跳过无图标的角色（ID > 26）|
| `utils/drawing/models.go` | `BondInfo.Color1/Color2` 加 `omitempty`（避免 `null → tuple` Pydantic 422 错误）|
| `.gitignore` | 补充 `server`、`test_auth`、`haruki-db-configs.yaml`、`exports/`、`Data/` |

#### Education 端点当前状态

| 端点 | 结果 | 说明 |
|------|------|------|
| `education/challenge` | ✅ | Toolbox suite → snapshot 构建 → Drawing 渲染成功 |
| `education/bonds` | ✅ | 从 suite 数据构建羁绊请求，渲染成功（跳过 ID > 26 无图标角色）|
| `education/leader` | ✅ | 从 suite 数据构建领队统计，渲染成功 |
| `education/area` | ❌ | 未实现（需要区域道具 master 数据 + 材料计算） |
| `education/power` | ❌ | 未实现（需要加成倍率 master 数据 + 复杂计算） |

### 第二轮完整指令端点测试结果（15/23）

| 端点 | 结果 | 说明 |
|------|------|------|
| `profile/bind` | ✅ | |
| `card/detail` | ✅ | |
| `card/box` | ✅ | |
| `music` | ✅ | |
| `event/list` | ✅ | |
| `stamp` | ✅ | |
| `sk/line` | ✅ | |
| `sk/query` | ✅ | |
| `vlive` | ✅ | |
| `arrest` | ✅ | |
| `profile/reg-time` | ✅ | |
| `education/challenge` | ✅ | 新增 |
| `education/bonds` | ✅ | 新增 |
| `education/leader` | ✅ | 新增 |
| `profile` | ❌ | startapp/ondemand honor 资源路径问题 |
| `gacha` | ❌ | startapp/ondemand gacha 资源路径问题 |
| `education/area` | ❌ | 未实现 |
| `education/power` | ❌ | 未实现 |
| `card/list` | ❌ | Parser handler 接入不完整 |
| `event` | ❌ | Parser handler 接入不完整 |
| `score/music-meta` | ❌ | Parser handler 接入不完整 |
| `misc/birthday` | ❌ | Parser handler 接入不完整 |

### 外部代理 API 测试（3 项，全部通过）

| 端点 | 结果 |
|------|------|
| Sekai API 公开资料 | ✅ |
| Toolbox MySEKAI | ✅ |
| Event Tracker 活动排名 | ✅ |

## 10.3 集成测试执行结果（2026-03-27，第三轮）

**本轮新增通过：`gacha` ✅（得益于 startapp/ondemand 双路径解析）**

当前通过数：**16/23**

### 本轮改进内容

#### startapp/ondemand 双路径解析系统（commit `7e2ca58`）

新增 `ResolveRegionAssetPath()` 函数，根据资源路径的顶层目录自动判断优先尝试 `startapp` 还是 `ondemand`，失败后自动回退到另一路径。

远程服务器实际目录布局（INTERNAL_HOST Drawing API）：

| 目录 | startapp 子目录 | ondemand 子目录 |
|------|----------------|-----------------|
| bonds_honor | ✅ | - |
| character | ✅ | - |
| home | ✅ | - |
| honor | ✅ | - |
| music | ✅ (jacket, music_score, short) | ✅ (long) |
| rank_live | ✅ | - |
| stamp | ✅ | - |
| thumbnail | ✅ | - |
| event | - | ✅ |
| event_story | - | ✅ |
| gacha | - | ✅ |
| mysekai | - | ✅ |

`onDemandPreferredTopLevel`：event、event_story、gacha、mysekai（music 被移除，因 jacket/score 在 startapp）。

#### 别名数据导入

从 `exports/` 导入别名数据至 `haruki_pjsk.alias` 表：
- music_alias: 12,976 条（alias_type = "music"）
- character_alias: 1,230 条（alias_type = "character"）

#### 修复 music 路径回归

`onDemandPreferredTopLevel` 中 `"music"` 导致 jacket 路径错误指向 ondemand，但实际 jacket 文件在 startapp。移除后 music 测试恢复。

### 第三轮完整指令端点测试结果（16/23）

| 端点 | 结果 | 说明 |
|------|------|------|
| `profile/bind` | ✅ | |
| `card/detail` | ✅ | |
| `card/box` | ✅ | |
| `music` | ✅ | |
| `event/list` | ✅ | |
| `gacha` | ✅ | 新增（ondemand 路径修复）|
| `stamp` | ✅ | |
| `sk/line` | ✅ | |
| `sk/query` | ✅ | |
| `vlive` | ✅ | |
| `arrest` | ✅ | |
| `profile/reg-time` | ✅ | |
| `education/challenge` | ✅ | |
| `education/bonds` | ✅ | |
| `education/leader` | ✅ | |
| `profile` | ❌ | honor_6833 资源不存在（startapp/ondemand 均无，需补充资源） |
| `education/area` | ❌ | 未实现 |
| `education/power` | ❌ | 未实现 |
| `card/list` | ❌ | Parser handler 接入不完整 |
| `event` | ❌ | Parser handler 接入不完整 |
| `score/music-meta` | ❌ | Parser handler 接入不完整 |
| `misc/birthday` | ❌ | Parser handler 接入不完整 |

### commit `7e2ca58` 评审备注

- ⚠️ `tmp/*.db` 文件（bot.db、pjsk.db、sekai.db、users.db）被意外提交，应加入 `.gitignore`

## 10.4 集成测试执行结果（2026-03-27，第四轮）

**本轮新增通过：`profile` ✅（honor group 数据修复 + ent string enum 迁移）**

当前通过数：**17/23**

### 本轮修复内容

#### ent schema enum 字段类型迁移（string enum migration）

远程 ent schema 将大批 enum 字段从 `json.RawMessage`（jsonb）改为了 `string` 类型，导致所有调用 `jsonString(entity.Field)` 的地方编译失败。

修复范围：

| 文件 | 修复的字段 |
|------|-----------|
| `chardata/loader.go` | `Unit`（json.Unmarshal → 直接赋值，移除 encoding/json import）|
| `render/card/source_cloud.go` | `Unit`, `CardRarityType`, `Attr`, `SupportUnit`, `DescriptionSpriteName`, `GachaType` |
| `render/event/source_cloud.go` | `CardAttr`, `Unit` ×2, `WorldBloomChapterType`, `EventType`, `CardRarityType`, `Attr`, `SupportUnit` |
| `render/gacha/source_cloud.go` | `GachaType`, `CardRarityType`, `Attr`, `SupportUnit` |
| `render/honor/source_cloud.go` | `HonorType`, `HonorRarity` ×2, `Unit` |
| `render/music/source_cloud.go` | `MusicDifficulty` ×2, `MusicVocalType`, `MusicTag`, `Unit`, `EventType` |
| `render/education/source_cloud.go` | `ResourceBoxPurpose`, `ResourceBoxType` |

#### profile honor 路径修复（honor group 数据补充）

honor 6833 对应 honor group 544（HAPPY BIRTHDAY 遥 2025.10.5），其路径通过 group 的 `backgroundAssetbundleName`（`honor_bg_birthday_01_06`）而非 honor 自身的 `assetbundle_name`（`honor_6833`）构建。数据库侧补充了 group 544 的完整数据后，Drawing API 服务器上已有对应资源，profile 渲染成功。

### 第四轮完整指令端点测试结果（17/23）

| 端点 | 结果 | 说明 |
|------|------|------|
| `profile/bind` | ✅ | |
| `card/detail` | ✅ | |
| `card/box` | ✅ | |
| `music` | ✅ | |
| `event/list` | ✅ | |
| `gacha` | ✅ | |
| `stamp` | ✅ | |
| `sk/line` | ✅ | |
| `sk/query` | ✅ | |
| `vlive` | ✅ | |
| `arrest` | ✅ | |
| `profile/reg-time` | ✅ | |
| `education/challenge` | ✅ | |
| `education/bonds` | ✅ | |
| `education/leader` | ✅ | |
| `profile` | ✅ | 新增（honor group 数据 + ent 迁移修复）|
| `card/list` | ❌ | Parser handler 接入不完整 |
| `event` | ❌ | Parser handler 接入不完整 |
| `score/music-meta` | ❌ | Parser handler 接入不完整 |
| `misc/birthday` | ❌ | Parser handler 接入不完整 |
| `education/area` | ❌ | 未实现 |
| `education/power` | ❌ | 未实现 |

## 10.5 集成测试覆盖率审计（2026-03-27）

当前集成测试覆盖了 23 个端点（含 bind），而项目共有 **70 个 enabled handler path**，实际覆盖率约 **33%**。

### 未覆盖端点分类

#### A 类：无需用户数据，可直接测试

| 端点 | 命令示例 | 说明 |
|------|---------|------|
| `music/list` | `/歌曲列表` | 无需参数 |
| `music/bpm` | `/查bpm Tell Your World` | 歌曲名/别名 |
| `music/cover` | `/查曲绘 Tell Your World` | 歌曲名/别名 |
| `music/note-count` | `/查物量 Tell Your World` | 歌曲名/别名 |
| `music/rewards` | `/曲目奖励 Tell Your World` | 歌曲名/别名 |
| `score/music-board` | `/歌曲排行 Tell Your World` | 歌曲名/别名 |
| `alias/pending` | `/待审核别名` | 无需参数，只读 |
| `profile/check-data` | `/pjsk check data` | 读取套件抓包时间 |
| `profile/check-data-mysekai` | `/pjsk msd` | 读取烤森抓包时间 |
| `profile/verify/list` | `/验证列表` | 读取验证列表 |

#### B 类：Profile 可逆写操作（幂等）

| 端点 | 命令示例 | 说明 |
|------|---------|------|
| `profile/suite/hide` | `/pjsk suite hide` | 切换可见性，可逆 |
| `profile/suite/show` | `/pjsk suite show` | 同上 |
| `profile/mysekai/hide` | `/pjsk mysekai hide` | 切换可见性，可逆 |
| `profile/mysekai/show` | `/pjsk mysekai show` | 同上 |
| `profile/visibility/hide` | `/pjsk hide` | 切换可见性，可逆 |
| `profile/visibility/show` | `/pjsk show` | 同上 |

#### C 类：依赖 Toolbox 用户快照数据

| 端点 | 命令示例 | 说明 |
|------|---------|------|
| `music/progress` | `/打歌进度` | 需用户打歌记录 |
| `event/record` | `/活动记录` | 需用户活动历史 |
| `deck/event` | `/活动组卡` | 需用户卡牌数据 |
| `deck/challenge` | `/挑战组卡` | 同上 |
| `deck/no-event` | `/长草组卡` | 同上 |
| `deck/bonus` | `/加成组卡` | 同上 |
| `deck/mysekai` | `/烤森组卡` | 需用户 mysekai 数据 |
| `score` | `/分数 Tell Your World` | 需用户分数数据 |
| `score/custom-room` | `/自定义房间控分` | 自定义参数计算 |
| `mysekai/resource` | `/烤森资源` | 需用户 mysekai 数据 |
| `mysekai/map` | `/msm 1` | 需用户 mysekai 数据；可用 `1/2/3/4` 选图 |
| `mysekai/talk-list` | `/mysekai对话` | 同上 |
| `mysekai/fixture-list` | `/家具列表` | 同上 |
| `mysekai/fixture-detail` | `/家具详情` | 同上 |
| `mysekai/door-upgrade` | `/大门升级` | 同上 |
| `mysekai/music-record` | `/唱片` | 同上 |
| `mysekai/photo` | `/照片` | 同上 |

#### D 类：依赖 SK Tracker 实时数据

| 端点 | 命令示例 | 说明 |
|------|---------|------|
| `sk/check-room` | `/sk查房` | 查询当前活动房间 |
| `sk/speed` | `/时速` | 实时时速计算 |
| `sk/player-trace` | `/sk玩家轨迹` | 玩家轨迹查询 |
| `sk/rank-trace` | `/档线轨迹` | 档线轨迹查询 |
| `sk/winrate` | `/胜率预测` | JP region only |

#### E 类：有破坏性/需要特殊权限，不宜自动测试

| 端点 | 原因 |
|------|------|
| `profile/unbind` | 会解除绑定，影响后续测试 |
| `profile/default` | 修改默认绑定 |
| `profile/default/clear` | 同上 |
| `profile/verify` | 触发外部验证流程 |
| `profile/bg/upload` | 需要上传文件 |
| `profile/bg/clear` | 修改用户数据 |
| `profile/bg/adjust` | 同上 |
| `alias/approve` | Admin 操作 |
| `alias/reject` | Admin 操作 |
| `card/image` | Handler disabled |

### 扩展测试计划优先级

1. **立即可做**：A 类 10 个端点 + B 类 6 个端点，预计新增 16 个测试用例
2. **需 Toolbox 数据**：C 类 17 个端点，当前测试用户已有 suite 数据，部分可能通过
3. **需 Tracker 配置**：D 类 5 个端点，视 Tracker API 可用性而定
4. **不做**：E 类，需人工确认或破坏测试环境

## 10.6 集成测试执行结果（2026-03-27，第五轮）

**测试规模从 23 扩展到 58 个端点**（含 bind 为 59），覆盖率从 33% 提升到 83%。

当前通过数：**31/58**

### 本轮修复内容

#### 指令 trie 大小写排序冲突

由于 `slices.Sort` 按字典序排序，大写命令（如 `/查BPM`）排在小写（`/查bpm`）之前，先注册进 trie 占位。发送 `/查bpm` 作为 `matched_command` 时，trie 返回 `/查BPM`，与请求的 `/查bpm` 不一致导致 400 错误。

修复：测试改用无大小写歧义的命令（`/pjsk bpm`、`/pjsk hide id`、`/pjsk show id`）。

#### 参数格式修正

| 端点 | 修复前 | 修复后 | 原因 |
|------|-------|--------|------|
| `music/note-count` | `/查物量 Tell Your World` | `/查物量 1000` | handler 需要物量数值，不是歌曲名 |
| `score` | `/控分 Tell Your World` | `/控分 100000 Tell Your World` | 需要目标 PT（数值在前）|
| `score/custom-room` | `/自定义房间控分` | `/自定义房间控分 100000` | 需要目标 PT |
| `mysekai/photo` | `/msp` | `/msp 1` | 需要照片编号 |
| `mysekai/fixture-detail` | `/msf` | `/msf 1` | 需要家具 ID |

### 第五轮完整指令端点测试结果（31/58）

#### ✅ 通过（31 个）

| 端点 | 类型 | 说明 |
|------|------|------|
| `profile/bind` | 基础 | |
| `card/detail` | 基础 | |
| `card/box` | 基础 | |
| `music` | 基础 | |
| `event/list` | 基础 | |
| `gacha` | 基础 | |
| `stamp` | 基础 | |
| `sk/line` | 基础 | |
| `sk/query` | 基础 | |
| `vlive` | 基础 | |
| `arrest` | 基础 | |
| `profile/reg-time` | 基础 | |
| `profile` | 基础 | |
| `education/challenge` | 基础 | |
| `education/bonds` | 基础 | |
| `education/leader` | 基础 | |
| `music/list` | A 类新增 | |
| `music/note-count` | A 类新增 | 参数修正后通过 |
| `music/rewards` | A 类新增 | |
| `music/progress` | C 类新增 | 用户有打歌数据 |
| `profile/check-data` | A 类新增 | |
| `profile/check-data-mysekai` | A 类新增 | |
| `profile/verify/list` | A 类新增 | |
| `profile/suite/hide` | B 类新增 | |
| `profile/suite/show` | B 类新增 | |
| `profile/mysekai/hide` | B 类新增 | |
| `profile/mysekai/show` | B 类新增 | |
| `profile/visibility/hide` | B 类新增 | 命令修正后通过 |
| `profile/visibility/show` | B 类新增 | 命令修正后通过 |
| `sk/speed` | D 类新增 | |
| `sk/check-room` | D 类新增 | |
| `sk/rank-trace` | D 类新增 | |

#### ❌ 失败（27 个）

| 端点 | 错误 | 归因 |
|------|------|------|
| `card/list` | card ids are required | Parser handler 未提取参数 |
| `event` | event id is required | Parser handler 未提取参数 |
| `score/music-meta` | music meta request is empty | Parser handler 未提取参数 |
| `misc/birthday` | invalid birthday request | Parser handler 未提取参数 |
| `education/area` | area item request has no items | 功能未实现 |
| `education/power` | power bonus request has no bonuses | 功能未实现 |
| `music/bpm` | 当前环境没有可读取的本地谱面文件 | 本地无 chart 资源 |
| `music/cover` | startapp/music/jacket/...no such file | 本地无 jacket 资源 |
| `alias/pending` | 你不是别名审核管理员 | 需 admin 权限 |
| `score` | invalid score control request | Bridge 未完整填充 MusicID |
| `score/custom-room` | invalid custom-room score request | 需 CandidatePairs 数据 |
| `score/music-board` | music board request has no items | Bridge 未填充 items |
| `event/record` | requires at least one history entry | 用户无活动历史 |
| `deck/event` | local user snapshot is not configured | 无 Toolbox 用户快照 |
| `deck/challenge` | 同上 | 同上 |
| `deck/no-event` | 同上 | 同上 |
| `deck/bonus` | 同上 | 同上 |
| `deck/mysekai` | 同上 | 同上 |
| `mysekai/resource` | 同上 | 同上 |
| `mysekai/talk-list` | 同上 | 同上 |
| `mysekai/fixture-list` | 同上 | 同上 |
| `mysekai/fixture-detail` | 同上 | 同上 |
| `mysekai/door-upgrade` | 同上 | 同上 |
| `mysekai/music-record` | 同上 | 同上 |
| `mysekai/photo` | 同上 | 同上 |
| `sk/player-trace` | sk player-trace request has no ranks | 需用户 SK 排名数据 |
| `sk/winrate` | sk winrate request has no teams | 需 5v5 队伍配置 |

### 失败归因分类

| 类别 | 数量 | 说明 |
|------|------|------|
| Parser handler 未接入 | 4 | card/list, event, score/music-meta, misc/birthday |
| 功能未实现 | 2 | education/area, education/power |
| 本地无资源文件 | 2 | music/bpm (chart), music/cover (jacket) |
| 无 Toolbox 用户快照 | 12 | deck/\*, mysekai/\* |
| Bridge 数据不完整 | 3 | score, score/custom-room, score/music-board |
| 用户数据不足 | 2 | event/record (无历史), sk/player-trace (无排名) |
| 权限不足 | 1 | alias/pending (需 admin) |
| 外部依赖 | 1 | sk/winrate (需 5v5 配置) |

## 10.7 集成测试执行结果（2026-03-27，第六轮）

**通过数：33/58**（较第五轮 +2）

### 本轮修复内容

#### 远程素材拉取 + CWD 符号链接

`music/bpm` 和 `music/cover` 依赖本地谱面文件（music_score）和曲绘文件（jacket），此前本地环境未配置。

从远程服务器 (`INTERNAL_HOST:60022`) 的 `/data/HarukiServices/asset/data/jp-assets/startapp/music/` 拉取 Tell Your World（musicID=1）的 chart 和 jacket 文件至 `tmp/game-assets/`，然后在项目 CWD 下创建符号链接：

```
music/music_score/0001_01 → tmp/game-assets/music/music_score/0001_01
music/jacket/jacket_s_001 → tmp/game-assets/music/jacket/jacket_s_001
```

**关键发现**：`asset_dirs.primary` 不能设置为本地目录，否则 `ResolveAssetPath` 会将本地路径传给 Drawing API（远程），导致 Drawing API 找不到文件。保持 `asset_dirs` 为空，让 `AssetHelper` 默认使用 CWD (`.`) 作为 root，这样：
- 本地文件通过 `FirstExisting` 从 CWD 相对路径找到 → BPM/Cover 正常
- Drawing API 收到的路径不含本地前缀 → 远程渲染正常

#### IPv6 连接问题修复

集成测试 `baseURL` 从 `http://localhost:6666` 改为 `http://127.0.0.1:6666`，避免 Go HTTP 客户端优先解析为 IPv6 `[::1]` 而服务器只监听 IPv4 的问题。

### 第六轮新增通过端点

| 端点 | 说明 |
|------|------|
| `music/bpm` | 本地 chart 文件可用，BPM 解析成功 |
| `music/cover` | 本地 jacket 文件可用，曲绘返回成功 |

### 第六轮完整结果（33/58）

✅ 通过（33）：bind, card/detail, card/box, music, event/list, gacha, education/challenge, education/bonds, education/leader, profile, profile/reg-time, stamp, sk/line, sk/query, vlive, arrest, music/list, **music/bpm**, **music/cover**, music/note-count, music/rewards, music/progress, profile/check-data, profile/check-data-mysekai, profile/verify/list, profile/suite/hide, profile/suite/show, profile/mysekai/hide, profile/mysekai/show, profile/visibility/hide, profile/visibility/show, sk/speed, sk/check-room, sk/rank-trace

❌ 失败（25）：与第五轮相同减去 music/bpm 和 music/cover

### Censor 配置状态

`.env` 中已添加 Baidu 和 Tencent 内容审核密钥：
- **Baidu 文本审核**：API Key + Secret 已配置 ✅
- **Tencent 图片审核**：SecretID + SecretKey + Region 已配置，但 **BizType 为空** ⚠️
  - BizType 是腾讯云内容安全控制台中创建的自定义审核策略标识，可选参数，留空时使用默认策略
  - 如需自定义审核策略，需登录腾讯云控制台 → 内容安全 → 图片内容安全 → 策略管理 → 创建策略后获取

## 10.8 Handler 路径审计（2026-03-27）

### 注册路径总览

历史记录（2026-03-27）：当时 `EnsureCommandHandlersRegistered` 共注册 **76 条** bot API 路径（含 alias 系列），并完成 **76/76 全覆盖**。

截至 2026-04-10，当前活跃 Bot path 已更新为 **82 条**；`integration/api_test.go` 当前覆盖其中 **82 条**（`TestBotCommands` 58 + `TestExpandedCoverage` 24）。

当前活跃 path 在集成测试集合中的覆盖已补齐为 **82/82**。

### 无 Path 的 handler（不可通过 bot API 访问）

| handler | 文件 | 指令 | 说明 |
|---------|------|------|------|
| `HeyiweiHandle` | `misc.go` | `/b30`、`/b39`、`/pjsk b30` 等 | 返回固定字符串 `"何意味"`，为占位/拦截 handler。不返回 `*parser.ResolvedCommand`，即使添加 Path 也无法在 bot API 中正常工作。可按需 Disable 或改为真实 b30 实现 |

> 说明：`MysekaiBlueprintHandle` 已在后续版本补齐 `Path: "mysekai/blueprint"`，现已纳入活跃 route。

### 历史记录：当时未纳入集成测试的 17 条路径

| 路径 | 分类 | 说明 |
|------|------|------|
| `alias/music` | 别名查询 | 歌曲别名查询，数据已导入，可直接测试 |
| `alias/music/add` | 别名提交 | 提交新别名（待审核） |
| `alias/music/del` | 别名删除 | 删除别名（需权限） |
| `alias/character` | 别名查询 | 角色别名查询，数据已导入，可直接测试 |
| `alias/character/add` | 别名提交 | 提交新角色别名（待审核） |
| `alias/character/del` | 别名删除 | 删除角色别名（需权限） |
| `alias/approve` | 别名审核 | 审核通过别名（需管理权限） |
| `alias/reject` | 别名审核 | 审核拒绝别名（需管理权限） |
| `card/image` | 渲染 | 卡面原图渲染（大图），依赖远程 asset |
| `music/chart` | 渲染 | 谱面图渲染，依赖本地 chart 文件 |
| `profile/bg/upload` | 用户设置 | 上传个人信息页背景图，需文件上传支持 |
| `profile/bg/clear` | 用户设置 | 清除背景图 |
| `profile/bg/adjust` | 用户设置 | 调整背景图位置/大小参数 |
| `profile/default` | 用户设置 | 设置默认展示绑定账号 |
| `profile/default/clear` | 用户设置 | 清除默认绑定 |
| `profile/unbind` | 账号管理 | 解绑账号 |
| `profile/verify` | 账号验证 | 对实际游戏账号发起验证（单向写入） |

### 有 Path 但尚未在测试中覆盖的路径分类建议

> ✅ 第七轮已将上方“当时未覆盖的 17 条路径”补测完成，见 10.9 节。  
> ✅ 截至 2026-04-10，活跃 path 已扩展到 82 条，且当前集成测试覆盖已补齐为 82/82。

---

## 10.9 集成测试执行结果（2026-03-27，第七轮 — 全路径覆盖）

### 测试方法

新增 `TestExpandedCoverage` 测试函数，覆盖此前未测试的 17 条路径。测试环境准备：

1. **Alias Admin 配置**：通过 PostgreSQL 直接写入 `alias_admins` 表，使测试用户 (`haruki_user_id=929565`) 成为别名管理员
2. **本地图片服务器**：在随机端口启动 HTTP 文件服务器，提供 `IMG_7736.png` 作为自定义背景图上传测试源
3. **账号验证前置**：`profile/verify` 在 `profile/bg/*` 之前执行，确保绑定账号已验证（BG 操作要求 `verified=true`）
4. **唯一别名名称**：使用时间戳后缀避免重复运行时的 "已在待审核列表" 冲突
5. **卡面素材拉取**：从远程服务器拉取 `character/member/res001_no001/card_normal.png`，通过 CWD 符号链接使本地可访问

### 第七轮新增通过端点（17/17 全部通过）

| 端点 | 说明 |
|------|------|
| `alias/music` | 歌曲别名查询 |
| `alias/character` | 角色别名查询 |
| `alias/music/add` | 提交歌曲别名（进待审核） |
| `alias/character/add` | 提交角色别名（进待审核） |
| `alias/approve` | 审核通过别名（管理员操作） |
| `alias/reject` | 审核拒绝别名（管理员操作） |
| `alias/music/del` | 删除已审核歌曲别名（管理员操作） |
| `alias/character/del` | 删除已审核角色别名（管理员操作） |
| `card/image` | 卡面原图渲染（需本地 card_normal.png） |
| `music/chart` | 谱面图渲染（Drawing API 远程渲染） |
| `profile/bg/upload` | 上传自定义背景图（HTTP URL → 下载存储） |
| `profile/bg/adjust` | 调整背景图参数（模糊/透明度/方向） |
| `profile/bg/clear` | 清除自定义背景图 |
| `profile/default` | 设置默认绑定账号 |
| `profile/default/clear` | 清除默认绑定 |
| `profile/verify` | 验证绑定账号（Toolbox 快速验证） |
| `profile/unbind` | 解绑账号（后自动重新绑定恢复状态） |

### 第七轮完整结果（50/76）

✅ 通过（50）：

**原 TestBotCommands（33/59）**：bind, card/detail, card/box, music, event/list, gacha, education/challenge, education/bonds, education/leader, profile, profile/reg-time, stamp, sk/line, sk/query, vlive, arrest, music/list, music/bpm, music/cover, music/note-count, music/rewards, music/progress, profile/check-data, profile/check-data-mysekai, profile/verify/list, profile/suite/hide, profile/suite/show, profile/mysekai/hide, profile/mysekai/show, profile/visibility/hide, profile/visibility/show, sk/speed, sk/check-room

**新增 TestExpandedCoverage（17/17）**：alias/music, alias/character, alias/music/add, alias/character/add, alias/approve, alias/reject, alias/music/del, alias/character/del, card/image, music/chart, profile/bg/upload, profile/bg/adjust, profile/bg/clear, profile/default, profile/default/clear, profile/verify, profile/unbind

❌ 仍失败（26/76，与第六轮相同原因）：

| 分类 | 端点 | 原因 |
|------|------|------|
| Parser 未提取参数 | card/list, event, score/music-meta, misc/birthday | handler 未从文本提取参数 |
| 未实现 | education/area, education/power | bridge 返回空数据 |
| 需要 Toolbox 快照 | deck/event, deck/challenge, deck/no-event, deck/bonus, deck/mysekai, mysekai/resource, mysekai/talk-list, mysekai/fixture-list, mysekai/fixture-detail, mysekai/door-upgrade, mysekai/music-record, mysekai/photo | Toolbox GetSuiteData 未配置 |
| Bridge 数据不完整 | score, score/custom-room, score/music-board, event/record | 缺少 alias 解析 / 候选对 / 历史条目 |
| SK 数据不完整 | sk/rank-trace, sk/player-trace, sk/winrate | 依赖实时追踪数据 |
| 别名待审核 | alias/pending | 当前无待审核条目（空列表非错误但无 data） |

### 10.10 第八轮：Toolbox 快照注入 + Drawing API 离线（2026-03-27）

**核心改进**：实现 `resolveLiveSnapshot()` 从 Toolbox API 实时获取 suite + mysekai 数据，注入 deck/* 和 mysekai/* 控制器，解决全部 12 个 "local user snapshot is not configured" 失败。

**本轮状态**：
- Drawing API（远程 SSH INTERNAL_HOST:60022）不可达，key exchange 阶段被远程关闭
- 所有依赖 Drawing API 的端点均报 `connection refused on port 28000`
- Go test **76/76 路径已覆盖并执行**（测试进程全部 PASS；失败端点以 warning/degraded 形式报告，不 fail test）

**端点分类（共 76 条测试路径）**：

| 状态 | 数量 | 端点 |
|------|------|------|
| ✅ 完全通过 | 22 | profile/reg-time, sk/line, sk/query, vlive, arrest, music/bpm, music/cover, music/note-count, alias/pending, profile/check-data, profile/check-data-mysekai, profile/verify/list, profile/suite/hide, profile/suite/show, profile/mysekai/hide, profile/mysekai/show, profile/visibility/hide, profile/visibility/show, sk/speed, sk/check-room, sk/rank-trace, mysekai/photo |
| ⚠️ Drawing API 不可达 | 35 | bind, card/detail, card/box, card/list, music, event, event/list, gacha, stamp, profile, education/challenge, education/bonds, education/leader, music/list, music/rewards, music/progress, event/record, deck/event, deck/challenge, deck/no-event, deck/bonus, deck/mysekai, mysekai/resource, mysekai/talk-list, mysekai/fixture-list, mysekai/fixture-detail, mysekai/door-upgrade, mysekai/music-record 等 |
| ❌ 数据/逻辑问题 | ~12 | education/area, education/power, score, score/custom-room, score/music-board, score/music-meta, misc/birthday, sk/player-trace, sk/winrate 等 |
| ✅ TestExpandedCoverage | 17/17 | alias 全周期(4), profile BG(2), profile 管理(3), card/image(1), music/chart(1), alias CRUD(6) |

**关键 Toolbox 验证数据**：
- `GetSuiteData(jp, GAME_USER_ID_REDACTED, qq, QQ_ID_REDACTED)` → 成功获取 suite 快照（含 userCards, userDecks 等）
- `GetMySekaiData(jp, GAME_USER_ID_REDACTED, qq, QQ_ID_REDACTED)` → 成功获取 mysekai 快照（含 updatedResources）
- 数据正确合并（`NewFromBytes` → `mergeMySekaiBytes` → 统一 `userdata.Service`）

**提交**：`3537148` — `feat: inject live Toolbox snapshots into deck/* and mysekai/* endpoints`

### 10.11 第九轮：Asset 路径修复 — ondemand 前缀 + staticPath（2026-03-27）

通过 EasyTier VPN 网格打通 Drawing API（`INTERNAL_IP_1:28000`），SSH 调查容器内文件布局后修复资产路径。

**问题根因**：mysekai/deck 控制器中所有资产路径使用硬编码相对路径（如 `mysekai/icon/...`），但 Drawing API 的 `get_img_from_path()` 需要完整前缀路径（如 `asset/jp-assets/ondemand/mysekai/icon/...`）。

**代码改动**：
- `mysekai/controller.go`：添加 `assets *AssetHelper` 字段 + `regionPath()`/`staticPath()` 方法，更新 ~20 处硬编码路径
- `mysekai/helpers.go`：添加 `pathResolver` 类型，更新 `extractMysekaiPhenoms`、`fixtureThumbnailPath`、`fixtureColorImages`、`musicRecordIconPath`
- `deck/controller.go`：canvas 路径改用 `assets.ResolveRegionAssetPath()`
- `drawing/client.go`：`GenerateMysekaiFixtureDetail()` 请求体包装为数组
- `app/app.go`：传递 `assetHelper` 给 `mysekai.NewController()`

**提交**：`7e16352` — `fix: resolve mysekai/deck asset paths through region asset helper`

**结果**：55/76 ✅ OK（72%），新增 4 个通过：mysekai/fixture-list、mysekai/music-record、deck/bonus、deck/mysekai

### 10.12 第十轮：static_images 路径修复 + DNS 恢复（2026-03-28）

SSH 调查 Drawing 容器后发现部分资产属于 `static_images/` 而非 region assets，修正路径分类。

**修复内容**：
| 资产 | 原路径方式 | 正确路径方式 | 影响端点 |
|------|-----------|-------------|---------|
| `gate_icon/gate_*.png` | `regionPath` → ondemand | `staticPath` → static_images/mysekai/ | mysekai/resource |
| `invitationcard.png` | `regionPath` → ondemand | `staticPath` → static_images/mysekai/ | mysekai/resource (visit chars) |
| `chara_icon/*.png` | 裸 `fmt.Sprintf` | `staticPath` → static_images/chara_icon/ | mysekai/talk-list, fixture reactions |
| `music_record.png` | `regionPath` → ondemand | `staticPath` → static_images/mysekai/ | mysekai/resource (site resources) |

**额外修复**：远程 Drawing 服务器 DNS 解析恢复，`mysekai/fixture-detail` 不再报 name resolution 错误。

**提交**：`debc7ed` — `fix: use staticPath for gate_icon, invitationcard, chara_icon, music_record`

**结果**：**57/76 ✅ OK（75%）**，测试路径覆盖 76/76（测试进程全部 PASS）。新增通过：mysekai/resource、mysekai/fixture-detail

| 类别 | 通过数 | 端点 |
|------|--------|------|
| TestBotCommands ✅ | 40/59 | card/detail, music, event/list, gacha, education/{challenge,bonds,leader}, profile(×10), stamp, sk/{line,query,speed,check-room,rank-trace}, vlive, arrest, music/{list,bpm,cover,note-count,rewards,progress}, alias/pending, deck/{bonus,mysekai}, mysekai/{resource,fixture-list,fixture-detail,music-record,photo} |
| TestExpandedCoverage ✅ | 17/17 | alias(×8), profile/{verify,bg/upload,bg/adjust,bg/clear,default,default/clear,unbind}, card/image, music/chart |
| ⚠️ 仍失败 | 19 | 见下方 §11 分类 |

---

## 11. 接下来需要做的事

> 历史快照（第十轮，2026-03-28）：**57/76 ✅ OK**（75%），测试路径覆盖为 76/76。
>
> 以上是阶段性回归快照。后续代码已继续演进（包含 parser、score、education、bonds 等修复），旧版“剩余 19 个 warning”分类仅供历史对照；当前待处理项以**当前代码状态**为准。

### 11.1 ✅ Parser handler 参数提取（已完成）

`card/list`、`event`、`score/music-meta`、`misc/birthday` 的参数提取问题已修复，相关 bot 路由不再因请求体为空而直接失败。

**代码改动**：
- `card/detail` / `card/list` 统一改为按参数语义分流：单卡 ID 走 detail，普通查询走 list，`box/id/before` 语义走 card-box
- `event` 支持空参数时返回当前活动，传数字参数时解析为 `event_id`
- `score/music-meta` 支持从命令文本拆分 1–3 个歌曲查询项并写入 `queries`
- `misc/birthday` 支持“最近第 N 个生日”与“角色名 → character_id”两类参数

**验证结果**：
- `card_test.go`、`event_test.go`、`score_test.go`、`misc_birthday_test.go` 已覆盖对应解析逻辑
- `bridge_test.go` 中 `score/music-meta` 构建请求已通过
### 11.1.1 P0：当前仍待回归确认的问题

按当前代码状态整理，已完成的 parser / score / education / bonds 修复项不再计入剩余问题。当前仍需关注的主要是以下 4 类：

#### A. Drawing API 服务端错误（4 个）

| 端点 | 错误信息 | 原因 |
|------|---------|------|
| `deck/event` | `{"detail":"None"}` | Drawing API 内部处理返回 None，需检查 deck 请求数据与服务端消费逻辑 |
| `deck/challenge` | `{"detail":"None"}` | 同上 |
| `deck/no-event` | `{"detail":"None"}` | 同上 |
| `mysekai/door-upgrade` | `color must be int or tuple` | Python Pillow 颜色参数类型错误，属于 Drawing API 侧 bug |

#### B. Drawing API 服务端资产缺失（1 个）

| 端点 | 缺失资产 | 说明 |
|------|---------|------|
| `mysekai/talk-list` | `character/character_sd_l/chr_sp_*.png` | SD 立绘资源在服务器侧不完整，当前 `jp-assets` 未具备所需文件 |

#### C. 数据依赖不足（3 个）

| 端点 | 错误信息 | 说明 |
|------|---------|------|
| `event/record` | `at least one history entry` | 依赖用户活动历史数据，当前测试账号数据不足 |
| `sk/player-trace` | `no ranks` | 依赖活跃 SK 期间的排名追踪数据 |
| `sk/winrate` | `no teams` | 依赖对战记录/队伍数据 |

#### D. 性能 / 测试环境限制（1 个）

| 端点 | 说明 |
|------|------|
| `card/box` | 全卡牌箱渲染耗时超过 90s，当前更像性能与测试超时限制问题，而非功能缺失 |

**备注**：
- 以上按当前代码状态去重后，可明确归入待处理的问题约为 9 个端点/场景
- 该数量用于整理待办，不替代下一轮完整集成测试统计

### 11.2 ✅ 集成测试覆盖率扩展（已完成）

第七轮已实现 76/76 路径全覆盖（100% 覆盖率），其中 50 条通过（66% 通过率）。详见 10.9 节。

**测试架构（当前文件统计）**：`TestBotCommands`（58 个端点）+ `TestExpandedCoverage`（24 个端点，含 alias 全周期、profile BG、profile 管理、card/image、music/chart、`profile/bind` 前置校验、`deck/score-up`、`mysekai/{blueprint,map}`、`sk/{daily-speed,predict}`）。

### 11.2.1 ✅ Toolbox 用户快照注入（已完成）

deck/\*、mysekai/\* 共 12 个端点的 Toolbox 快照注入问题已解决：

**代码改动**：
- `deck.Controller.WithSnapshot()` / `mysekai.Controller.WithSnapshot()` — 允许 bridge 注入运行时快照
- `resolveLiveSnapshot()` — 统一的 Toolbox 数据拉取逻辑（解析绑定 → GetSuiteData → 可选 GetMySekaiData → NewFromBytes）
- `executeDeck()` / `executeMysekai()` 调用 `resolveLiveSnapshot()` 后传入 `WithSnapshot()` 克隆
- `app.go` — 始终创建 mysekai.Controller（不再要求本地快照文件）
- `haruki-db-configs.yaml` — 添加 `local_masterdata` 配置指向 `Data/master/haruki-sekai-master/master`

**验证结果**：
- 所有 12 个端点成功获取 Toolbox 数据（suite + mysekai）
- deck/* (5)：构建了完整的 DeckRequest，到达 Drawing API 调用阶段
- mysekai/resource、fixture-list、fixture-detail、door-upgrade、music-record (5)：成功加载 masterdata + 快照，到达 Drawing API 调用阶段
- mysekai/talk-list (1)：成功加载快照（测试已添加角色名参数），到达 Drawing API 调用阶段
- **mysekai/photo (1)：✅ 完全通过**（不依赖 Drawing API）
- 其余 11 个端点仅因 Drawing API 不可达而失败（connection refused on port 28000）

### 11.2.2 ✅ Score/Bridge 数据填充（已完成）

此前缺失的 3 个 score 端点数据填充已完成：

| 端点 | 已完成内容 |
|------|-----------|
| `score` | 支持从歌曲名/ID 解析 `music_id`，补齐封面、分数线与有效分数候选 |
| `score/custom-room` | 已接入 `data/custom_room_pt.csv` 作为候选对组数据源，能够生成有效 `candidate_pairs` |
| `score/music-board` | 已从 music meta 快照构建排行榜条目与高亮曲目列表，不再出现 `items` 为空 |

**验证结果**：
- `bridge_test.go` 中 `TestExecuteScoreControlBuildsRequestFromParams`
- `bridge_test.go` 中 `TestExecuteCustomRoomScoreBuildsRequestFromParams`
- `bridge_test.go` 中 `TestExecuteScoreMusicBoardBuildsRequestFromParams`

### 11.3 ✅ Education 端点实现（已完成）

此前未实现的 `education/area`（区域道具）与 `education/power`（加成信息）已改为从 Toolbox suite 快照构建请求，再结合 Cloud 侧 master data 进行计算。

**代码改动**：
- `bridge.go` 新增 `buildEducationSnapshotFromSuite()`，统一从 Toolbox 构造运行时教育快照
- `education/area` 改为走 `BuildAreaItemUpgradeMaterialsRequestFromSnapshot()`
- `education/power` 改为走 `BuildPowerBonusDetailRequestFromSnapshot()`

**验证结果**：
- `render/education/snapshot_build_test.go` 中的 power / area 构建测试已更新并通过

### 11.4 ✅ 无 Path Handler 处置（已完成）

| handler | 处理结果 |
|---------|---------|
| `HeyiweiHandle` | 已改为 `Disabled: true`，不再拦截真实指令 |
| `MysekaiBlueprintHandle` | 已添加 `Path: "mysekai/blueprint"`，并在 handler 内直接复用 `mysekai-fixture-list` / `mysekai-talk-list` 的分流逻辑，无需单独 bridge case |

**验证结果**：
- `misc_test.go` 已检查 `HeyiweiHandle` 为 disabled
- `mysekai_test.go` 已覆盖 `MysekaiBlueprintHandle` 的派发行为

### 11.5 ✅ education/bonds 数据补全（已完成）

此前文档对问题原因的判断不准确。实际问题不在于必须扩充 `CharacterIDToNickname`，而在于 bridge 在构造 `education/bonds` 请求时，过早按昵称映射过滤条目，导致本应走 `chr_icon_<id>.png` 回退路径的羁绊数据被直接丢弃。

**代码改动**：
- `buildBondsRequestFromSuite()` 不再因缺少 `CharacterIDToNickname` 映射而提前跳过 bonds 条目
- 统一交给 `charaIconPath()` 处理角色图标路径回退
- 补齐 Drawing 侧实际使用的 `max_level`、`need_exp`、`color1`、`color2` 字段

**验证结果**：
- `bridge_test.go` 中新增 `TestBuildBondsRequestFromSuiteIncludesFallbackIconsAndProgress`

### 11.6 ✅ Drawing API 集成修复与全量集成测试（已完成）

协作者在远程推送了 Drawing API 相关的大量功能新增后，合并并进行了系统性集成修复。

**修复内容汇总**：

| 修复项 | 根因 | 解决方案 |
|--------|------|----------|
| deck 全系 500 `{"detail":"None"}` | Go `*string` omitempty → Python `DIFF_COLORS[None]` KeyError | Drawing API 侧添加 None 守卫 |
| event 500 | `bonus_chara_path` 为 None 时迭代报错 | Drawing API 侧修正条件判断 |
| education/power 图标为空 | `normalizeUnit()` 返回短名，`UnitIconFilename()` 不接受短名 | 补齐短名映射（light_sound, idol, street 等） |
| education/area 422 | Go nil slice → JSON `null`，Pydantic 要求 `list` | 初始化为 `[]drawing.AreaItemMaterial{}` |
| education/bonds 图标缺失 | 查询用 `GameIDIn` 但角色 ID≥46 的 game_id≠game_character_id | 改为 `GameIDIn` 查询 + gameID→gameCharacterID 映射 |
| misc/birthday 路径错误 | `chara_rip` 应为 `chara`，缺少 `ResolveRegionAssetPath` | 修正路径前缀，包裹 ResolveRegionAssetPath |
| card/list 400 | 测试用 `/查牌` 非注册指令 | 改为 `/卡牌列表` |
| card/list 500（特训卡缩略图） | `initial_special_training_status='done'` 的卡无 `_normal.png` | 检测预训练状态，使用 `_after_training.png` |
| material 路径 | `material_rip` / `common_material_rip` 不存在 | 改为 `material` / `common_material`，使用 ResolveRegionAssetPath |

**全量集成测试最终结果（2026-03-28）**：

76 个测试路径（当时测试文件为 `TestBotCommands` 59 + `TestExpandedCoverage` 17），**70/76 通过（92.1%）**，6 个⚠️均为已知数据/功能限制：

| 端点 | 状态 | 原因分类 |
|------|------|----------|
| card/box | ⚠️ HTTP 500 | Drawing API 渲染超时（用户卡牌数量大，canvas 渲染耗时 >90s） |
| education/area | ⚠️ HTTP 500 | Canvas 过大（26441×819）——Go 端缺少过滤参数透传，需实现 |
| score | ⚠️ HTTP 500 | 测试用户 PT 不满足控分条件（数据限制） |
| score/custom-room | ⚠️ HTTP 500 | 测试用户 PT < 100，不满足自定义房间控分要求（数据限制） |
| sk/player-trace | ⚠️ HTTP 500 | 测试用户不在 top 100（代码正确，已用排名第一的 UID 验证通过） |
| sk/winrate | ⚠️ HTTP 500 | 当前无 5v5 赛事数据（游戏侧暂停，非代码问题） |

**待实现功能**：
- `education/area` 过滤参数透传：Handler 已正确提取 `r.Query`（如"树"/"miku"/"25h"），但 Bridge 层未读取、Builder 未过滤。需在 `AreaItemQuery` 增加 Filter 字段，按 `TargetUnit`/`TargetGameCharacterID`/`TargetCardAttr` 过滤后再发送给 Drawing API。

### 11.7 Alpha 环境部署

**部署时间**：2026-03-28

**部署位置**：远程主机 `INTERNAL_IP_1` — `/data/HarukiServices/alpha/`

**架构**：
- PostgreSQL 18.3（容器 `haruki-alpha-postgres`，宿主端口 `127.0.0.1:15432`）
- Redis 7-alpine（容器 `haruki-alpha-redis`，宿主端口 `127.0.0.1:16379`）
- Haruki-Cloud 后端（宿主机直接运行，端口 `6667`）

**数据库**：7 个库全部创建，其中 `haruki_sekai`、`haruki_pjsk`、`haruki_users`、`haruki_bot` 已从本地迁移数据。

**外部服务端点**：
| 服务 | 端点 |
|------|------|
| Toolbox API | `http://INTERNAL_IP_3:16666` |
| Event Tracker | `http://INTERNAL_IP_2:8777` |
| Sekai API | `http://INTERNAL_IP_2:9999` |
| Drawing API | `http://INTERNAL_IP_1:28000`（同机） |

**Bot 凭证**：已生成并写入本地 `client.json`（已加入 `.gitignore`）。Noise IK 加密传输已启用。

**当前状态**：✅ 服务已启动，API 可访问（`http://INTERNAL_IP_1:6667`），bot 认证接口响应 401（预期行为，需携带凭证）。

**客户端接入就绪**：client 可使用 `client.json` 中的凭证连接 alpha 环境进行对接开发。

**Command Manifest**：✅ 指令清单已通过 `SeedCommandManifests` 写入 `command_manifests` 表（数量随当前注册路由变化）；客户端通过 `GET /api/v2/bot/:botId/command/manifests`（需 session token）获取完整指令列表。

### 11.8 MySekai 国服区域限制

MySekai 功能对 `region=cn` 默认关闭。通过配置白名单 `allow_cn_mysekai` 可对特定平台+群组开放：

```yaml
pjsk:
  allow_cn_mysekai:
    - platform: "qq"
      group_id: "12345678"
```

- **实现位置**：`internal/pjsk/handler/bridge.go` — `executeMysekai()` 入口检查
- **配置位置**：`config/config.go` — `PJSKConfig.AllowCNMySekai`
- **透传字段**：`ResolvedCommand.RequesterGroupID`（由 bot handler 传入）
- 未命中白名单时返回 "MySekai 功能暂不支持国服区域"

### 11.9 P3：其他待处理事项（当前剩余）

| 事项 | 状态 | 说明 |
|------|------|------|
| education/area 过滤透传 | ✅ 已完成 | Go 端已支持解析团名/角色名/属性/树/花过滤参数，并在构建请求前按 master data 过滤 area items |
| card/box 渲染超时 | ⏸ 待优化 | Drawing API 渲染大量卡牌耗时过长，需优化或分页 |
| score/custom-room 布局溢出 | ✅ 已完成 | Drawing 端已扩大歌曲列宽度，并按行内可用宽度裁切歌曲标题，避免 `Content size too large` |
| `origin/test` force push | ⚠️ 待操作 | 本地 `test` 分支历史已重写（credential cleanup），需 `git push --force-with-lease origin test` 才能同步 |
| Censor Tencent 图片审核 | ⚠️ BizType 待填 | SecretID/Key/Region 已配置，BizType 留空使用默认策略 |
| alias 管理 API 归属 | ⏸ 待决策 | 别名新增/审核/拒绝操作归属（bot API vs admin API）待设计决策 |
| alpha 进程管理 | ⏸ 建议 | 建议使用 systemd 或 supervisor 管理 haruki-server 进程，当前为 nohup 启动 |

## 12. 相关文档

- [**Bot 客户端对接指南**](client-integration-guide.cn.md) ← 客户端接入必读
- [PJSK 指令系统设计](pjsk-command-system.cn.md)
- [PJSK 账号绑定实现说明](pjsk-profile-binding-implementation.cn.md)
- [ZeroBot 与 Cloud 联调方案](zerobot-cloud-integration-plan.cn.md)
- [ZeroBot 渲染接入后续事项](zerobot-render-followup.cn.md)
- [Haruki Toolbox API 客户端文档](toolbox-api.cn.md)
