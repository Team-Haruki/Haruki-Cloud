# Haruki-Cloud PJSK 组卡功能分阶段改造方案

> 最后更新：2026-04-10
>
> 本文只记录 `Haruki-Cloud` 中 PJSK 组卡命令族的改造方案、需求边界、风险点和分阶段实施计划，不代表这些改动已经全部实现。

## 1. 文档目的

当前 `Haruki-Cloud` 的组卡入口已经具备基础能力，但和帮助文档、refer 实现仍有明显差距，尤其集中在：

1. 参数面不完整。
2. WL / 挑战 / 歌曲比较语义不完整。
3. 一部分参数在 `handler` 层被剥离了，但没有真实落到后续请求中。
4. 一部分语义不是单纯的 parser 问题，而是“推荐前 profile 预处理”和“批量推荐调度”问题。

本文档的目标是：

1. 明确当前组卡功能的完整需求。
2. 明确 Cloud 现状与 refer 的差距。
3. 给出可拆分、可逐步落地的实现计划。
4. 为每一步附上明确的测试计划，避免大改后无法定位回归。

## 2. 参考内容

本方案主要参考以下内容。

### 2.1 Cloud 当前实现

- `internal/pjsk/handler/sekai/deck.go`
- `internal/pjsk/handler/sekai/deck_builder.go`
- `internal/pjsk/handler/sekai/deck_extractor.go`
- `internal/pjsk/handler/sekai/deck_extract_targets.go`
- `internal/pjsk/handler/sekai/deck_config.go`
- `internal/pjsk/handler/sekai/deck_types.go`
- `internal/pjsk/handler/sekai/deck_helpers.go`
- `internal/pjsk/handler/sekai/deck_test.go`
- `internal/pjsk/render/deck/query.go`
- `internal/pjsk/render/deck/controller_options.go`
- `internal/pjsk/render/deck/controller_request.go`
- `internal/pjsk/render/deck/controller_engine.go`

### 2.2 refer 行为

- `refer/deck.py`

重点对齐的 refer 入口包括：

1. `extract_target_event`
2. `extract_target_event_or_simulate_event`
3. `extract_random_strategy`
4. `extract_fixed_cards_and_characters`
5. `extract_card_config`
6. `extract_multilive_options`
7. `extract_music_and_diff`
8. `extract_addtional_options`
9. `extract_event_options`
10. `extract_challenge_options`
11. `extract_no_event_options`
12. `extract_bonus_options`
13. `extract_mysekai_options`

### 2.3 帮助文档

帮助文档参考：

- `lunabot_nonebot/helps/sekai.md` 第 `804` 行开始的 `## 🧮 组卡`

本地分析时使用的路径为：

- `/home/xmlq/codes/lunabot-xmlq/lunabot_nonebot/helps/sekai.md`

## 3. 改造范围

本轮方案覆盖以下命令族：

1. `/组卡` `/活动组卡`
2. `/挑战组卡`
3. `/最强组卡`
4. `/加成组卡` `/控分组卡`
5. `/烤森组卡` `/ms组卡`

同时覆盖这些链路：

1. `handler/sekai/deck_*` 参数解析。
2. `render/deck.AutoQuery` 请求结构。
3. 推荐前 option 构造。
4. 推荐前 profile 过滤 / 修改。
5. 歌曲比较的批量推荐调度。

## 4. 非目标

以下内容不应和本轮改造混在同一批次里：

1. 不重写 deck-service 引擎本身。
2. 不改 Drawing 的布局、协议或画图文案。
3. 不顺手重做整个 `render/deck` controller 架构。
4. 不把所有组卡相关历史问题一次性打包进一个超大提交。

如果后续需要修改 drawing 展示层，应另开独立文档和独立提交。

## 5. 当前现状摘要

### 5.1 当前已经具备的能力

Cloud 当前已经支持：

1. 组卡命令入口注册。
2. 基础 live 类型解析：`多人/协力`、`单人/solo`、`自动/auto`。
3. 目标解析：`综合力`、`实效/倍率/时效`。
4. 算法解析：`dfs/sa/ga/all`。
5. 歌曲名和难度的基础解析。
6. 显式歌曲 ID `music123`。
7. 活动 ID 的基础解析。
8. 模拟活动的基础团色解析。
9. 模拟 WL 的基础解析：`wl1/ wl2 + 角色`。
10. 基础协力参数：`队友综合`、`队友实效`、`实效下限`。
11. 固定卡牌 / 固定角色的基础解析。
12. 卡牌养成配置和 `bf不变`。
13. 随机策略型参数：`技能顺序平均/最大/最小`、`技能吸取平均/最大/最小`。

### 5.2 当前主要缺口

当前主要缺口集中在以下几类：

1. `顶配`、`次顶配`、`当前`、`区域道具15级`、`仅团/仅属性`、`排除卡牌`、`歌曲比较`、`技能顺序12345` 在当前解析链中未完整支持。
2. `5火` 这类体力参数目前只会被剥掉，但没有稳定落到后续 query 字段。
3. 加成组卡当前允许“纯数字活动 ID”这一行为，和帮助文档要求不一致。
4. 挑战组卡当前只支持“可选单角色”，未完整支持“无角色时全部角色各组一队”。
5. WL 的默认章节选择、非 WL 活动报错、`终章`、`25` 歧义等 refer 语义还未完整对齐。
6. `WorldBloomCharacterQuery`、`ChallengeLiveCharacterQuery`、`FixedCharacterQueries` 这类 query fallback 字段目前没有被 `render/deck` 下游真实消费，存在“解析出来但无效”的风险。
7. 歌曲比较并不是 parser 小修，它需要额外的批量推荐和结果排序逻辑。

## 6. 完整需求清单

## 6.1 命令覆盖需求

### 活动组卡

需要支持：

1. 当期活动组卡。
2. 指定活动组卡。
3. 模拟团名 + 颜色活动组卡。
4. 指定 WL 章节活动组卡。
5. 模拟 WL 活动组卡。
6. 指定歌曲 / 难度。
7. 指定体力。
8. 指定协力环境参数。
9. 指定固定卡组 / 固定角色。
10. 支持歌曲比较。

### 挑战组卡

需要支持：

1. 指定角色挑战组卡。
2. 未指定角色时，所有角色各组一队。
3. 指定歌曲 / 难度。
4. `auto` 挑战。
5. `当前` 挑战卡组。
6. 歌曲比较，但前提是必须指定角色。

### 最强组卡

需要支持：

1. 默认最大分组卡。
2. `综合` 目标。
3. `实效` 目标。
4. 单曲或默认曲目。
5. 继承通用卡牌过滤和固定队伍语义。

### 加成组卡

需要支持：

1. 当前活动目标加成组卡。
2. 显式活动 `event123` / `活动123`。
3. 多个目标加成值。
4. 不允许使用纯数字活动 ID。

### 烤森组卡

需要支持：

1. 当前活动。
2. 显式活动。
3. 模拟团色活动。
4. 固定卡牌 / 固定角色。
5. 复用 mysekai 的 live 语义，而不是普通活动 live 语义。

## 6.2 通用参数需求

### 基础参数

必须支持：

1. 歌曲名和难度。
2. `多人` `协力` `multi`。
3. `单人` `solo`。
4. `自动` `auto`。
5. `综合力` `综合`。
6. `实效` `倍率` `时效`。
7. `dfs` `sa` `ga` `all`。

### 活动相关参数

必须支持：

1. `1火` `5火` `10火`，默认 `0火`。
2. `123` `event123` `活动123`，但这条规则只适用于活动组卡，不能直接套到加成组卡。
3. `140 wl1`
4. `140 miku`
5. `mmj 绿`
6. `wl1 miku`
7. `wl2 miku`

### 协力相关参数

必须支持：

1. `实效200`
2. `队友综合37w`
3. `队友实效210`

### 卡组设置参数

必须支持：

1. `顶配`
2. `次顶配`
3. `仅mmj`
4. `仅vs`
5. `仅紫`
6. `-123`
7. `-456`
8. `当前`
9. `#123 456`
10. `#miku rin`
11. `bf不变`
12. `禁用`
13. `满破`
14. `满技能`
15. `已读`
16. `画布`
17. `四星满破满技能`
18. `123满破满技能`
19. `456禁用`
20. `区域道具15级`

### 随机因素控制参数

必须支持：

1. `技能顺序平均`
2. `技能顺序最大`
3. `技能顺序最小`
4. `技能顺序12345`
5. `技能吸取平均`
6. `技能吸取最大`
7. `技能吸取最小`

### 歌曲比较参数

必须支持：

1. `歌曲比较 龙hard 虾expert sage`
2. `歌曲比较 当前`
3. `歌曲比较 #1 2 3 4 5`
4. 固定歌曲集合时比较这些歌曲。
5. 不固定歌曲时，使用当前固定队伍比较所有歌曲。

## 6.3 各模式默认行为需求

### 活动组卡

默认行为应为：

1. 默认 `live_type = multi`。
2. 默认目标为“最大化活动 PT / 分数”。
3. 未指定活动时，取“当前活动；若当前没有则取下一个活动”。

### 挑战组卡

默认行为应为：

1. 默认 `live_type = challenge`。
2. 若用户指定 `auto`，则走 `challenge_auto`。
3. 未指定角色时，进入“每角色各组一队”模式。

### 最强组卡

默认行为应为：

1. 默认 `live_type = multi`。
2. 默认目标为分数最大。

### 加成组卡

默认行为应为：

1. 固定 `live_type = solo`。
2. 固定 `target = bonus`。
3. 未显式指定活动时，取当前活动。

### 烤森组卡

默认行为应为：

1. 固定 `live_type = mysekai`。
2. 默认走当前活动或模拟活动。

## 6.4 冲突与优先级需求

必须遵守以下规则：

1. `#固定卡牌` / `#固定角色` 必须放在所有参数最后。
2. 固定卡牌和固定角色互斥。
3. 固定角色时，第一个角色固定为队长。
4. `技能顺序12345` 只能在卡组完全固定时使用。
5. `歌曲比较` 在未限定歌曲时，必须先固定一个卡组。
6. 挑战组卡做歌曲比较时，必须先指定角色。
7. 团名 + 颜色模拟活动必须同时存在，不能只给一半。
8. 加成组卡不允许用纯数字指定活动。
9. `123 ex` 这类输入不能被错误解释成活动 `123`。
10. `25` 既可能是活动，也可能是 `25时` 团名，需要按 refer 规则处理歧义。

## 7. 关键注意点

## 7.1 不要把“解析支持”误当成“功能已完成”

当前 deck 相关链路至少分成四层：

1. `handler` 参数提取。
2. `AutoQuery` / option 映射。
3. 推荐前 profile 准备。
4. 批量推荐和结果后处理。

像下面这些功能，不是只加 parser keyword 就算完成：

1. `顶配`
2. `次顶配`
3. `当前`
4. `区域道具15级`
5. `歌曲比较`
6. `排除卡牌`
7. `仅团/仅属性`

## 7.2 query 字段要么真实消费，要么不要保留成“假支持”

当前下游只稳定消费 ID 类字段，未稳定消费 query fallback 字段。

因此本轮实现应二选一：

1. 下游完整支持 `WorldBloomCharacterQuery`、`ChallengeLiveCharacterQuery`、`FixedCharacterQueries`。
2. 或者在 handler 层直接完成昵称解析，无法解析时立刻报错，不继续带着 query fallback 往下传。

不能继续保留“参数被解析了，但不会影响最终结果”的状态。

## 7.3 不要在一个 PR 里同时改 parser、profile、compare 和渲染

建议每个阶段只覆盖一类问题：

1. 先补协议字段和测试骨架。
2. 再补活动 / WL 语义。
3. 再补挑战模式。
4. 再补 profile 修饰项。
5. 最后补歌曲比较。

## 7.4 帮助文档和实现必须同步校验

本轮所有阶段都要以帮助文档作为验收输入，而不是只看 refer 或只看当前测试。

原因是：

1. 用户侧实际用的是帮助文档。
2. refer 是行为参考，不等同于 Cloud 的最终职责切分。
3. 帮助文档里已经明确了若干限制，例如加成组卡不能用纯数字活动 ID。

## 8. 建议的协议与职责收口

## 8.1 handler 层职责

handler 层只负责：

1. 解析命令参数。
2. 构造标准化 query / additional option。
3. 在必要时做语义级校验并尽早报错。

handler 层不应负责：

1. 真的去构造顶配 profile。
2. 真的去读取当前主队。
3. 真的去跑歌曲比较批量推荐。

这些能力应该交给 `render/deck` 的推荐准备层。

## 8.2 render/deck 层职责

`render/deck` 需要承接：

1. query 字段落到 option。
2. profile 过滤与 profile 覆盖。
3. 当前队伍读取。
4. 顶配 / 次顶配 profile 构造。
5. 区域道具等级覆盖。
6. 歌曲比较批量推荐与排序。

## 8.3 建议新增或明确的 query 能力

当前建议在 `deckAutoQueryParams` 与 `render/deck.AutoQuery` 中新增或明确以下字段：

1. `Boost`
2. `AreaItemLevel`
3. `UnitFilter`
4. `AttrFilter`
5. `ExcludedCards`
6. `UseCurrentDeck`
7. `MaxProfile`
8. `SubMaxProfile`
9. `MusicCompare`
10. `MusicCompareQueries`
11. `SpecificSkillOrder`
12. 如仍保留 query fallback，则要补齐下游消费链。

字段命名可以在实现阶段再做收敛，但能力面应覆盖这些语义。

## 9. 分步实现计划

以下步骤按“单步目标清晰、风险可控、便于回滚”的原则拆分。每一步都应单独提交和单独回归，不建议跨步合并。

## 第 0 步：建立基线和冻结验收样例

### 目标

在正式改逻辑前，先把帮助文档中的典型输入和当前 Cloud 的已有测试整理成稳定基线，避免后续每次改完都不知道是“新功能未做”还是“旧功能回归”。

### 实现计划

1. 整理帮助文档里的所有组卡示例，形成测试样例清单。
2. 把当前 `deck_test.go` 中已有覆盖点分类为：
   - 已实现且必须保持。
   - 当前未实现，但计划支持。
   - 当前行为与帮助文档冲突，后续需要改。
3. 明确每个模式的默认行为和报错文案范围。
4. 先不修改实现逻辑，只补文档和测试 TODO 标记。

### 测试计划

1. 审阅现有 `deck_test.go`，列出覆盖矩阵。
2. 新增一个文档化测试清单，不要求本步新增功能测试。
3. 运行：
   - `go test ./internal/pjsk/handler/sekai`
   - `go test ./internal/pjsk/render/deck`

### 完成标准

1. 后续每一步都有明确的新增测试目标。
2. 已知“当前不支持”的能力不再靠记忆维护。

## 第 1 步：补齐 query 协议字段，不扩展业务行为

### 目标

先把后续要用到的 query / option 承接面补齐，避免后面在 parser 里加了能力，却发现下游没有字段可传。

### 实现计划

1. 在 `handler/sekai/deck_types.go` 中补齐缺失字段。
2. 在 `render/deck/query.go` 中补齐对应字段。
3. 在 `render/deck/controller_options.go` 中把新字段安全映射到 option。
4. 只做字段和基础映射，不在本步实现完整业务语义。
5. 明确哪些字段是“下游必须消费”的，哪些字段只是中间态。

### 测试计划

1. 新增 query 序列化 / 反序列化测试。
2. 新增 `controller_options` 字段透传测试。
3. 确认本步不会改变现有命令语义。
4. 运行：
   - `go test ./internal/pjsk/handler/sekai -run Deck`
   - `go test ./internal/pjsk/render/deck -run Controller`

### 建议文件范围

1. `internal/pjsk/handler/sekai/deck_types.go`
2. `internal/pjsk/render/deck/query.go`
3. `internal/pjsk/render/deck/controller_options.go`
4. `internal/pjsk/render/deck/controller_test.go`

### 完成标准

1. `boost / area_item / filter / current / compare / specific_skill_order` 等能力有明确字段承接。
2. 不引入实际行为变化。

## 第 2 步：收口活动选择与 WL 语义

### 目标

先把活动组卡、加成组卡、烤森组卡共享的“活动选择”语义对齐，这是当前最容易引起错误解析的部分。

### 实现计划

1. 对齐活动 ID 选择规则：
   - 活动组卡允许 `123` `event123` `活动123`
   - 加成组卡只允许 `event123` `活动123`
2. 对齐 WL 相关规则：
   - `140 wl1`
   - `140 miku`
   - `wl1 miku`
   - `wl2 miku`
3. 对齐 WL 默认章节选择逻辑。
4. 对齐非 WL 活动上显式 `wl1/wl2` 的报错。
5. 补齐 `终章` 特殊映射。
6. 处理 `25` 团名与活动 ID 的歧义。
7. 统一活动组卡、加成组卡、烤森组卡三条路径的 event selector 行为。

### 测试计划

1. 新增活动选择解析测试：
   - 当前活动 / 下一个活动 fallback
   - `event123`
   - `123 ex` 不误判
   - `140 wl1`
   - `140 miku`
   - `wl2 miku`
   - 非 WL 活动上的 `wl1`
   - `25h 可爱`
   - `event25`
2. 新增加成组卡限制测试：
   - `event123 120`
   - `123 120` 应报错或按帮助文档拒绝
3. 运行：
   - `go test ./internal/pjsk/handler/sekai -run 'TestEventDeck|TestBonusDeck|TestMysekaiDeck'`

### 建议文件范围

1. `internal/pjsk/handler/sekai/deck_builder.go`
2. `internal/pjsk/handler/sekai/deck_extract_targets.go`
3. `internal/pjsk/handler/sekai/deck_helpers.go`
4. `internal/pjsk/handler/sekai/deck_test.go`

### 完成标准

1. 活动选择语义与帮助文档一致。
2. WL 解析不再只停留在“能识别 token”，而是具备完整默认和报错行为。

## 第 3 步：补齐挑战组卡语义

### 目标

将挑战组卡从“单角色简单解析”扩展为完整模式，包括 `challenge_auto`、全角色分支和挑战特有的 `当前` 逻辑。

### 实现计划

1. 明确挑战组卡的 live type 归一逻辑：
   - 默认 `challenge`
   - `auto` -> `challenge_auto`
2. 增加“未指定角色时为所有角色各组一队”的 query / option 表达方式。
3. 增加挑战模式使用 `当前` 时的校验：
   - 必须指定角色
   - 读取该角色当前挑战卡组
4. 增加挑战组卡歌曲比较前置校验：
   - 必须指定角色
5. 如后端已有挑战默认歌曲推荐能力，则明确何时启用；如暂不实现，则在本步写成非目标说明。

### 测试计划

1. 新增挑战模式解析测试：
   - `/挑战组卡`
   - `/挑战组卡 miku`
   - `/挑战组卡 miku auto`
   - `/挑战组卡 当前`
   - `/挑战组卡 miku 当前`
   - `/挑战组卡 miku 歌曲比较 10th 群青apd`
2. 新增 render 层 option 测试，验证 `challenge` / `challenge_auto` 的映射。
3. 运行：
   - `go test ./internal/pjsk/handler/sekai -run Challenge`
   - `go test ./internal/pjsk/render/deck -run 'Challenge|Controller'`

### 建议文件范围

1. `internal/pjsk/handler/sekai/deck_builder.go`
2. `internal/pjsk/handler/sekai/deck_extractor.go`
3. `internal/pjsk/render/deck/controller_options.go`
4. `internal/pjsk/render/deck/controller_engine.go`
5. `internal/pjsk/handler/sekai/deck_test.go`

### 完成标准

1. 挑战模式不再被当成普通 `solo/auto` 的轻微变种。
2. 挑战独有的默认行为和约束稳定下来。

## 第 4 步：补齐卡组修饰项和 profile 预处理

### 目标

集中处理那些“不是 parser 小修”的功能：`顶配`、`次顶配`、`当前`、`仅团/仅属性`、`排除卡牌`、`区域道具等级`、`火数`。

### 实现计划

1. 在 parser 中补齐以下参数提取：
   - `顶配`
   - `次顶配`
   - `当前`
   - `仅团`
   - `仅属性`
   - `排除卡牌`
   - `区域道具等级`
   - `火`
2. 在 `render/deck` 推荐准备层实现这些参数的真实语义：
   - 顶配 / 次顶配 profile 构造
   - 当前主队读取
   - 卡牌过滤
   - 排除卡牌
   - 区域道具覆盖
   - 体力对 event pt / score 的影响
3. 明确 `当前` 在活动组卡和挑战组卡上的差异：
   - 活动组卡可以动态读取主队
   - 挑战组卡依赖抓包更新
4. 明确 `次顶配` 的区域道具 15 级语义。

### 测试计划

1. 新增 parser 测试：
   - `顶配`
   - `次顶配`
   - `当前`
   - `仅mmj`
   - `仅紫`
   - `-123 -456`
   - `区域道具15级`
   - `5火`
2. 新增 render / controller / profile 预处理测试：
   - 顶配 profile 是否绕过用户卡组限制
   - 次顶配是否带区域道具 15 级
   - 当前主队是否固定为 5 张卡
   - 过滤和排除是否真实影响 userCards
3. 若本地存在 deck-service 联调环境，再补最小集成测试。
4. 运行：
   - `go test ./internal/pjsk/handler/sekai -run Deck`
   - `go test ./internal/pjsk/render/deck`

### 建议文件范围

1. `internal/pjsk/handler/sekai/deck_extractor.go`
2. `internal/pjsk/handler/sekai/deck_builder.go`
3. `internal/pjsk/handler/sekai/deck_types.go`
4. `internal/pjsk/render/deck/controller_options.go`
5. `internal/pjsk/render/deck/controller_engine.go`
6. `internal/pjsk/render/deck/controller_request.go`
7. 可能新增专门的 profile 预处理辅助文件

### 完成标准

1. 这些参数不再只是“能识别关键词”，而是会真实影响推荐输入。
2. `火` 不再出现“被剥离但无效”的假支持。

## 第 5 步：补齐固定卡组高级语义与手动技能顺序

### 目标

在前面基础已经稳定后，再补“固定卡组专用语义”，避免它和前几步同时改动导致问题难以定位。

### 实现计划

1. 支持 `技能顺序12345`。
2. 增加“仅在卡组完全固定时允许手动技能顺序”的校验。
3. 明确“完全固定”的来源：
   - `#1 2 3 4 5`
   - `当前`
4. 如需要，在 query 中显式增加 `SpecificSkillOrder`。
5. 明确固定角色场景下的 leader 位置语义是否允许手动技能顺序；若不允许，应直接报错。

### 测试计划

1. 新增解析测试：
   - `技能顺序平均`
   - `技能顺序12345`
   - 非固定卡组下 `技能顺序12345` 报错
2. 新增 render option 测试：
   - `specific_skill_order`
   - `skill_order_choose_strategy = specific`
3. 运行：
   - `go test ./internal/pjsk/handler/sekai -run Skill`
   - `go test ./internal/pjsk/render/deck -run 'Skill|Controller'`

### 建议文件范围

1. `internal/pjsk/handler/sekai/deck_extractor.go`
2. `internal/pjsk/handler/sekai/deck_helpers.go`
3. `internal/pjsk/handler/sekai/deck_types.go`
4. `internal/pjsk/render/deck/query.go`
5. `internal/pjsk/render/deck/controller_options.go`

### 完成标准

1. `技能顺序12345` 行为与 refer 保持一致。
2. 不再允许未固定卡组时误用手动技能顺序。

## 第 6 步：实现歌曲比较

### 目标

最后单独处理 `歌曲比较`，因为它不仅是参数解析，还涉及批量推荐、候选歌曲确定和结果排序。

### 实现计划

1. 在 parser 中补齐 `歌曲比较` 标记和“多首歌曲列表”提取。
2. 支持两种模式：
   - 显式限定歌曲集合
   - 固定卡组后比较所有歌曲
3. 加入前置校验：
   - 不限定歌曲时必须固定卡组
   - 挑战组卡做歌曲比较时必须指定角色
4. 在 `render/deck` 中加入批量推荐调度。
5. 对批量结果按 refer 语义排序并截断。
6. 明确展示层需要的歌曲标题 / 封面 / 难度元数据准备方式。

### 测试计划

1. 新增 parser 测试：
   - `歌曲比较 当前`
   - `歌曲比较 #1 2 3 4 5`
   - `歌曲比较 龙hard 虾expert sage`
2. 新增批量推荐测试：
   - 输入多首歌曲，检查是否拆成多组 option
   - 返回结果后检查排序逻辑
3. 新增错误路径测试：
   - 未固定卡组且未限定歌曲
   - 挑战组卡未指定角色就做歌曲比较
4. 运行：
   - `go test ./internal/pjsk/handler/sekai -run Compare`
   - `go test ./internal/pjsk/render/deck`

### 建议文件范围

1. `internal/pjsk/handler/sekai/deck_builder.go`
2. `internal/pjsk/handler/sekai/deck_extractor.go`
3. `internal/pjsk/handler/sekai/deck_types.go`
4. `internal/pjsk/render/deck/controller_engine.go`
5. `internal/pjsk/render/deck/controller_request.go`
6. `internal/pjsk/render/deck/controller_test.go`

### 完成标准

1. 歌曲比较不再只是 parser 能认出关键词，而是完整跑通推荐、排序和展示。
2. 大量计算路径只在真正启用 `歌曲比较` 时触发。

## 第 7 步：帮助文档同步和回归收尾

### 目标

在核心功能完成后，统一补齐帮助文本、回归测试和未决边界说明，避免后续再次出现“帮助文档和真实实现不一致”。

### 实现计划

1. 对照帮助文档逐条回归。
2. 更新 Cloud 自身文档和必要的注释。
3. 如有最终不实现的 refer 语义，必须在文档里写清楚。
4. 清理阶段性兼容逻辑和临时 TODO。

### 测试计划

1. 人工按帮助文档示例逐条验收。
2. 补一份 e2e 回归清单。
3. 运行完整测试：
   - `go test ./internal/pjsk/handler/sekai`
   - `go test ./internal/pjsk/render/deck`
   - 如环境允许，运行 deck-service 联调测试

### 完成标准

1. 帮助文档、实现、测试三者语义一致。
2. 不再依赖“口头说明”记住哪些参数可用、哪些不可用。

## 10. 建议的 PR 切分

建议按以下粒度提交，而不是大包大揽：

1. PR-1：测试基线和 query 字段补齐。
2. PR-2：活动选择、WL、加成组卡限制。
3. PR-3：挑战组卡完整语义。
4. PR-4：顶配 / 次顶配 / 当前 / 过滤 / 区域道具 / 火数。
5. PR-5：手动技能顺序。
6. PR-6：歌曲比较。
7. PR-7：帮助文档与回归收尾。

每个 PR 都应满足：

1. 有明确的用户可见目标。
2. 有新增测试。
3. 文件范围尽量聚焦。
4. 不混入无关重构。

## 11. 风险与回滚建议

### 11.1 高风险点

以下点需要特别谨慎：

1. `当前` 涉及主队 / 挑战卡组读取，容易和 snapshot 数据源耦合。
2. `顶配` / `次顶配` 涉及 profile 构造，容易绕过现有用户卡过滤逻辑。
3. `歌曲比较` 涉及批量推荐，最容易引入性能问题。
4. WL 默认章节逻辑依赖时间判断，最容易出现在边界时间点行为不稳。

### 11.2 回滚策略

建议每个阶段都保持可独立回滚：

1. parser 新字段补齐应先于复杂行为。
2. 歌曲比较必须最后做，避免前几步不稳定时一起搅在同一提交里。
3. 如某一步引入了 profile 预处理副作用，应优先回滚该步，而不是去临时补更多条件分支。

## 12. 当前建议

如果按风险和收益排序，建议实现顺序为：

1. 第 1 步：字段补齐。
2. 第 2 步：活动 / WL / 加成限制。
3. 第 3 步：挑战组卡。
4. 第 4 步：profile 修饰项。
5. 第 5 步：手动技能顺序。
6. 第 6 步：歌曲比较。
7. 第 7 步：文档和回归收尾。

这样可以先把“最容易产生错误解析”的路径稳定下来，再处理 profile 级和批量推荐级能力。
