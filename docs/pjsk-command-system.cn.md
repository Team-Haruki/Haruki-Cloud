# Haruki-Cloud PJSK 指令系统技术文档

> 最后更新：2026-03-23（v1.0）

---

## 1. 系统概览

PJSK 指令系统由**两个解耦层**组成，共同实现"Bot 原始文本消息 → PNG 图片"的完整链路：

```
用户消息（原始文本）
        │
        ▼
┌──────────────────────────────┐
│  Layer 1：指令解析层          │  internal/pjsk/parser/
│  GlobalCommandResolver        │  ← 正则路由 + 特征提取 + 类型化查询解析
└──────────────┬───────────────┘
               │ ResolvedCommand
               ▼
┌──────────────────────────────┐
│  Layer 2：请求构建 & 执行层   │  internal/pjsk/handler/
│  Handler（Trie 分发）         │  ← 命令前缀匹配 + 参数提取 + 反射注册
│  Bridge（模块路由）           │  ← ResolvedCommand → Render 控制器
└──────────────┬───────────────┘
               │ []byte (PNG)
               ▼
         图片返回给 Bot
```

两层可独立调用：API 层通过 `bridge.Execute()` 直接传入 `ResolvedCommand`；ZeroBot 等 Bot 框架则走完整的 Trie 分发链路。

---

## 2. Layer 1：指令解析层（`internal/pjsk/parser/`）

### 2.1 文件清单

| 文件 | 作用 | 关键导出 |
|------|------|---------|
| `global_resolver.go` | 指令路由引擎，将原始文本解析为 `ResolvedCommand` | `GlobalCommandResolver`, `NewGlobalCommandResolver()`, `ResolvedCommand` |
| `extractor.go` | 通用特征提取器（区服、稀有度、属性等） | `Extractor`, `ExtractResult[T]` |
| `parser.go` | 卡片查询解析 | `CardParser`, `CardQueryInfo`, `QueryType` |
| `music_parser.go` | 歌曲查询解析 | `MusicParser`, `MusicQueryInfo`, `MusicQueryType` |
| `event_parser.go` | 活动查询解析 | `EventParser`, `EventQueryInfo`, `EventFilter`, `EventQueryType` |
| `command_parser.go` | 事件数据库指令解析（SK/绑定） | `CommandParser`, `EventCommand`, `CommandType` |
| `utils.go` | 工具函数 | `isNumeric()` |

---

### 2.2 GlobalCommandResolver

#### 构造方式

```go
func NewGlobalCommandResolver(nicknames map[string]int) *GlobalCommandResolver
```

`nicknames` 是角色昵称→角色 ID 的映射，由 `chardata.Loader` 从 Sekai DB 加载并定期刷新。

#### Resolve() 执行流程

```
输入字符串
  1. TrimSpace
  2. 空字符串 → 返回 {Module: ModuleHelp, IsHelp: true}
  3. ExtractRegionPrefix()  → 检测 "/cn/event-list" 格式
  4. ExtractRegion()        → 检测 "-r cn" 格式
  5. ExtractVerbose()       → 检测 "-v/--verbose"
  6. ExtractPreview()       → 检测 "-p/--preview"
  7. ExtractHelp()          → 检测 "-h/--help/帮助"
  8. 区服 fallback → "jp"
  9. 遍历 globalRoutes（46 条正则），首个匹配的 → Module + Mode + Query
 10. 返回 *ResolvedCommand
```

#### ResolvedCommand 结构体

```go
type ResolvedCommand struct {
    Module    TargetModule    // 目标模块枚举
    Mode      string          // 功能模式，如 "card-detail"、"music-list"
    Query     string          // 路由提取后的剩余文本（供下游解析）
    Region    string          // 区服代码：jp / en / cn / tw / kr（默认 jp）
    Params    json.RawMessage // 可选 JSON 附加参数
    IsHelp    bool            // 是否请求帮助
    IsVerbose bool            // 是否详细模式
    IsPreview bool            // 是否预览模式
}
```

---

### 2.3 TargetModule 枚举（13 个模块）

```go
const (
    ModuleUnknown    TargetModule = iota // 0
    ModuleCard                           // 1  查卡
    ModuleGacha                          // 2  卡池
    ModuleMusic                          // 3  音乐
    ModuleEvent                          // 4  活动
    ModuleDeck                           // 5  组卡
    ModuleSK                             // 6  SK 冲分
    ModuleMysekai                        // 7  烤森
    ModuleProfile                        // 8  个人中心
    ModuleHelp                           // 9  帮助
    ModuleEducation                      // 10 养成
    ModuleScore                          // 11 分数控制
    ModuleStamp                          // 12 贴纸
    ModuleMisc                           // 13 杂项
)
```

---

### 2.4 正则路由表（46 条）

路由在 `init()` 中初始化，采用大小写不敏感匹配（`(?i)`），每条包含：
- `pattern *regexp.Regexp`：模式，捕获组 2 为 Query
- `module TargetModule`：目标模块
- `mode string`：功能模式

| 模块 | 模式数 | 典型命令前缀 | 模式名 |
|------|--------|------------|--------|
| Card | 3 | `/查卡`、`/卡面`、`/查箱` | card-detail / card-list / card-box |
| Gacha | 1 | `/卡池`、`/gacha` | gacha |
| Music | 5 | `/查曲`、`/谱面`、`/打歌进度`、`/曲目奖励`、`/歌曲列表` | music-detail / music-chart / music-progress / music-rewards / music-list |
| Deck | 5 | `/组卡`、`/挑战组卡`、`/长草组卡`、`/加成组卡`、`/烤森组卡` | deck-event / deck-challenge / deck-no-event / deck-bonus / deck-mysekai |
| Event | 2 | `/活动`、`/活动列表` | event-detail / event-list |
| Education | 5 | `/挑战赛`、`/加成信息`、`/区域道具`、`/羁绊`、`/加成统计` | education-challenge / education-power / education-area / education-bonds / education-leader |
| Score | 4 | `/分数`、`/自定义房间分数`、`/曲目meta`、`/曲目榜` | score-control / score-custom-room / score-music-meta / score-music-board |
| Stamp | 1 | `/贴纸` | stamp-list |
| Misc | 1 | `/角色生日` | misc-birthday |
| SK | 7 | `/sk线`、`/sk查询`、`/查房`、`/sk时速`、`/玩家轨迹`、`/档线轨迹`、`/胜率` | sk-line / sk-query / sk-check-room / sk-speed / sk-player-trace / sk-rank-trace / sk-winrate |
| Mysekai | 6 | `/烤森资源`、`/家具列表`、`/家具详情`、`/大门升级`、`/唱片`、`/对话列表` | mysekai-resource / mysekai-fixture-list / mysekai-fixture-detail / mysekai-door-upgrade / mysekai-music-record / mysekai-talk-list |
| Profile | 2 | `/sk`（前缀）、`/个人中心` | profile |
| Help | 1 | `/help`、`/帮助` | help |

> 注：路由匹配采用首匹配原则，顺序即优先级。列表类路由放在详情类前（如 music-list 优先于 music-detail）。

---

### 2.5 Extractor（通用特征提取器）

`Extractor` 是所有解析器的共享基础组件，提供 11 种提取方法：

```go
type ExtractResult[T any] struct {
    Value     T       // 提取到的值
    Remaining string  // 移除匹配后的剩余文本
    Found     bool    // 是否找到
}
```

| 方法 | 提取内容 | 工作方式 | 示例 |
|------|---------|---------|------|
| `ExtractCharacter()` | 角色 ID（int） | 从 nicknames map 最长匹配 | "mnr" → 24 |
| `ExtractRarity()` | 稀有度字符串 | 字典规则："4星"→"rarity_4"，"生日"→"rarity_birthday" | |
| `ExtractAttribute()` | 属性字符串 | 字典规则：5 种属性（cute/cool/pure/happy/mysterious），中英文别名 | "粉"→"cute" |
| `ExtractSkill()` | 技能类型 | 字典规则："分"→"score_up"，"奶"→"life_recovery" | |
| `ExtractSupply()` | 供给类型 | 字典规则："fes"→"festival"，"限定"→"limited" | |
| `ExtractRegionPrefix()` | 区服（前缀方式） | 检测 `/region` 开头，如 `/cn/event` | |
| `ExtractRegion()` | 区服（Flag 方式） | 匹配 `-r jp` | |
| `ExtractYear()` | 年份（int） | 全形"2024年"、短形"24年"、相对"今年"/"去年" | |
| `ExtractHelp()` | 帮助标志 | `-h`、`--help`、`帮助` | |
| `ExtractVerbose()` | 详情标志 | `-v`、`--verbose` | |
| `ExtractPreview()` | 预览标志 | `-p`、`--preview` | |
| `ExtractID()` | 纯数字 ID | `^\s*(\d+)\s*$` | |

**规则构建机制：**
- `buildRules(map[string]string)` 按键长度降序排列（最长优先匹配）
- ASCII 键自动添加单词边界 `\b`，非 ASCII 不添加
- 首个匹配的规则胜出

---

### 2.6 类型化解析器

#### CardParser

**用途：** 将 `r.Query` 解析为卡片查询意图。

**解析优先级：**
1. 昵称+负数序号（`mnr-1`）→ Seq 类型
2. 纯数字 ID（`190`）→ ID 类型
3. 多条件过滤（`mnr 4星 粉 分`）→ Filter 类型

**输出：CardQueryInfo**

```go
type CardQueryInfo struct {
    Type        QueryType  // ID / Seq / Filter
    Value       int        // 卡片 ID（ID 类型）
    Sequence    int        // 负序号（Seq 类型，如 -1）
    CharacterID int        // 角色 ID
    Rarity      string     // "rarity_3"/"rarity_4"/"rarity_birthday"
    Attr        string     // "cute"/"cool"/"pure"/"happy"/"mysterious"
    SkillType   string     // "score_up"/"perfect_score_up"/"judgment_accuracy_up"/"life_recovery"
    SupplyType  string     // "normal"/"limited"/"festival"/"birthday"
    Year        int        // 发布年份
    Original    string     // 原始输入
}
```

---

#### MusicParser

**用途：** 将 `r.Query` 解析为歌曲查询意图。

**解析优先级：**
1. `extractDiff()`：提取谱面难度（easy/ez/绿谱→easy，normal/hard/expert/master/append 等）
2. 数字 ID 或 "id" 前缀 → ID 类型
3. 负索引（`-1`）→ Seq 类型
4. "event" 前缀 → Event 类型
5. 昵称+数字（`mnr1`）→ Ban 类型
6. 其余 → Title 关键词搜索

**输出：MusicQueryInfo**

```go
type MusicQueryInfo struct {
    Type       MusicQueryType // ID / Seq / Event / Ban / Title / Chart
    Value      int            // ID 或索引或活动 ID
    Diff       string         // 难度
    Keyword    string         // 标题或别名
    BanCharID  int            // 角色 ID（Ban 类型）
    BanSeq     int            // 序号（Ban 类型）
    Original   string
}
```

---

#### EventParser

**用途：** 将 `r.Query` 解析为活动查询意图。

**解析优先级：**
1. 数字 ID 或 "event" 前缀 → ID 类型
2. 昵称+数字（`mnr2`）→ Ban 类型
3. "next"/"prev"/"current"/负索引 → Seq 类型
4. 多条件过滤 → Filter 类型

**过滤字段（EventFilter）：**

```go
type EventFilter struct {
    Unit        string  // ln/mmj/vbs/wxs/25h/vs/mix → 对应单位英文名
    EventType   string  // marathon/5v5→cheerful_carnival/wl→world_bloom
    Year        int     // 活动年份
    CharacterID int
    Attr        string
}
```

---

#### CommandParser（EventCommand）

**用途：** 解析 SK 查分及绑定类指令。

| 类型 | 触发条件 | 说明 |
|------|---------|------|
| `CmdTypeEventQuerySelf` | 空 / "sk" | 查自己 |
| `CmdTypeEventQueryAt` | "@123456" | @某人 |
| `CmdTypeEventQueryUID` | 10 位以上数字 | 按游戏 UID 查 |
| `CmdTypeEventQueryRank` | 1–9 位数字 | 按榜位查 |
| `CmdTypeEventQueryRankRange` | "100-200" | 查榜位区间 |
| `CmdTypeEventQueryMultiRank` | "1 2 3" | 查多个榜位 |
| `CmdTypeBind` | "bind 350..." | 绑定 UID |
| `CmdTypeUnbind` | "unbind" | 解绑 |

---

### 2.7 区服处理

**支持区服：** `jp`、`en`、`cn`、`tw`、`kr`

**优先级（高→低）：**
1. 路径前缀：`/cn/event-list` → region="cn"，输入改写为 `/event-list`
2. Flag：`/event-list -r tw` → region="tw"
3. 默认：`"jp"`

---

### 2.8 Chardata 集成（角色昵称加载）

**文件：** `internal/pjsk/chardata/loader.go`

```
Sekai DB（sekai.Client）
  ↓ 查询 game_characters + game_character_units
  ↓ 构建昵称变体（firstName / givenName / fullName / unitName）
  ↓ 存入 map[string]int（昵称 → game_id）

Loader.Nicknames() → 返回深拷贝（线程安全 RWMutex）
  ↓
NewGlobalCommandResolver(nicknames) / NewCardParser(nicknames) 等
```

- 每个角色最多生成 4 种昵称变体
- 后台定时刷新（`StartBackgroundRefresh`），默认 24h，不中断服务
- 未加载时返回 `nil`（解析仍工作，只是无角色昵称匹配）

---

## 3. Layer 2：请求构建 & 执行层（`internal/pjsk/handler/`）

### 3.1 文件清单

| 位置 | 文件 | 作用 |
|------|------|------|
| 顶层 | `handler.go` | Trie 分发器，命令注册与匹配 |
| 顶层 | `bridge.go` | `Execute()` — 将 ResolvedCommand 路由到渲染控制器 |
| 顶层 | `context.go` | HandlerContext 接口定义 |
| `sekai/` | `handler.go` | SekaiCommandHandler + 反射自动注册 |
| `sekai/` | `helpers.go` | `makeResolvedCmd` / `makeResolvedCmdWithParams` |
| `sekai/` | `card.go` | 卡片功能（7 个 Handler） |
| `sekai/` | `event.go` | 活动功能（5 个 Handler） |
| `sekai/` | `music.go` | 音乐功能（13 个 Handler） |
| `sekai/` | `gacha.go` | 卡池功能（2 个 Handler） |
| `sekai/` | `deck.go` | 组卡功能（6 个 Handler） |
| `sekai/` | `education.go` | 养成功能（5 个 Handler） |
| `sekai/` | `entertainment.go` | 娱乐功能（7 个，均已禁用） |
| `sekai/` | `misc.go` | 杂项功能（8 个 Handler） |
| `sekai/` | `profile.go` | 个人中心（27+ Handler，大部分禁用） |
| `sekai/` | `score.go` | 分数控制（4 个 Handler） |
| `sekai/` | `sk.go` | SK 功能（12 个 Handler） |
| `sekai/` | `stamp.go` | 贴纸功能（8 个 Handler，部分禁用） |
| `sekai/` | `mysekai.go` | 烤森功能（10 个 Handler） |
| `sekai/` | `vlive.go` | 虚拟 LIVE（1 个，已禁用） |
| `sekai/` | `chart.go` | 谱面查询（1 个 Handler） |

---

### 3.2 Trie 分发器（`handler.go`）

#### 数据结构

```
commandHandlerTree（根节点）
├── '/' → node
│   ├── '查' → node
│   │   ├── '卡' → [handler: CardListHandle, priority=100]
│   │   └── '曲' → [handler: MusicDetailHandle, priority=100]
│   └── ...
└── ...（26 个字母 + 中文 rune 展开）
```

#### 关键类型

```go
type CommandHandler interface {
    IsDisabled() bool
    GetCommands() []string   // 支持的命令前缀列表
    GetPriority() int        // 默认 100，越大优先级越高
    GetHelper() string       // 帮助文本
    Handle(Context) (interface{}, error)
}
```

#### 匹配规则
- **分隔符**（空格、`-`、`_`、`.`）作为命令边界，跳过不消费
- **贪心匹配**：优先最长匹配
- **优先级碰撞**：新注册 > 旧注册时替换（stderr 警告）
- **结果**：返回 `matchedHandler`（Handler + 剩余参数 ArgText）

---

### 3.3 SekaiCommandHandler 反射注册

`RegisterSekaiCommandHandler()` 通过反射扫描 `sekaiHandlers{}` 结构体上所有签名为 `func() SekaiCommandHandler` 的方法，自动注册为命令处理器。

#### 命令扩展规则

```
原始命令: ["/查卡"]
区服: [JP, CN, TW, KR, EN]
前缀参数 PrefixArgs: ["", "wl"]

生成命令:
  /查卡
  /jp/查卡    /jp/wl/查卡
  /cn/查卡    /cn/wl/查卡
  /tw/查卡    /tw/wl/查卡
  /kr/查卡    /kr/wl/查卡
  /en/查卡    /en/wl/查卡
```

#### SekaiHandlerContext（增强上下文）

```go
type SekaiHandlerContext struct {
    handler.HandlerContext
    region             renderregion.Value  // 解析出的区服
    originalTriggerCmd string              // 剥离区服前缀前的命令
    prefixArg          string              // 如 "wl"
    uidArg             string              // 平台用户 ID
    flags              map[string]bool     // "is_verbose", "is_preview", "is_help"
}
```

---

### 3.4 Bridge（`bridge.go`）

#### 核心函数

```go
func Execute(ctx context.Context, resolved *parser.ResolvedCommand, app *renderapp.App) ([]byte, error)
```

**执行流程：**
1. nil 检查（resolved / app）
2. `switch resolved.Module` → 调用对应 `executeXxx(resolved, app)`
3. `executeXxx` 内部 `switch resolved.Mode` → 构建类型化 Query
4. 调用 `mergeParams(r.Params, &q)` 将 JSON Params 反序列化到 Query 结构体
5. 调用对应 Render 控制器方法 → 返回 `[]byte` PNG

#### 模块 → 渲染控制器映射

| 模块 | Render 控制器 | 支持模式 |
|------|-------------|---------|
| Card | `app.Cards` | card-detail / card-list / card-box |
| Event | `app.Events` | event-detail / event-list / event-record |
| Music | `app.Music` | music-detail / music-list / music-chart / music-progress / music-rewards |
| Gacha | `app.Gachas` | gacha |
| Deck | `app.Decks` | deck-event / deck-challenge / deck-no-event / deck-bonus / deck-mysekai |
| Education | `app.Edu` | education-challenge / education-power / education-area / education-bonds / education-leader |
| SK | `app.SK` | sk-line / sk-query / sk-check-room / sk-speed / sk-player-trace / sk-rank-trace / sk-winrate |
| Score | `app.Score` | score-control / score-custom-room / score-music-meta / score-music-board |
| Profile | `app.Profiles` | profile |
| Mysekai | `app.MySekai` | mysekai-resource / mysekai-fixture-list / mysekai-fixture-detail / mysekai-door-upgrade / mysekai-music-record / mysekai-talk-list |
| Stamp | `app.Stamps` | stamp-list |
| Misc | `app.Misc` | misc-birthday |

#### mergeParams 机制

```go
func mergeParams(params json.RawMessage, target interface{}) {
    if len(params) == 0 { return }
    _ = json.Unmarshal(params, target)  // 静默忽略错误；无关字段跳过
}
```

Handler 将额外参数（如难度、显示选项）序列化为 JSON 存入 `r.Params`，Bridge 在构建 Query 后调用 `mergeParams` 将其反序列化到对应字段。

---

### 3.5 请求构建完整示例

#### 示例 1：查卡（带难度的谱面查询）

```
用户输入: "/谱面预览 mnr master"

① GlobalCommandResolver.Resolve("/谱面预览 mnr master")
   → Module=ModuleMusic, Mode="music-chart", Query="mnr master", Region="jp"

② API 层调用 bridge.Execute(ctx, resolved, app)

③ bridge.executeMusic:
   case "music-chart":
       q := music.ChartQuery{Region: "jp"}
       // Query "mnr master" 中 "master" 被 handler 层提取为 Params
       mergeParams(r.Params, &q)   // q.Difficulty = "master"
       // Query 剩余 "mnr" 作为关键词
       q.Query = r.Query
       return app.Music.RenderMusicChart(q)

④ 返回 PNG bytes → API 响应
```

#### 示例 2：活动组卡

```
用户输入: "/cn/活动组卡 150"

① GlobalCommandResolver:
   ExtractRegionPrefix → Region="cn", 输入改写为 "/活动组卡 150"
   → Module=ModuleDeck, Mode="deck-event", Query="150", Region="cn"

② bridge.executeDeck:
   case "deck-event":
       q := deck.AutoQuery{Region: "cn", RecommendType: "event"}
       mergeParams(r.Params, &q)   // 可能包含目标活动 ID
       return app.Decks.RenderAutoRecommend(q)
```

---

### 3.6 错误处理

```
Handler.Handle() → 返回 (nil, error)
  ↓
bridge.Execute() → 检查错误，返回 (nil, error)
  ↓
API 层（api/bot/pjsk/handler.go）
  - 解析错误    → 400 + BotCommandErrorResponse
  - 模块不匹配  → 400 + ExpectedModule/ExpectedMode
  - 渲染失败    → 500 + BotCommandErrorResponse
  - 成功        → 200 + Content-Type: image/png + PNG bytes
```

**常见错误场景：**
- `"请输入要查询的卡牌"` — Handler 参数为空
- `"TODO: 抽卡记录未实现"` — 已注册但未实现的 Handler
- `"bridge: unsupported module"` — 模块枚举值超出范围
- `"bridge: nil resolved command"` — 上游解析失败未检查

---

## 4. 功能 Handler 汇总表

### 活跃 Handler（已启用）

| 模块 | Handler | 命令示例 | Bridge 模式 |
|------|---------|---------|------------|
| Card | CardDetailHandle / CardListHandle | `/查卡`、`/card` | card-detail / card-list / card-box |
| Card | CardBoxHandle | `/查箱`、`/box` | card-box |
| Event | EventHandle | `/活动列表`、`/events` | event-list |
| Event | EventDetailHandle | `/活动`、`/查活动` | event-detail |
| Event | EventRecordHandle | `/活动记录` | event-record |
| Music | MusicDetailHandle | `/查曲`、`/music` | music-detail |
| Music | MusicListHandle | `/歌曲列表`、`/定数表` | music-list |
| Music | MusicRewardsHandle | `/曲目奖励`、`/打歌挖矿` | music-rewards |
| Music | MusicProgressHandle | `/打歌进度` | music-progress |
| Music | ChartHandle | `/谱面`、`/查谱` | music-chart |
| Gacha | GachaHandle | `/卡池`、`/查卡池` | gacha |
| Deck | Event/Challenge/NoEvent/Bonus/MysekaiDeckHandle | `/组卡`、`/烤森组卡` | deck-* |
| Education | Challenge/PowerBonus/AreaItem/Bonds/LeaderCount | `/挑战赛`、`/羁绊` | education-* |
| Score | ScoreControlHandle | `/分数`、`/控分` | score-control |
| Score | MusicMetaHandle / MusicBoardHandle | `/曲目meta`、`/曲目榜` | score-music-meta / score-music-board |
| SK | Line/Query/Speed/CheckRoom/PlayerTrace/RankTrace/Winrate | `/sk线`、`/查房`、`/胜率` | sk-* |
| Mysekai | Resource/FixtureList/FixtureDetail/DoorUpgrade/MusicRecord/TalkList | `/烤森资源`、`/msf` | mysekai-* |
| Stamp | StampHandle | `/贴纸`、`/stamp` | stamp-list |
| Misc | MiscBirthdayHandle | `/角色生日` | misc-birthday |
| Profile | ProfileHandle / ProfileInfoHandle | `/个人中心`、`/profile` | profile |

### 禁用 Handler（`Disabled: true`）

| 模块 | Handler 数 | 说明 |
|------|-----------|------|
| Entertainment | 7 | 娱乐小游戏（猜谱面/猜卡面/抽卡等），整体禁用 |
| Profile (管理) | ~20 | 绑定/解绑/隐藏 ID/上传背景等，业务逻辑未完成 |
| Stamp (扩展) | 6 | 制作贴纸/随机贴纸/刷新，功能未完善 |
| Mysekai (扩展) | 4 | 蓝图、拍照、数据检查、绑定切换 |
| Gacha (记录) | 1 | 抽卡记录，TODO |
| Vlive | 1 | 虚拟 LIVE 列表 |

---

## 5. 与 API 层的接入

### 5.1 通用端点（`api/legacy/pjsk/command.go`）

```
POST /internal/pjsk/command
Body: {"command": "...", "server": "jp"}

→ GlobalCommandResolver.Resolve(command)
→ bridge.Execute(ctx, resolved, renderApp)
→ image/png 响应
```

直接使用 `GlobalCommandResolver` 做路由，适合内部服务调用（ZeroBot 等）。

### 5.2 功能端点（`api/bot/pjsk/handler.go`）

```
GET|POST /api/v2/bot/:botId/pjsk/card/detail?command=<Base64>

→ parseBotRequest() → decodeCommand() → Base64 → OneBot JSON → 文本
→ resolver.Resolve(text) → ResolvedCommand
→ 校验 Module+Mode 与端点匹配
→ bridge.Execute(ctx, resolved, renderApp)
→ image/png 响应
```

额外增加了 Module/Mode 端点匹配校验（防止前缀映射错误端点）。

### 5.3 Trie 路径（已注册但未对外暴露 API）

`sekai/handler.go` 的 Trie 分发路径目前仅在集成测试中使用，未直接挂载到 HTTP 路由。其 `ResolvedCommand` 输出格式与 `GlobalCommandResolver` 完全兼容，可通过 `bridge.Execute()` 统一执行。

---

## 6. 测试覆盖

| 包 | 测试文件 | 覆盖内容 |
|----|---------|---------|
| `parser` | `parser_test.go` | 区服前缀提取（10 用例）、预览 Flag（4 用例）、帮助路由（6 用例）、完整路由分发（4 用例） |
| `handler/sekai` | 各模块 `*_test.go` | Handler 反射注册、命令前缀生成、区服扩展 |
| `api/bot/pjsk` | `handler_test.go` | Base64 解码、OneBot JSON 解析、端点匹配、模式拒绝 |

**覆盖不足的部分：**
- `CardParser`、`MusicParser`、`EventParser`、`CommandParser` 的类型化解析逻辑无单元测试
- `Extractor` 的稀有度/属性/技能/供给/年份提取无单元测试（仅区服和 Flag 有测试）

---

## 7. 设计原则小结

| 原则 | 体现 |
|------|------|
| **关注点分离** | 路由匹配（parser）与执行（bridge）完全解耦；可独立替换 |
| **最长优先匹配** | 昵称提取、Trie 匹配均采用贪心最长匹配 |
| **静默降级** | `mergeParams` 忽略 JSON 错误；`decodeCommand` 失败降级为纯文本 |
| **线程安全** | chardata.Loader 使用 RWMutex；Trie 使用 treeMutex |
| **可扩展** | 新增模块只需：① 加正则路由 ② 写 SekaiCommandHandler ③ 在 bridge 中加 case |
| **区服感知** | 所有 Query 类型均携带 Region 字段，渲染控制器使用区服选择数据源 |

---

## 8. 相关文档索引

| 文档 | 内容 |
|------|------|
| `docs/architecture.cn.md` | 项目整体架构 |
| `docs/project-status-summary.cn.md` | 当前进度与待办 |
| `docs/database-schemas.cn.md` | 数据库 Schema 详解 |
| `docs/utils-query.cn.md` | `utils/query` 跨 DB 查询门面 |

---

**维护者**：Haruki-Cloud Team  
**文档版本**：v1.0  
**创建日期**：2026-03-23
