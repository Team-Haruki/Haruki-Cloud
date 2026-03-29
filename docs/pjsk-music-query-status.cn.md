# Haruki-Cloud PJSK 歌曲查询现状整理

> 最后更新：2026-03-29
>
> 本文记录当前 `Haruki-Cloud` 中“歌曲查询相关”入口的实际落地状态，用于后续排查和继续收口。

## 1. 本次扫描范围

本次只检查“接受歌曲查询文本”的入口，不包含纯数值型或纯用户数据型入口。

已检查的主链包括：

1. `music-detail`
2. `music-list` 的 `keyword`
3. `music-cover`
4. `music-bpm`
5. `music-chart`
6. `score-control`
7. `score/music-meta`
8. `score/music-board`
9. `deck` 的指定歌曲

未纳入“歌曲查询统一”范围的入口包括：

1. `music-note-count`
   - 输入的是物量数字，不是歌曲查询文本。
2. `music-progress`
   - 输入的是难度，不是歌曲查询文本。
3. `music-rewards`
   - 当前不接受歌曲参数。

## 2. 当前统一语义

### 2.1 难度别名

当前权威难度提取位于：

1. `internal/pjsk/render/music/parse_helpers.go`

当前保留的主要难度别名：

1. `easy / ez / 绿谱`
2. `normal / nm / 蓝谱`
3. `hard / hd / 黄谱`
4. `expert / ex / exp / 红谱`
5. `master / ma / mas / 紫谱`
6. `append / app / apd / 粉谱`

当前明确不再保留：

1. `红`
2. `紫`
3. 其他单字难度缩写

原因是这类别名太短，容易误伤曲名正文。

### 2.2 歌曲 ID 语义

当前统一约定如下：

1. `music<id>` / `music<id><diff>`
   - 表示显式歌曲 ID。
   - 如果 ID 不存在，应直接报错，不回退。
2. 裸 `123` / `id123`
   - 在纯歌曲相关入口中，先按歌曲 ID 解释。
   - 如果歌曲 ID 不存在，再回退到标题 / alias 解析。
3. `deck`
   - 因为存在活动 ID 等其他数字语义，所以不把裸数字直接视为歌曲 ID。
   - `deck` 中显式指定歌曲时，统一使用 `music<id>`。

### 2.3 多曲拆分语义

当前以下入口支持多曲拆分：

1. `score/music-meta`
2. `score/music-board` 的关注歌曲

当前统一支持的分隔符：

1. `/`
2. `|`
3. 换行符

### 2.4 关注歌曲语义

当前 `score/music-board` 的关注歌曲支持：

1. 标题
2. 已审核歌曲 alias
3. `music<id>`
4. `music<id><diff>`
5. 内嵌难度写法，例如 `テオex`
6. `*` 全难度展开，例如 `千本樱*`

## 3. 当前公共主链

### 3.1 解析主链

当前歌曲查询的公共解释主链位于：

1. `internal/pjsk/render/music/parse_helpers.go`
2. `internal/pjsk/render/music/parser.go`
3. `internal/pjsk/render/music/search.go`

顺序概括为：

1. 先从输入文本中提取难度
2. 再判断是否是特殊查询语法
   - `music<id>`
   - `123` / `id123`
   - `-1`
   - `event123`
   - 箱活查询
3. 否则作为标题 / alias 查询词
4. 标题 / alias 解析优先尝试正式 alias service，再回退到 masterdata

### 3.2 alias 主链

当前歌曲 alias 统一通过：

1. `internal/pjsk/alias/service.go`
2. `TryResolveMusicTitleOrAliasID(...)`

使用方统一由 `music.Controller` 注入 resolver。

## 4. 各入口现状

| 入口 | 当前状态 | 备注 |
| --- | --- | --- |
| `music-detail` | 已接入统一主链 | 通过 `ResolveMusicCover()` 走公共 parser/search |
| `music-list keyword` | 已接入统一主链 | 支持 alias，支持 `music<id>`，隐式数字按纯歌曲入口规则处理 |
| `music-cover` | 已接入统一主链 | 直接走 `ResolveMusicCover()` |
| `music-bpm` | 已接入统一主链 | 额外支持难度优先选择本地谱面 |
| `music-chart` | 已接入统一主链 | 通过 `SearchChart()` 统一解释歌曲查询 |
| `score-control` | 已接入统一主链 | 通过 `ResolveMusicMetaRequests()` 解析歌曲 |
| `score/music-meta` | 已接入统一主链 | 多曲拆分统一支持 `/`、`|`、换行 |
| `score/music-board` | 已接入统一主链 | 参数提取已改为顺序剥离式；支持 `*` 全难度展开 |
| `deck` 指定歌曲 | 已接入统一主链 | 裸数字不当歌曲 ID，显式指定用 `music<id>` |

## 5. 本次扫描发现并已修正的问题

### 5.1 `music-meta` 兜底过宽

问题：

1. `ResolveMusicMetaRequests()` 在 search 失败后会继续做关键字兜底。
2. 这会削弱 `music<id>` 的“显式 ID 失败直接报错”语义。

本次修正：

1. 只有在“标题型查询”下，才允许继续做关键字兜底。
2. 对 `music<id>` 这类结构化查询，search 失败后直接返回错误。

影响：

1. `score/music-meta`
2. `score-control`

都会一起遵守这条显式 ID 语义。

### 5.2 `music-board` 参数提取仍是旧 token 模式

问题：

1. 旧实现按空格 token 逐个扫描。
2. 关注歌曲里的内嵌难度、裸数字和技能参数容易互相抢解析权。

本次修正：

1. 改成顺序剥离式解析：
   - `page`
   - `live_type`
   - `target`
   - `ascend`
   - `strategy`
   - `skills`
   - `power`
   - `deck_bonus`
   - `play_interval`
   - `level_filter`
   - `diff_filter`
   - `spec_queries`
2. `multi` 的单个技能值现在要求带 `技能` 或 `实效` 关键词，避免误吞裸数字歌曲查询。
3. 关注歌曲支持 `*` 全难度展开。

## 6. 当前仍保留的业务差异

这些不是 bug，而是当前有意保留的入口差异：

1. `deck`
   - 不能把裸数字直接解释为歌曲 ID，因为会和活动 ID 等业务参数冲突。
2. `music-list`
   - `keyword` 既可以是标题 / alias，也可以是 `music<id>` 或隐式歌曲 ID。
3. `music-chart`
   - 仍保留负数索引、活动曲、箱活这类历史查歌语法。

## 7. 建议后续排查顺序

如果后面继续扩展歌曲查询相关能力，建议优先按下面顺序检查：

1. 先看 `render/music/parse_helpers.go`
2. 再看 `render/music/parser.go`
3. 再看 `render/music/search.go`
4. 再看具体入口是否只是“负责拆参数”，还是又自己重写了一套歌曲解释逻辑

只有当入口存在业务特殊语义时，才应在 `handler` 层保留额外解析。

## 8. 这轮扫描后的结论

截至本次扫描结束，`Haruki-Cloud` 中主要的歌曲查询入口已经基本接到同一条正式主链上。

当前更需要警惕的不是“主链缺失”，而是下面两类回归：

1. 新入口又在 `handler` 层手写一套曲名 / 难度 / ID 解析
2. 结构化查询（特别是 `music<id>`）在后续链路里被额外的兜底逻辑偷偷放宽

后续如果再做歌曲相关功能，优先复用现有 `render/music` helper，而不是重新起一套解析规则。
