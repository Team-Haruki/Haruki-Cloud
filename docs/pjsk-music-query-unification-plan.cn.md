# Haruki-Cloud PJSK 歌曲查询统一改造方案

> 最后更新：2026-03-29
>
> 本文只记录“歌曲查询相关语义”的统一改造方案，用于后续逐项落地与回归，不代表这些改动已经全部实现。

## 1. 文档目的

当前 `Haruki-Cloud` 中，凡是接受“曲名 / 歌曲别名 / 难度 / 特殊查歌语法”的入口，都可以视为“歌曲查询相关”。

这类逻辑已经不再只是 `music/*` 模块独有，至少还涉及：

1. `score/music-meta`
2. `score/music-board`
3. `deck` 中的指定歌曲参数
4. `music/chart`
5. `music/bpm`
6. `music/cover`
7. `music/list` 的关键词筛选

目前这些入口虽然大多已经接到了正式的歌曲 alias 解析链，但“参数语义”仍然分散在 `handler` 和 `render/music` 两层，导致同样的输入在不同入口表现并不一致。

本文档的目标是：

1. 把现状和问题点收口成一份清单。
2. 明确后续统一到哪一层。
3. 给出分步落地顺序，避免边修边忘。

## 2. 当前现状

### 2.1 主链已经存在，但没有完全统一

当前真正的歌曲检索主链在 `render/music` 中：

1. `internal/pjsk/render/music/parser.go`
2. `internal/pjsk/render/music/search.go`
3. `internal/pjsk/render/music/controller.go`
4. `internal/pjsk/alias/service.go`

大致顺序是：

1. `handler` 把原始文本透传给 bridge / controller。
2. `render/music.Parser` 尝试解析歌曲查询类型。
3. `SearchService` 按解析结果决定查找方式。
4. `titleResolver` 先尝试正式歌曲 alias，再回退到 masterdata 搜索。

这条链已经覆盖：

1. `music-detail`
2. `music-chart`
3. `music-bpm`
4. `music-cover`
5. `score/music-meta`
6. `score/music-board`
7. `music-list` 的 keyword 过滤

### 2.2 现有入口分布

目前可以按“是否接受歌曲查询文本”分成三类：

| 分类 | 入口 | 当前情况 |
|------|------|----------|
| 单曲查询 | `music-detail` `music-chart` `music-bpm` `music-cover` | 已走统一查歌主链，但部分入口仍在 handler 前置拆难度 |
| 多曲查询 | `score/music-meta` `score/music-board` | 已走统一查歌主链，但排行参数抽取仍与 refer 不一致 |
| 业务复用查询 | `deck` 指定歌曲、`music-list` keyword | 已接 alias 链，但语义和其他入口还未完全统一 |

### 2.3 当前已经统一的部分

当前可以确认已经统一的能力有：

1. 歌曲 alias 由 `alias.Service.TryResolveMusicID(...)` 提供。
2. 歌曲正式标题与已审核 alias 的解析顺序已经固定。
3. `handler` 层不再提前把歌曲名硬转成 `music id`。
4. `score/music-meta`、`score/music-board`、`deck` 都已经接入正式歌曲 alias 主链。

### 2.4 当前还不统一的部分

当前最主要的不一致集中在三类语义：

1. 难度提取
2. 排行参数抽取
3. 关注歌曲 / 指定歌曲的组合语法

## 3. 当前问题清单

### 3.1 难度提取存在多套实现

目前至少有三套和“歌曲难度”相关的逻辑：

1. `internal/pjsk/handler/sekai/music.go` 中的 `extractMusicDifficulty(...)`
2. `internal/pjsk/render/music/parser.go` 中的 `Parser.extractDiff(...)`
3. `refer/music.py` / `refer/score.py` 中的 refer 语义

它们现在的差异包括：

1. 支持的别名集合不一致
2. 是否支持中文别名不一致
3. 是否支持内嵌难度写法不一致
4. `handler` 与 `render` 会重复提取
5. 现有 `parser.extractDiff(...)` 中还存在映射错误，`红` 被映射成了 `master`

### 3.2 handler 层和 render 层职责仍然交叉

当前不少入口会在 `handler` 层先做一次简化版参数处理，再把剩余字符串交给 `render/music`：

1. `music-detail`
2. `music-list`
3. `music-progress`
4. `music-bpm`
5. `deck` 指定歌曲

这样的问题是：

1. 相同输入在不同入口可能得到不同结果。
2. 后续修一处很容易漏另一处。
3. refer 中允许的写法，不一定能在当前 Cloud 全部入口复现。

### 3.3 `music-board` 的参数抽取还没对齐 refer

当前 `score/music-board` 在 `handler/sekai/score.go` 中做了一版 token 级解析，但和 refer 的真实语义仍有差异：

1. refer 是按顺序一轮轮从 `args` 中剥离参数，不是纯 token 匹配。
2. refer 的 `extract_param_from_args(...)` 是“长度优先的子串匹配”。
3. refer 的 `extract_diff(...)` 也是“长度优先的子串匹配”。
4. 当前 Cloud 还没有按 refer 实现技能组抽取。
5. 当前 Cloud 还没有实现 `*` 表示“关注歌曲全难度展开”的语义。
6. 当前 Cloud 的关注歌曲难度提取还不是 refer 那种内嵌式写法。

### 3.4 `music-meta` 与 `music-board` 的多曲语义并不一致

当前：

1. `music-meta` 用 `/` 或 `|` 拆分多首歌曲。
2. `music-board` 则先抽参数，再把剩余部分再做 `/` 或 `|` 切分。

这本身不是错误，但后续需要明确两者是否继续保持差异：

1. `music-meta` 目标是“显式比较多首歌”
2. `music-board` 目标是“在排行结果中额外关注若干歌曲”

因此建议：

1. 保留两者不同的外层语法
2. 统一两者内部对“单条歌曲查询文本”的解释方式

### 3.5 帮助文本与真实实现存在偏差

当前至少有一类明显偏差：

1. `music/chart` 的帮助文本写了 `-1leak`
2. 当前 parser 实际只支持 `-1`

这种问题说明：

1. 歌曲查询语义没有形成单一权威定义
2. 文案和实现容易分叉

## 4. 统一改造目标

后续应把“歌曲查询语义”分成两层：

### 4.1 第一层：公共歌曲查询语义层

这层只负责解释一段“歌曲查询文本”是什么意思，输出标准结构。

建议能力包括：

1. 歌曲标识类型解析
   - 显式歌曲 ID：`music123`
   - 显式歌曲 ID + 难度：`music123ex`
   - 纯数字 `123`
   - `id123`
   - 负序号 `-1`
   - 活动 `event123`
   - ban / 箱活序号
   - 曲名
   - 已审核歌曲 alias
2. 难度提取
   - 统一别名集合
   - 统一优先级
   - 统一中文别名支持
   - 支持 refer 风格的内嵌难度
3. 查询结果结构
   - 原始文本
   - 清洗后的查询文本
   - 解析出的 `difficulty`
   - 查询类型
   - 对应 `music id` 或 keyword

这里需要额外区分两类语义：

1. `music<id>` / `music<id><diff>` 是跨入口可复用的“显式歌曲 ID”语法。
2. 裸数字 `123` / `id123` 是否代表歌曲 ID，要看当前入口是不是“纯歌曲相关”场景。

这层应该尽量放在 `render/music`，因为：

1. 这里本来就是歌曲搜索主链所在位置
2. `handler` 不应该关心 alias 解析和最终查歌语义
3. `deck`、`score`、`music` 都可以复用

### 4.2 第二层：各业务入口的参数语法层

这层只负责解释某个命令自己的语法，不负责决定“怎么查歌”。

例如：

1. `music-detail`
   - 只需要提取“是否显式带难度”
2. `music-board`
   - 需要提取页码、live type、target、strategy、技能、过滤器、关注歌曲
3. `deck`
   - 需要提取“指定歌曲”和“挑战角色 / 活动角色”等其他业务字段

规则应当是：

1. 业务入口负责拆“命令参数结构”
2. 公共歌曲查询层负责解释“单条歌曲查询文本”

## 5. 具体方案

### 5.1 抽一个统一的歌曲查询 helper

建议新增一组公共 helper，优先放在 `internal/pjsk/render/music` 内部，避免 `handler` 层再维护平行语义。

建议至少拆出下面几类能力：

1. `ExtractMusicDifficulty(text string, mode ...)`
   - 统一歌曲难度别名
   - 支持内嵌匹配
   - 返回 `difficulty + cleanedText`
2. `ParseMusicLookupQuery(text string, options ...)`
   - 统一解释一条歌曲查询文本
   - 供 `detail/chart/bpm/cover/meta/board/deck` 共用
3. `SplitMusicCompareQueries(text string)`
   - 给 `music-meta` 这种“多首歌对比”用
   - 统一支持 `/`、`|`、换行
4. `ParseMusicBoardArgs(text string)`
   - 专门给 `music-board` 用，但内部仍调用公共歌曲 helper

这里的关键不是 helper 名字，而是只保留一套权威语义。

### 5.2 统一难度别名表

后续应只有一份权威的难度别名定义。

建议支持至少这些标准归一值：

1. `easy`
2. `normal`
3. `hard`
4. `expert`
5. `master`
6. `append`

建议统一接纳的别名分两类：

1. 英文短写
   - `ez` `nm` `hd` `ex` `exp` `ma` `mas` `app` `apd`
2. 中文常用写法
   - `绿谱` `蓝谱` `黄谱` `红谱` `紫谱` `粉谱`

这里需要特别注意：

1. 不再让 handler 和 render 各自维护不同 alias 集。
2. 明确 `红谱 -> expert`，不能再映射错。
3. 不保留 `红`、`紫` 这类过短别名，避免误伤曲名正文。

### 5.3 统一“单条歌曲查询文本”的解释规则

建议统一成下面的优先级：

1. 先抽难度
2. 先判断是否是显式歌曲 ID 语法
   - `music123`
   - `music123ex`
3. 再按当前入口上下文判断其他特殊查询类型
   - `id123`
   - `123`
   - `-1`
   - `event123`
   - ban / 箱活
4. 如果不是特殊查询，则作为曲名 / alias 查询词

需要注意：

1. “抽难度”本身也要支持 refer 风格的内嵌写法。
2. `music<id>` / `music<id><diff>` 一旦进入显式歌曲 ID 分支，如果没有匹配到歌曲，应直接报错，不再回退。
3. 对“纯歌曲相关”入口，裸 `123` / `id123` 默认先按歌曲 ID 解释；如果该 ID 不存在，再回退到名称 / alias 匹配。
4. 对 `music-board` 的关注歌曲，还需要支持 `*` 强制全难度展开。
5. 对 `deck` 这类混合语义入口，裸 `123` / `id123` 不应强行解释为歌曲 ID，因为它们可能表示其他业务含义，例如活动 ID。

### 5.4 `music-board` 专项方案

`music-board` 建议保持独立的业务参数解析器，但要改成“顺序剥离式”实现，并对齐 refer：

1. 页码
2. live type
3. target
4. ascend
5. strategy
6. skills
7. power
8. deck bonus
9. play interval
10. level filter
11. diff filter
12. spec music queries

其中：

1. `skills` 要按 refer 规则支持：
   - `solo/auto` 需要 5 个数字
   - `multi` 需要 1 个数字并复制为 5 份
   - 先清理 `技能`、`实效`
   - 数字统一按百分比除以 100
2. `spec music queries` 需要支持：
   - 默认按单条歌曲查询解释
   - `*` 表示全难度展开
   - 内嵌难度写法
3. `diff filter` 应只吃“整段就是难度”的写法，不吞掉真正的曲名

### 5.5 `music-meta` 专项方案

`music-meta` 的目标比较简单，不需要对齐 `music-board` 那么复杂的参数系统。

建议规则保持为：

1. 外层继续用 `/`、`|`、换行拆 1 到 3 首歌曲
2. 每一段内部统一交给公共歌曲查询 helper

这样能保证：

1. 多曲比较语法不变
2. 曲名 / alias / 难度语义和其他入口统一

### 5.6 `deck` 专项方案

`deck` 的“指定歌曲”仍然有业务特殊性：

1. 允许使用曲名或 alias
2. 允许使用 `music<id>` 显式指定歌曲 ID
3. 允许使用 `music<id><diff>` 同时指定歌曲 ID 与难度
4. 裸 `123` 或 `id123` 不应直接视为 `deck` 指定歌曲语法，因为在 `deck` 中它们可能表示活动 ID

因此建议：

1. `deck` 参数层先识别 `music<id>` / `music<id><diff>` 这条专属语法
2. 其余情况再回落到“曲名 / alias + 可选难度”的公共歌曲 helper
3. 对 `deck` 来说，`music<id>` 是显式歌曲 ID 入口；裸 `123`、`id123` 则继续保留给 `deck` 自己的其他参数语义
4. 不再直接调用 `handler/sekai/music.go` 里的旧 `extractMusicDifficulty(...)`

## 6. 建议落地顺序

为了减少回归面，建议按下面顺序推进：

### 阶段 1：先收口公共语义

1. 把难度别名表统一到一处
2. 修掉现有 `render/music.Parser.extractDiff(...)` 的错误映射
3. 提供统一的“单条歌曲查询文本”解析 helper
4. 给 helper 补齐单元测试

### 阶段 2：先改 `music/*` 单曲入口

优先收口：

1. `music-detail`
2. `music-chart`
3. `music-bpm`
4. `music-cover`

原因是：

1. 这些入口语义最简单
2. 适合先验证新 helper 是否稳定
3. 改完后最容易人工回归

### 阶段 3：再改 `score/music-meta`

1. 保持 `/`、`|`、换行拆分逻辑
2. 让单条 query 的解释完全复用公共 helper
3. 补上“多首歌分别带难度”的测试

### 阶段 4：最后改 `score/music-board`

这是变更量最大的部分，应单独处理：

1. 改成顺序剥离式参数提取
2. 补齐技能参数
3. 补齐 `*` 全难度展开
4. 补齐内嵌难度语义
5. 补齐 refer 对照测试

### 阶段 5：再收 `deck`

1. 把 `deck` 的指定歌曲逻辑切到统一 helper
2. 在 `deck` 参数层补上 `music<id>` / `music<id><diff>` 的显式歌曲 ID 语法
3. 保留“裸数字 / id 前缀继续走 deck 自身业务语义，不直接当作歌曲 ID”的约束
4. 补充曲名 / alias / 难度 / 显式 `music<id>` 四类组合测试

## 7. 测试建议

后续实现时，至少应覆盖下面这些回归样例：

### 7.1 单曲查询

1. `テオ`
2. `テオ ex`
3. `ex テオ`
4. `テオex`
5. `ma千本樱`
6. `id123`
7. `123`
8. `-1`
9. `event123`
10. 纯歌曲入口中，`123` / `id123` 未命中任何歌曲 ID 时，继续按名称 / alias 回退
11. 任意入口中，`music123` / `music123ex` 未命中任何歌曲 ID 时直接报错

### 7.2 多曲比较

1. `Tell Your World / 初音未来的消失`
2. `Tell Your World ex / 初音未来的消失 ma`
3. `别名A | 别名B`
4. `Tell Your World\n初音未来的消失`

### 7.3 歌曲排行

1. `多人 时速 升序 2页 ex >30`
2. `solo max 技能 120 120 120 120 120`
3. `多人 200实效`
4. `综合33w 400加成 间隔28s`
5. `ma千本樱`
6. `千本樱*`
7. `SongA / SongB`

### 7.4 deck 指定歌曲

1. 指定曲名
2. 指定歌曲 alias
3. 指定曲名 + 难度
4. `music123`
5. `music123ex`
6. `music999999` 这种不存在的显式歌曲 ID 应直接报错
7. 裸 `123` 或 `id123` 不应被误判成歌曲 ID

## 8. 需要同步更新的文档与文案

后续真开始落地时，除了改代码，还要同步检查：

1. `docs/project-status-summary.cn.md`
2. `docs/pjsk-alias-resolution-update.cn.md`
3. `music/chart` 的帮助文本
4. 其他命令帮助中的歌曲查询示例

否则会出现“实现已经更新，但文档仍然按旧语义写”的问题。

## 9. 当前建议结论

当前这条线不建议继续零散地在各个 `handler` 里补判断，而是应优先完成下面两件事：

1. 先收口一套权威的“歌曲查询公共语义层”
2. 再让 `music`、`score`、`deck` 这些入口逐个复用

只有这样，后面处理：

1. 歌曲 alias
2. 曲名 masterdata
3. 难度缩写
4. 排行关注歌曲
5. deck 指定歌曲

这些语义时，才能真正做到“一处修正，多处生效”。
