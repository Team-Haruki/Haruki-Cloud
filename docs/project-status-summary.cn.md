# Haruki-Cloud 项目进展总结

> 最后更新：2026-03-24（v14.0）
>
> 涉及 `Haruki-ZeroBot` 联调的协议边界，请优先参考 `docs/zerobot-cloud-integration-plan.cn.md`。

## 1. 当前结论

`Haruki-Cloud` 当前已经完成两部分核心合并：

1. `Service-Test` 的渲染子系统
2. `Test_Instruction_Parser` 的解析资源和处理资源

但 PJSK 指令主链路的目标模型已经重新明确：

1. 云端下发 manifest。
2. 客户端构建前缀树并命中 `path`。
3. 客户端按命中的 `path` 调用 `/api/v2/bot/:botId/pjsk/*`，并上传 `matched_command`。
4. 命中的端点在云端内部先校验 `matched_command -> handler.path`，再解析原文、提取参数，并进入统一执行链路返回图片或文本。

也就是说，客户端负责“命中哪个端点”，云端端点负责“这个端点到底怎么解释原文”。

## 2. 已完成的合并内容

### 2.1 渲染子系统

`Service-Test` 的渲染能力已经稳定落在：

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

### 3.1 客户端入口

- `GET /api/v2/bot/:botId/command/manifests`

### 3.2 业务端点

- `GET /api/v2/bot/:botId/pjsk/<path>?command_payload=<base64(ob11 pack)>`

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
4. `Execute(...)` 统一返回 `payload + data_type`
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
| **活动排名（honor badge 上的名次）** | **未接入** | ⚠️ 见下方存疑项 |

### 存疑 / 待确认

1. **活动排名显示（UserEventResults）**：honor builder 中 `query.Rank > 0` 会将 honor badge 上的 FcOrApLevel 替换为活动排名数字。当前 `BuildProfileRequestFromAPI` 显式传入 `nil` 忽略此字段，honor badge 正常显示 honor 等级。**协作者对 honor builder 这部分的设计意图尚未确认**，待对齐后再决定是否补充来源（如从工具箱 key 查询补充 `userEventResults`）。

2. ~~**IsHideUID**~~：已解决。读取 `query.Visible`（binding 的 `Visible` 字段），`IsHideUID = !binding.Visible`。

3. **UpdateTime**：`BuildProfileRequestFromAPI` 固定传入 `nil`，确保 image cache system 在相同渲染内容下得到稳定的 cache key。

### 待完成

~~**ProfileInfoHandle 接入**：已完成（v12.0）。~~

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

- **别名系统（alias-feature）**：`/注册时间` 等指令的别名支持，待后续设计

## 5.4 Image Cache System & 颗粒度 Ban（v13.0 新增）

### Image Cache System

**背景**：此前 bot API 对图片渲染结果直接以 `image/png` raw bytes 返回，不符合设计——bot 框架（如 go-cqhttp / nonebot）应收到 OneBot11 `MessageSegment`，而非裸字节流。

**实现方案**：

| 层级 | 说明 |
|------|------|
| `utils/imagecache.Client` | 新建包；`StoreAndGetURL(data, group)` 将 PNG 以 SHA-256 内容寻址写入磁盘并返回 CDN URL |
| `renderapp.Config.ImageCacheURI/Dir` | 新增两个字段，对应 `pjsk_render.image_cache.uri/dir` |
| `renderapp.App.ImageCache` | 工厂 `New()` 中直接初始化，nil 时降级到 raw bytes |
| `CommandResultDataTypeImageURL` | `result.go` 新增类型；bridge 图片执行器调用 `imageCacheOrBytes()` 决定返回类型 |
| bot handler | 新增 `case CommandResultDataTypeImageURL` → 返回 JSON OneBot11 image segment `{"type":"image","data":{"file":"<url>"}}` |

**配置示例**：
```yaml
pjsk_render:
  image_cache:
    uri: "https://image-cache.example.haruki.local"
    dir: "/var/haruki/image-cache"
```

**降级行为**：`image_cache.uri` 或 `dir` 未配置时，`imagecache.New()` 返回 nil；bridge 自动回退为 `CommandResultDataTypeImagePNG`（raw bytes），保持兼容。

---

### 颗粒度 Ban Check

**背景**：users 表中已定义 `ban_state`、`pjsk_ban_state`、`pjsk_main_ban_state` 等多级 ban 字段，此前未接入任何命令处理链路。

**Ban 层级（检查顺序，首个命中即返回）**：

```
ban_state              → 全平台禁用
└── pjsk_ban_state     → 全 PJSK 模块禁用
    ├── pjsk_main_ban_state     → Card/Gacha/Event/Music/Deck/Education/Profile/Arrest/RegTime/CheckData/Stamp/Misc
    ├── pjsk_ranking_ban_state  → SK
    ├── pjsk_alias_ban_state    → Alias（待实现）
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

## 6. 全功能链路状态（v14.0 全量审计）

### 6.1 已全链路接通（E2E Ready）✅

以下功能从 bot 客户端入站 → handler → bridge → executor → 出站响应全部打通：

| 模块 | 功能 | 路径 |
|------|------|------|
| **Profile** | 绑定 / 解绑 / 设主账号 / 清除默认绑定 / 查看个人信息 | profile/bind · unbind · default · default/clear · profile |
| **Arrest** | 逮捕（self/at\_user/uid 三模式，含 Visible 检查） | arrest |
| **RegTime** | 注册时间查询（JP/EN + TW/KR/CN 双算法） | profile/reg-time |
| **CheckData** | 套件抓包时间（/sud）/ MySekai 抓包时间（/msd） | profile/check-data · check-data-mysekai |
| **Card** | 卡面详情 / 卡牌列表 / 卡牌一览（Box） | card/detail · list · box |
| **Music** | 歌曲详情 / 列表 / 进度 / 奖励 / 谱面预览 | music · list · progress · rewards · chart |
| **Gacha** | 卡池列表 | gacha |
| **Deck** | 活动/挑战/长草/加成/烤森 组卡推荐 | deck/event · challenge · no-event · bonus · mysekai |
| **Event** | 活动列表 / 活动详情 / 活动记录 | event/list · event · event-record |
| **Education** | 挑战信息 / 加成信息 / 区域道具 / 羁绊 / 队长统计 | education/challenge · power · area · bonds · leader |
| **Score** | 分数计算 / 自定义房间 / 歌曲 meta / 歌曲排行 | score · custom-room · music-meta · music-board |
| **SK** | 档线 / 查询 / 时速 / 查房 / 玩家轨迹 / 档线轨迹 / 胜率预测 / 日速 / SK 预测 / 水表 | sk/line · query · speed · check-room · player-trace · rank-trace · winrate · (日速/预测/水表→复用) |
| **MySekai** | 资源 / 对话列表 / 家具列表 / 家具详情 / 大门升级 / 唱片 / 蓝图 | mysekai/resource · talk-list · fixture-list · fixture-detail · door-upgrade · music-record |
| **Stamp** | 贴纸列表 | stamp |
| **Misc** | 角色生日 | misc/birthday |

> **统计**：约 75 个 handler · 15 个 module · 全部有 bridge case

---

### 6.2 已定义但未实现（Disabled / TODO）❌

以下功能 handler 已存在但 `Disabled: true`，executor 为存根，不暴露到 bot API：

**Profile 系统（23 个）**：交换绑定、隐藏/展示 UID、隐藏/展示抓包数据、黑名单管理、认证、背景图管理、用户统计、绑定历史、访客账号

**Music 系统（7 个）**：别名查询/添加/删除/同步、BPM 查询、曲绘查询、物量统计

**Stamp 系统（7 个）**：贴纸制作、随机贴纸、批量刷新、底图管理

**Card 系统（3 个）**：角色别名查询、卡面原图、卡牌剧情（仅 JP）

**Event 系统（2 个）**：活动剧情（仅 JP）、自动送火（仅 JP）

**Gacha 系统（1 个）**：抽卡记录

**MySekai 系统（3 个）**：照片下载、抓包数据检查、MSR 换绑

**Entertainment 系统（全部 8 个）**：猜曲绘 / 猜谱面 / 猜卡面 / 听歌识曲、模拟抽卡、活动限制管理（全部仅 JP）

**Virtual Live（1 个）**：vlive 查询

> **统计**：约 55 个 handler，均无 module 分配或无 executor 实现

---

### 6.3 特殊 handler（绕过 bridge）

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
6. `context.Background()` 在 bot handler 中硬编码（M-1），应传 `c.Context()`

## 10. 相关文档

- [PJSK 指令系统设计](pjsk-command-system.cn.md)
- [PJSK 账号绑定实现说明](pjsk-profile-binding-implementation.cn.md)
- [ZeroBot 与 Cloud 联调方案](zerobot-cloud-integration-plan.cn.md)
- [ZeroBot 渲染接入后续事项](zerobot-render-followup.cn.md)
