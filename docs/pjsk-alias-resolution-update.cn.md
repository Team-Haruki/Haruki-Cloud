# Haruki-Cloud PJSK 别名与名称解析链路整理

> 最后更新：2026-03-28
>
> 本文只整理这轮已经落地的“歌曲别名 / 角色别名 / handler 参数透传”相关改动，不覆盖其他功能实现。

## 1. 文档目的

这轮调整的目标有三个：

1. 把歌曲查询统一接到正式的歌曲 alias 解析链。
2. 把角色查询统一接到正式的角色 alias + masterdata 名称解析链。
3. 清掉 `handler` 层残留的旧 `currentNicknames` 前置硬转逻辑，避免入口层和 bridge/render 层各自维护一套角色解析规则。

核心原则是：

1. `handler` 负责拆分语法，不负责把角色名或曲名提前硬转成最终 ID。
2. 歌曲解析由 `music controller / requestbuilder / bridge` 统一完成。
3. 角色解析由 `bridge.resolveGameCharacterIDByQuery(...)` 统一完成。

## 2. 关键结论

### 2.1 歌曲别名与角色别名是两套链路

当前项目里需要明确区分两类“昵称 / 别名”：

1. 歌曲：
   - 正式曲名、曲目 ID 来自 masterdata。
   - 已审核歌曲别名来自 `pjsk alias` 表。
2. 角色：
   - 正式角色名、角色 ID 来自 masterdata。
   - 已审核角色别名来自 `pjsk alias` 表。

这两类解析已经不再混用。

### 2.2 `handler` 层不再维护旧的角色昵称主链

此前 `handler/sekai` 里有一套基于 `currentNicknames` 的旧角色提取逻辑。

这一轮之后：

1. `education-area`
2. `deck`
3. `sk` 的 WL 角色参数

都改成了“先保留 query，再统一交给 bridge 解析”。

### 2.3 公共角色解析已经支持正式 alias service

`bridge.resolveGameCharacterIDByQuery(...)` 现在的顺序是：

1. 优先尝试正式的 `character alias service`
2. 再回退到 masterdata 名称匹配

因此，走这条公共链的入口现在都能识别：

1. 角色 ID
2. 正式中文名
3. 正式英文名
4. 已审核角色别名

## 3. 歌曲别名链路

### 3.1 入口与核心文件

主要文件：

1. `internal/pjsk/alias/service.go`
2. `internal/pjsk/render/music/controller.go`
3. `internal/pjsk/render/music/search.go`
4. `internal/pjsk/render/music/lookup.go`
5. `internal/pjsk/render/music/meta_request.go`
6. `internal/pjsk/render/music/board_request.go`
7. `internal/pjsk/requestbuilder/score_control.go`
8. `internal/pjsk/handler/bridge.go`

### 3.2 当前行为

`alias.Service` 提供：

1. `TryResolveMusicID(ctx, token)`

`music.Controller` 提供：

1. `SetAliasResolver(...)`
2. `resolveMusicTitleQuery(...)`
3. `resolveMusicListKeywordFilter(...)`

因此，音乐相关链路现在的行为是：

1. 如果 query 能命中已审核歌曲别名，则优先直接解析成 `music id`
2. 如果没命中歌曲别名，再回退到 masterdata 曲名搜索

### 3.3 已接入的主要入口

当前已经接到统一歌曲解析链的入口包括：

1. `music-detail`
2. `music-list`
3. `music-cover`
4. `music-bpm`
5. `music-chart`
6. `score-control`
7. `score-music-meta`
8. `score-music-board`
9. `deck` 里的 `music_query`

补充说明：

1. `music-list` 在 bridge 里会把 `r.Query` 回填到 `keyword`
2. 如果 `keyword` 命中歌曲别名，会按精确 `music id` 过滤，而不是做普通字符串模糊匹配

### 3.4 resolver 注入规则

这一轮也统一了 `music controller` 的 resolver 注入规则：

1. 只有在 `app.Aliases != nil` 时才会执行 `SetAliasResolver(app.Aliases)`
2. 不会再把 controller 上已有的 resolver 清空

这个处理出现在：

1. `internal/pjsk/handler/bridge.go`
2. `internal/pjsk/requestbuilder/score_control.go`

## 4. 角色别名链路

### 4.1 入口与核心文件

主要文件：

1. `internal/pjsk/alias/service.go`
2. `internal/pjsk/handler/bridge.go`
3. `internal/pjsk/handler/sekai/education.go`
4. `internal/pjsk/handler/sekai/deck_params.go`
5. `internal/pjsk/handler/sekai/sk.go`
6. `internal/pjsk/render/sk/controller.go`

### 4.2 alias service 能力

这一轮新增：

1. `alias.Service.TryResolveCharacterID(ctx, token)`

它的顺序与歌曲类似：

1. 先尝试角色 ID
2. 再尝试 masterdata 正式角色名
3. 再尝试已审核角色别名

### 4.3 bridge 公共角色解析

`internal/pjsk/handler/bridge.go` 中的：

1. `resolveGameCharacterIDByQuery(...)`

现在已经成为角色查询的公共主链。

当前顺序是：

1. 检查 query 是否为空
2. 如果 `app.Aliases` 可用，优先调用 `TryResolveCharacterID(...)`
3. 如果 alias service 没命中，再走 masterdata `Gamecharacter` 名称匹配

当前复用这条公共角色链的主要入口包括：

1. `education-area`
2. `deck` 的 `world_bloom_character_query`
3. `deck` 的 `challenge_live_character_query`
4. `deck` 的 `fixed_character_queries`
5. `sk` 的 `wl_character_query`

## 5. handler 层收口情况

### 5.1 `education-area`

相关文件：

1. `internal/pjsk/handler/sekai/education.go`
2. `internal/pjsk/render/education/query.go`
3. `internal/pjsk/handler/bridge.go`

当前行为：

1. `handler` 只保留 `CharacterQuery`
2. 不再在入口层用旧昵称表把角色名转成 `Cid`
3. 真正的角色解析在 bridge 层完成

### 5.2 `deck`

相关文件：

1. `internal/pjsk/handler/sekai/deck_params.go`
2. `internal/pjsk/render/deck/query.go`
3. `internal/pjsk/handler/bridge.go`

当前行为：

1. `handler` 负责拆分出：
   - `WorldBloomCharacterQuery`
   - `ChallengeLiveCharacterQuery`
   - `FixedCharacterQueries`
2. 不再在入口层依赖 `currentNicknames`
3. bridge 统一把这些 query 解析成最终角色 ID

`deck` 当前仍保留一个语义上的特殊处理：

1. `challenge` 或非 WL turn 场景下，如果角色查询词在角色链里找不到，且当前没有歌曲查询，则会回落成 `MusicQuery`
2. 这是为了避免把单个曲名误杀成“角色查询失败”

### 5.3 `sk`

相关文件：

1. `internal/pjsk/handler/sekai/sk.go`
2. `internal/pjsk/render/sk/controller.go`
3. `internal/pjsk/handler/bridge.go`

这轮前的状态是：

1. WL 角色参数只认 `wl5`、`cid5`、`chara5` 这种数字写法

这轮后的状态是：

1. 支持直接写角色名
2. 支持 `wl初音未来` / `chara初音未来` 这种前缀写法
3. `handler` 会保留 `wl_character_query`
4. bridge 会把它解析成 `wl_character_id`

## 6. 清理掉的旧实现

### 6.1 已移除的旧依赖

`internal/pjsk/handler/sekai` 下原来的这些旧入口辅助已经不再作为业务主链存在：

1. `currentNicknames`
2. `SetNicknames(...)`
3. `resolveNicknameArg(...)`

同时：

1. `EnsureCommandHandlersRegistered(nicknames map[string]int)` 中的昵称注入参数也已经不再参与 handler 解析逻辑

### 6.2 为什么要移除

旧实现的问题是：

1. 入口层和 bridge 层各自解析角色
2. 角色昵称只依赖静态表，不依赖正式 alias service
3. 不同 handler 对“角色名 / 昵称 / 别名”的支持能力不一致

移除后，职责边界更清晰：

1. `handler` 只拆语法
2. `bridge` 统一做角色解析
3. `alias service` 统一管理正式别名能力

## 7. 当前仍然有意保留的例外

### 7.1 `card` 查询链

`card-detail` / `card-list` / `card-image` 这条链当前没有改成走 bridge 公共角色 alias 解析。

原因是：

1. `render/card` 自己有一套独立的卡牌查询语义
2. 它不只是“角色名 -> 角色 ID”
3. 还要处理卡牌序号、卡牌筛选、卡牌 ID、角色+卡序等组合语义

因此，这部分本轮没有强行并到 bridge 公共链里。

### 7.2 `misc-birthday`

`misc-birthday` 当前也是保留“handler 透传 raw query，后面再解析”的模式。

只是它的实际解析位置不在 bridge，而是在：

1. `internal/pjsk/requestbuilder/misc_birthday.go`

## 8. 主要测试覆盖

这轮补过或更新过的重点测试包括：

1. `internal/pjsk/render/music/list_test.go`
2. `internal/pjsk/render/music/lookup_test.go`
3. `internal/pjsk/render/music/meta_request_test.go`
4. `internal/pjsk/requestbuilder/score_control_test.go`
5. `internal/pjsk/handler/sekai/education_test.go`
6. `internal/pjsk/handler/sekai/deck_test.go`
7. `internal/pjsk/handler/sekai/sk_tracker_params_test.go`
8. `internal/pjsk/handler/bridge_test.go`

本轮确认通过的关键测试包括：

1. `go test ./internal/pjsk/handler/sekai`
2. `go test ./internal/pjsk/alias`
3. `go test ./internal/pjsk/handler -run 'TestResolveDeckCharacterSelectionsUsesMasterdataQueries|TestResolveDeckCharacterSelectionsFallsBackChallengeQueryToMusic|TestResolveDeckCharacterSelectionsFallsBackWorldBloomQueryToMusic|TestResolveEducationAreaCharacterIDUsesMasterdataName|TestResolveTrackerCharacterSelectionUsesMasterdataQuery|TestResolveGameCharacterIDByQueryUsesApprovedAlias'`

## 9. 当前状态总结

截至本文档更新时，PJSK 相关的别名与名称解析状态可以总结为：

1. 歌曲 alias 已经进入正式主链
2. 角色 alias 已经进入 bridge 公共角色解析主链
3. `education` / `deck` / `sk` 已经不再依赖旧 `currentNicknames`
4. handler 层与 bridge/render 层的职责边界比之前清晰

如果后续还要继续收口，优先级应当是：

1. 继续检查是否还有新的业务入口绕过了公共角色解析链
2. 只在确实有必要时，再考虑是否要把 `card` 的独立解析模型继续合并
