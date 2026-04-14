# Haruki-Cloud 项目完成度跟踪

> 最后更新：2026-04-14（实战联调稳定化 + 安全加固）
>
> 本文基于 2026-04-08 ~ 2026-04-09 对当前仓库代码与 `docs/` 文档的交叉审计整理而成，用于持续跟踪“哪些能力已经稳定、哪些仍处于过渡阶段、哪些尚未暴露”。
>
> 2026-04-09 当日晚些时候已执行一轮主链稳定化：
>
> 1. `api/legacy/pjsk/` 已从仓库中移除，不再作为兼容主链保留。
> 2. `cmd/server/main.go` 已不再注册 legacy PJSK 路由。
> 3. `go test ./...` 当前已恢复全绿。
> 4. `integration` 测试改为默认关闭，需显式设置 `HARUKI_RUN_INTEGRATION=1` 才执行。
> 5. `integration/api_test.go` 已支持通过环境变量完整覆盖主配置，且不再依赖“先跑 `TestAuth`”的隐含顺序。
> 6. `/internal/*` 默认鉴权已收紧：未配置内部鉴权时默认拒绝，仅显式 `allow_insecure_internal_api=true` 才放宽。
> 7. `internal/pjsk/render/userdata` 已新增 `Snapshot` / `SnapshotProvider` / `SnapshotFactory` 抽象，并落地 `ToolboxSnapshotProvider` + `DefaultSnapshotFactory`；请求级 live snapshot 解析已从 `handler/resolver.go` 内联逻辑收口到 provider。
> 8. 最新架构已明确为“Cloud 只读、Toolbox 为事实来源”：生产运行时不再保留 `pjsk_user_snapshots`、`snapshot/upload`、验证后回填、读链写回等 Cloud 侧快照镜像机制。
> 9. 当前生产快照链路已收敛为 `Toolbox -> local static(debug fallback)`；`local_file` 仅保留给测试、联调和开发环境。
> 10. `Snapshot.RawValue(key)` 已落地，`profile`、`deck/mysekai` 公共资料卡与 `music progress/rewards` 已改为通过 render/controller 侧入口消费 snapshot/provider 数据；handler 层不再直接读取 `userPlayerFrames`、`userMusicAchievements` 这类具体 key，也不再手动拼 `music progress` 的 snapshot 结果。
> 11. `education/bonds` 与 `education/leader` 已改为直接消费 suite snapshot 中的 `userBonds`、`userCharacters`、`userCharacterMissionV2*` 字段，不再依赖 handler 层 `GetPrivateDataValue(...)` 单 key 查询。
> 12. `education` 的 bonds / leader snapshot builder 已下沉到 `render/education` controller，相关 bonds / level / character-style / leader-mission masterdata 也已收口进 `EducationProvider` / `DataSource`，handler 进一步瘦身。
> 13. `MySekaiPayloadProvider` 已收敛为直接读取 Toolbox 的专用只读 provider；`bridge_mysekai.go` 不再直接调用 `GetMySekaiData()`，且 “snapshot 优先、payload provider 兜底” 的请求期绑定逻辑已收口到统一 helper。`SuiteVisible=false` 但 `MySekaiVisible=true` 的账号也能通过统一 provider 读取 raw mysekai 数据。
> 14. `deck recommend auto` 运行时已正式收口为 HTTP 外部服务：`use_local_engine`、本地 cgo engine 与 Cloud 内启发式 fallback 已退出主链，`deck_cgo` 历史目录也已从仓库移除；未配置 `service_base_url` 时将直接报配置错误。
> 15. 2026-04-09 最新一轮稳定化已继续推进请求级上下文与资源回收：`music` / `score` / `sk` / `misc birthday` 主链不再把别名解析、forecast 抓取、生日 DB 查询挂到 `context.Background()`；服务退出时也会显式关闭 `Redis` 与 `censor DB` 连接。
> 16. 同日晚些时候，`card` / `event` / `gacha` / `music` / `profile` 的 provider-backed controller 已补上“按请求克隆 source/provider”链路：这些模块经由 `DatabaseProvider` 读取 masterdata 时，已能把请求 `ctx` 继续传到 `db_cards` / `db_events` / `db_musics` / `db_gachas` / `db_honors` / `db_player_frames` / `db_characters` / `db_skills` 查询层。
> 17. 继续推进后，`education` / `stamp` / `vlive` 也已接入同一套请求级 source/provider 克隆链路；当前主链 render provider 中已基本消除“直接用 `context.Background()` 发起 DB 查询”的问题，剩余个别 `Background()` 主要只出现在本地调试/静态 snapshot helper 与脚本入口的 nil-ctx 兜底逻辑。
> 18. `internal/pjsk/render/userdata` 的 `SnapshotFactory` 现已真正消费传入 `ctx`：live/local snapshot 构建时的 leader 卡图路径解析会跟随构建上下文；`NewFromBytesWithContext(...)`、`NewLocalFileServiceWithContext(...)` 也已补齐，MySekai JSON merge 逻辑收口到统一 helper。
> 19. `cmd/migrate` 已移除源码中的硬编码 Sekai DSN：现在优先读取 `HARUKI_SEKAI_DB_URL` / `HARUKI_SEKAI_DSN`，否则回退读取 `HARUKI_CONFIG_PATH` 或默认 `haruki-db-configs.yaml` 中的 `sekai.db_url`；迁移上下文也已挂到信号取消。
> 20. 2026-04-10 又继续执行了一轮大文件治理：`render/deck/controller.go`、`render/education/snapshot_build.go`、`render/mysekai/controller.go`、`render/deck/remote_engine.go`、`handler/sekai/sk.go`、`alias/service.go`、`handler/sekai/profile.go`、`handler/sekai/mysekai.go`、`render/music/controller.go`、`render/sk/forecast_provider.go`、`render/provider/db_education.go`、`render/provider/db_cards.go` 均已进一步按职责拆分，当前大文件热点已明显收缩到少数 provider / builder / extractor 文件。
> 21. 2026-04-10 本轮后续又继续清理了一批热点文件：`handler/sekai/deck_extractor.go`、`render/provider/db_musics.go`、`render/gacha/builder.go`、`handler/sekai/score_board_params.go`、`render/music/board_helpers.go`、`render/music/lookup.go`、`render/music/builder.go`、`render/event/builder.go`、`render/honor/builder.go`、`render/provider/db_honors.go` 均已进一步按职责拆分；`music/event/honor/provider` 相关 targeted tests 与 `go test ./...` 现已保持通过。
> 22. 2026-04-10 本轮继续收尾后，`render/userdata/local.go`、`render/sk/controller_trace.go`、`render/mysekai/helpers.go`、`render/music/board_request.go`、`render/mysekai/map_builder.go`、`render/profile/controller.go`、`render/provider/contextual.go` 也已进一步按职责拆分；同时补平了 `render/music/lookup.go` 上一轮拆分残留的重复定义编译债务，`music/sk/userdata/mysekai/profile/provider/handler` 定向回归已恢复通过。
> 23. 截至 2026-04-10，`go test ./...` 仍保持全绿；重构工作已从“主链架构收口”进入“剩余局部热点清理”阶段。

## 1. 范围与方法

本次审计范围包括：

- `cmd/`
- `api/`
- `internal/`
- `utils/`
- `drawing/`
- `docs/`

本次明确排除：

- `database/` 中的 ent 生成实现
- `ent/` 中的 schema / 生成代码
- 纯数据目录（如 `data/master/*`）

审计方法：

1. 阅读服务入口、路由注册、运行时装配与核心业务链路。
2. 对照 `docs/` 中的设计文档、状态文档与合并说明。
3. 读取 `internal/pjsk/handler/sekai/` 与 `internal/pjsk/render/` 的实现和测试。
4. 运行 `go test ./...`，记录当前失败包与原因。

## 2. 当前结论

当前仓库已经明显超过“搭骨架”阶段，属于：

- **主干已成型**
- **可内部联调**
- **重构已进入收尾阶段，剩余热点已缩小**

如果只看 `Haruki-Cloud` 作为 **PJSK Bot 新协议后端** 的目标完成度，本次审计给出的判断是：

- **业务主链完成度：约 93%**
- **发布级稳定度：约 90%**

更准确地说，它已经是一个 **可以继续在现有结构上推进，而不应推倒重来** 的项目。

再补一条当前事实：

- **结构性重构已进入后半段的最后一段**
- **当前主要剩余问题已经不再是“架构散乱”，而是少数 provider / helper / builder 热点仍偏厚**

## 3. 分档规则

| 分档 | 含义 | 当前跟踪用语 |
|------|------|--------------|
| `A` | 主链完成，已经是当前正式实现 | 稳定 |
| `B` | 已实现、可联调，但仍依赖外部服务、快照桥接或多源拼装 | 过渡 / 条件可用 |
| `C` | 兼容层或迁移层，不应再视为最终主模型 | 兼容 |
| `D` | 未实现、未暴露或显式 disabled | 未完成 |

## 4. 路由总量结论

按 **当前 handler registry 实测**，Bot API 现在暴露的是：

- **82 条活跃 Bot path**
- **0 个 disabled handler**（所有注册 handler 均无 `Disabled: true` 标记）

> 注意：旧文档中多处仍写 **76 条**。按照当前注册表运行结果，后续应以本文档中的 **82 条活跃 path 清单** 为准。
> 2026-04-10 更新：原文档曾列 9 个 disabled handler（CardStory、GachaRecord、EventStory 等），经代码核实这些 handler 已不存在于代码库中，disabled 列表已清空。

模块分布如下：

| 模块前缀 | 活跃 path 数 |
|----------|--------------|
| `alias` | 9 |
| `arrest` | 1 |
| `card` | 4 |
| `deck` | 6 |
| `education` | 5 |
| `event` | 3 |
| `gacha` | 1 |
| `misc` | 1 |
| `music` | 8 |
| `mysekai` | 9 |
| `profile` | 20 |
| `score` | 4 |
| `sk` | 9 |
| `stamp` | 1 |
| `vlive` | 1 |

## 5. 模块完成度矩阵

| 模块 | 活跃 path | 分档 | 当前判断 | 主要边界 |
|------|-----------|------|----------|----------|
| 协议 / 鉴权 / Manifest / Bot API | N/A | `A` | 主链已完成 | Noise NK 已落地、Auth AES-256-GCM 固定密钥已实现、请求去重+限流已上线 |
| Parser / Handler Registry | N/A | `A-` | 已完成 | 路径数与旧文档存在漂移 |
| Alias | 9 | `A-` | 稳定 | 审核类命令依赖管理员身份与 PJSK DB |
| Profile 账号体系 | 20 中的设置类 | `A-` | 稳定 | 依赖 users DB、PJSK DB、Toolbox 快速验证 |
| Profile 渲染 | `profile` | `B+` | 已实现 | 依赖 Sekai API / Toolbox / Drawing / ImageCache |
| Card | 4 | `A-` | 稳定 | 图片链依赖 Drawing / 资产完整性 |
| Music | 8 | `A-` | 稳定 | `progress/rewards/bpm` 仍有快照或环境依赖 |
| Score | 4 | `A-` | 稳定 | 复杂参数语义仍需保持回归测试 |
| SK / Tracker | 9 | `B+` | 已实现 | 强依赖 tracker 与绑定解析；需持续保持集成回归 |
| Event | 3 | `B` | 已实现 | `event-record` 更依赖真实历史数据 |
| Gacha | 1 | `B` | 基本可用 | 仍有 disabled 扩展能力 |
| Education | 5 | `B-` | 已接通 | 主要依赖 snapshot/provider + masterdata，部分路径仍在过渡 |
| Deck | 6 | `B-` | 已接通 | 依赖 snapshot / Drawing / recommend engine 过渡方案 |
| MySekai | 9 | `B-` | 已接通 | 已切到 Toolbox/provider 读链；仍保留本地 masterdata fallback |
| Stamp | 1 | `A-` | 稳定 | 功能范围已收口为贴纸列表 |
| VLive | 1 | `A` | 稳定 | 当前目标就是最小文本链路 |
| 公开 API（PJSK / CHUNITHM） | N/A | `A-` | 已完成 | PJSK 公开面刻意只保留 alias 查询 |
| Legacy `/internal/pjsk/*` | N/A | `D` | 已移除 | 2026-04-09 已从运行时与仓库中删除 |
| Drawing Python 服务 | N/A | `B` | 可用 | 仍受资产缺失与服务端细节 bug 影响 |

## 6. 活跃 Bot Path 清单

本节按 **当前活跃 path** 记录，供后续直接更新。

### 6.1 Alias（9）

模块判断：`A- / 稳定`

| Path | 分档 | 当前状态 | 备注 |
|------|------|----------|------|
| `alias/music` | `A-` | 稳定 | 歌曲别名查询 |
| `alias/music/add` | `A-` | 稳定 | 歌曲别名提交审核 |
| `alias/music/del` | `A-` | 稳定 | 歌曲别名删除 |
| `alias/character` | `A-` | 稳定 | 角色别名查询 |
| `alias/character/add` | `A-` | 稳定 | 角色别名提交审核 |
| `alias/character/del` | `A-` | 稳定 | 角色别名删除 |
| `alias/pending` | `A-` | 稳定 | 待审核列表 |
| `alias/approve` | `A-` | 稳定 | 审核通过 |
| `alias/reject` | `A-` | 稳定 | 审核拒绝 |

### 6.2 Arrest（1）

模块判断：`A- / 稳定`

| Path | 分档 | 当前状态 | 备注 |
|------|------|----------|------|
| `arrest` | `A-` | 稳定 | self / `@用户` / UID 三种目标模式都已接通 |

### 6.3 Card（4）

模块判断：`A- / 稳定`

| Path | 分档 | 当前状态 | 备注 |
|------|------|----------|------|
| `card/detail` | `A-` | 稳定 | 单卡详情 |
| `card/list` | `A-` | 稳定 | 条件筛选列表 |
| `card/box` | `A-` | 稳定 | 卡牌一览 / box 语义 |
| `card/image` | `A-` | 稳定 | 原图 / 多图消息；旧状态文档对此项判断已过期 |

### 6.4 Deck（6）

模块判断：`B- / 过渡`

| Path | 分档 | 当前状态 | 备注 |
|------|------|----------|------|
| `deck/event` | `B-` | 过渡 | 活动组卡；依赖 snapshot / HTTP recommend service / Drawing |
| `deck/challenge` | `B-` | 过渡 | 挑战组卡 |
| `deck/no-event` | `B-` | 过渡 | 长草 / 最强组卡 |
| `deck/bonus` | `B-` | 过渡 | 加成 / 控分组卡 |
| `deck/mysekai` | `B-` | 过渡 | MySekai 组卡；依赖更重 |
| `deck/score-up` | `A` | 稳定 | 纯文本计算，不依赖 Drawing；旧状态文档对此项判断已过期 |

### 6.5 Education（5）

模块判断：`B- / 过渡`

| Path | 分档 | 当前状态 | 备注 |
|------|------|----------|------|
| `education/challenge` | `B-` | 过渡 | 已走 snapshot；仍依赖 masterdata / Drawing |
| `education/power` | `B-` | 过渡 | 依赖 suite snapshot 与部分 MySekai 数据 |
| `education/area` | `B-` | 过渡 | 依赖 snapshot 与 area/masterdata |
| `education/bonds` | `B-` | 过渡 | 已走 snapshot/store；仍依赖 bonds masterdata |
| `education/leader` | `B-` | 过渡 | 已走 snapshot/store；仍依赖 mission masterdata |

### 6.6 Event（3）

模块判断：`B / 已实现`

| Path | 分档 | 当前状态 | 备注 |
|------|------|----------|------|
| `event` | `B` | 已实现 | 单活动详情 |
| `event/list` | `B` | 已实现 | 活动列表 / 过滤 |
| `event/record` | `B-` | 条件可用 | 需要真实用户活动历史数据 |

### 6.7 Gacha（1）

模块判断：`B / 基本可用`

| Path | 分档 | 当前状态 | 备注 |
|------|------|----------|------|
| `gacha` | `B` | 基本可用 | 列表主链已接通；扩展能力仍保留 disabled 实现 |

### 6.8 Misc（1）

模块判断：`B+ / 已实现`

| Path | 分档 | 当前状态 | 备注 |
|------|------|----------|------|
| `misc/birthday` | `B+` | 已实现 | 角色生日文本 -> 渲染链路已完成 |

### 6.9 Music（8）

模块判断：`A- / 稳定`

| Path | 分档 | 当前状态 | 备注 |
|------|------|----------|------|
| `music` | `A-` | 稳定 | 歌曲详情主链 |
| `music/list` | `A-` | 稳定 | 歌曲列表 / 关键字 / alias |
| `music/chart` | `A-` | 稳定 | 谱面预览 |
| `music/cover` | `A-` | 稳定 | 曲绘查询 |
| `music/note-count` | `A-` | 稳定 | 物量查询 |
| `music/bpm` | `B` | 条件可用 | 依赖本地谱面文件环境 |
| `music/progress` | `B` | 过渡 | 依赖快照 / 公开资料混合 |
| `music/rewards` | `B` | 过渡 | 依赖快照 / 公开资料混合 |

### 6.10 MySekai（9）

模块判断：`B- / 过渡`

| Path | 分档 | 当前状态 | 备注 |
|------|------|----------|------|
| `mysekai/resource` | `B-` | 过渡 | 依赖 snapshot / masterdata / Drawing |
| `mysekai/map` | `B-` | 过渡 | 依赖 snapshot / masterdata / Drawing |
| `mysekai/talk-list` | `B-` | 过渡 | 对资源完整性更敏感 |
| `mysekai/fixture-list` | `B-` | 过渡 | 依赖 snapshot / masterdata |
| `mysekai/fixture-detail` | `B-` | 过渡 | 依赖 snapshot / masterdata |
| `mysekai/door-upgrade` | `B-` | 过渡 | 依赖 Drawing 侧实现稳定性 |
| `mysekai/music-record` | `B-` | 过渡 | 依赖 snapshot / masterdata |
| `mysekai/blueprint` | `B-` | 过渡 | 当前是语义收口后的组合入口 |
| `mysekai/photo` | `B+` | 已实现 | 直接走图片下载，不依赖 Drawing 渲染 |

### 6.11 Profile（20）

模块判断：`混合：A- ~ B`

| Path | 分档 | 当前状态 | 备注 |
|------|------|----------|------|
| `profile` | `B+` | 已实现 | 资料渲染主链；依赖 Sekai API / Drawing / ImageCache |
| `profile/bind` | `A-` | 稳定 | 账号绑定 |
| `profile/bind/list` | `A-` | 稳定 | 绑定列表 |
| `profile/unbind` | `A-` | 稳定 | 解绑 |
| `profile/default` | `A-` | 稳定 | 设置默认绑定 |
| `profile/default/clear` | `A-` | 稳定 | 清除默认绑定 |
| `profile/verify` | `A-` | 稳定 | 快速验证 |
| `profile/verify/list` | `A-` | 稳定 | 验证列表 |
| `profile/visibility/hide` | `A-` | 稳定 | 隐藏 ID |
| `profile/visibility/show` | `A-` | 稳定 | 显示 ID |
| `profile/suite/hide` | `A-` | 稳定 | 隐藏 suite 抓包信息 |
| `profile/suite/show` | `A-` | 稳定 | 显示 suite 抓包信息 |
| `profile/mysekai/hide` | `A-` | 稳定 | 隐藏 MySekai 抓包信息 |
| `profile/mysekai/show` | `A-` | 稳定 | 显示 MySekai 抓包信息 |
| `profile/bg/upload` | `B` | 过渡 | 依赖验证状态、内容审核、BG 存储、ImageCache |
| `profile/bg/clear` | `B` | 过渡 | 依赖 BG 存储 |
| `profile/bg/adjust` | `B` | 过渡 | 依赖已存在背景图 |
| `profile/check-data` | `B` | 条件可用 | 依赖 Toolbox |
| `profile/check-data-mysekai` | `B` | 条件可用 | 依赖 Toolbox |
| `profile/reg-time` | `A-` | 稳定 | 注册时间查询 |

### 6.12 Score（4）

模块判断：`A- / 稳定`

| Path | 分档 | 当前状态 | 备注 |
|------|------|----------|------|
| `score` | `A-` | 稳定 | 分数控制 / 分数计算 |
| `score/custom-room` | `A-` | 稳定 | 自定义房间 |
| `score/music-meta` | `A-` | 稳定 | 曲目 meta |
| `score/music-board` | `A-` | 稳定 | 排行 / 对比；参数语义较复杂，需继续回归 |

### 6.13 SK（9）

模块判断：`B+ / 已实现`

| Path | 分档 | 当前状态 | 备注 |
|------|------|----------|------|
| `sk/query` | `B+` | 已实现 | tracker 主链，支持 UID / `@用户` |
| `sk/line` | `B+` | 已实现 | tracker 主链 |
| `sk/speed` | `B` | 已实现 | tracker 主链，需继续保持 bot 回归覆盖 |
| `sk/check-room` | `B` | 已实现 | tracker 主链 |
| `sk/rank-trace` | `B` | 已实现 | tracker 主链 |
| `sk/player-trace` | `B` | 条件可用 | 依赖 tracker 与目标解析的稳定性 |
| `sk/predict` | `B` | 已实现 | 基于 tracker 的预测线路 |
| `sk/daily-speed` | `B` | 已实现 | tracker 主链 |
| `sk/winrate` | `B-` | 条件可用 | 更依赖真实对战数据 |

### 6.14 Stamp（1）

模块判断：`A- / 稳定`

| Path | 分档 | 当前状态 | 备注 |
|------|------|----------|------|
| `stamp` | `A-` | 稳定 | 范围已经主动收口到贴纸列表 |

### 6.15 VLive（1）

模块判断：`A / 稳定`

| Path | 分档 | 当前状态 | 备注 |
|------|------|----------|------|
| `vlive` | `A` | 稳定 | 最小文本版主链已经完成 |

## 7. 未暴露 / Disabled Handler 清单

截至 2026-04-10，代码库中 **无任何 handler 设置 `Disabled: true`**。

> 历史说明：此前文档曾列 9 个 disabled handler（CardStory、GachaRecord、EventStory、Help、Update、NgWord、UploadHelp、ExtractCard、Heyiwei），经核实这些 handler 已在历次重构中移除，不再存在于代码库中。`CommandHandlerBase` 结构体仍保留 `Disabled bool` 字段，但当前无任何注册 handler 使用该字段。

## 8. Legacy 清理状态

截至 2026-04-10：

- `api/legacy/pjsk/` 空目录已删除（2026-04-10）。
- `api/legacy/` 目录已删除。
- `cmd/server/main.go` 已不再注册 `RegisterPJSKRenderRoutes(...)` 与 `RegisterPJSKCommandRoute(...)`。
- `/internal/pjsk/*` 不再作为运行时暴露路径保留。

当前判断：

- 分档：`D / 已移除`
- 说明：客户端主协议已经完全收口到 `/api/v2/bot/:botId/pjsk/<path>`

## 9. 当前测试快照

当前建议使用两套测试口径：

1. 默认开发口径

```bash
go test ./...
```

2. 显式开启集成测试

```bash
HARUKI_RUN_INTEGRATION=1 go test ./integration -count=1
```

按需覆盖的核心环境变量包括：

- `HARUKI_TEST_BASE_URL`
- `HARUKI_TEST_BOT_ID`
- `HARUKI_TEST_BOT_CREDENTIAL`
- `HARUKI_TEST_PLATFORM`
- `HARUKI_TEST_PLATFORM_USER_ID`
- `HARUKI_TEST_REGION`
- `HARUKI_TEST_GAME_USER_ID`
- `HARUKI_TEST_CREDENTIAL_SIGN_TOKEN`
- `HARUKI_TEST_USERS_DSN`
- `HARUKI_TEST_PJSK_DSN`
- `HARUKI_TEST_SERVER_PUBKEY_HEX`
- `HARUKI_TEST_IMAGE_PATH`

### 9.1 当前整体状态

截至 2026-04-09 本轮稳定化完成后：

- **默认口径 `go test ./...`：全绿**
- `integration` 包：默认 `Skip`
- 集成测试只有在显式设置 `HARUKI_RUN_INTEGRATION=1` 时才真正执行
- 集成测试主配置已支持通过环境变量覆盖，不再需要为了切环境直接改源文件
- 集成测试认证改为按需初始化，不再要求必须先手动跑 `TestAuth`

### 9.2 本轮已修复的失败点

| 位置 | 修复方式 |
|------|----------|
| `api/bot/pjsk` | 修正 `SK speed` 测试预期；补齐 `player-trace` tracker stub |
| `internal/pjsk/handler/sekai` | 将 `education/area` 测试与当前显式参数要求对齐 |
| `internal/pjsk/render/event` | 修正 `EventBrief.EventType` 输出为展示值 |
| `internal/pjsk/render/music` | `ResolveMusicBPM` 补齐普通本地路径 + `asset/{region}-assets/startapp/...` 双路径支持 |
| `internal/pjsk/render/profile` | 测试补齐显式 `CN` source 注册 |
| `integration` | 改为默认关闭，显式环境变量开启；主配置 env 化，并移除对 `TestAuth` 执行顺序的依赖 |

### 9.3 当前说明

当前“全绿”有一个前提：

- `integration` 不再默认强制执行，因此默认 `go test ./...` 反映的是 **单元测试 / 轻量组件测试基线已恢复**。

这符合当前阶段目标：先让主链改动具备稳定反馈面，再在需要时显式跑端到端环境测试。

### 9.4 核心区域稳定情况

下列核心包当前测试通过或总体稳定：

- `internal/pjsk/render/card`
- `internal/pjsk/render/deck`
- `internal/pjsk/render/education`
- `internal/pjsk/render/gacha`
- `internal/pjsk/render/honor`
- `internal/pjsk/render/misc`
- `internal/pjsk/render/music`
- `internal/pjsk/render/mysekai`
- `internal/pjsk/render/profile`
- `internal/pjsk/render/score`
- `internal/pjsk/render/sk`
- `internal/pjsk/render/stamp`
- `internal/pjsk/render/userdata`
- `internal/pjsk/render/vlive`
- `internal/pjsk/parser`
- `internal/pjsk/userdata`
- `api/bot/pjsk`
- `utils/drawing`
- `utils/logger`
- `utils/query`

### 9.5 已移除的失败包记录

以下内容作为历史记录保留，说明为什么曾经需要做 P0/P1/P2：

审计前失败点包括：

```bash
go test ./...
```

历史失败点包括：

| 位置 | 问题 |
|------|------|
| `api/bot/pjsk/handler_test.go:1111` | `sk/speed` 期望 `request_type=tracker`，当前实际为另一种请求类型 |
| `api/bot/pjsk/handler_test.go:1301` | `sk/player-trace` 返回了文本错误而非图片消息 |
| 历史 `api/legacy/pjsk/render_route_test.go:3308` | `routeGachaSource` 未实现 `GetGachaByEventID`（对应文件现已随 legacy 路由移除） |
| `internal/pjsk/handler/sekai/education_test.go:70` | `education/area` 空参数默认行为测试失败 |
| `internal/pjsk/render/event/builder_test.go:147` | 期望 `WorldLink`，实际得到 `world_bloom` |
| `internal/pjsk/render/music/lookup_test.go:198` | 当前环境没有可读取的本地谱面文件，无法查询 BPM |
| `internal/pjsk/render/music/lookup_test.go:241` | 同上 |
| `internal/pjsk/render/profile/controller_test.go:118` | `profile data source is not configured` |
| `integration/api_test.go:260` | `authentication failed` |

## 10. 当前主要风险

### 10.1 安全风险

| 项目 | 状态 | 说明 |
|------|------|------|
| Noise 对端静态公钥白名单 | 不适用 | 已从 IK 迁移至 NK 模式（v17.4），客户端不持有静态密钥，无需白名单校验 |
| `/internal/*` 默认保护强度 | 已收紧 | 未配置鉴权时默认拒绝；支持 `backend.accept_authorization` 或 `haruki_bot.internal_api_token`；仅显式 `allow_insecure_internal_api=true` 才放宽 |

补充说明（2026-04-14 更新）：

- **客户端静态公钥白名单已不适用**
  - v17.4 已将 Noise 从 IK 迁移至 NK 模式。NK 模式下客户端不持有静态密钥对，仅需知道服务端公钥即可建立加密通道。
  - Auth API 通过 AES-256-GCM 固定密钥加密 + credential JWT 验证提供身份认证，不再需要 Noise 层的客户端公钥校验。
  - 原 `secure.go` 中的 `peerStatic` 验证逻辑已在 NK 迁移时移除。

### 10.2 架构过渡风险

| 项目 | 状态 | 说明 |
|------|------|------|
| 强用户态模块正式 provider | 已完成 | `FallbackSnapshotProvider` 已增加 `allowFallback` 标志：生产环境仅使用 Toolbox，失败即报错；开发/测试保留本地 fallback + 警告日志 |
| MySekai 正式数据源 | 已完成 | `NewController` 改用 `MasterdataOptions` 结构体，`AllowFallback=false` 时 DB 失败不回退本地文件；本地 fallback 仅限 dev/test |
| Deck 正式 recommend engine | 已完成 | HTTP 外部服务已增加重试（`max_retries`/`retry_wait_time`）、断路器（连续 5 次失败后拒绝）、结构化日志 |

### 10.3 工程化风险

| 项目 | 状态 | 说明 |
|------|------|------|
| Legacy 兼容路由残留风险 | 已消除 | `api/legacy/pjsk` 已删除，`cmd/server/main.go` 也不再注册 legacy 路由 |
| 后台刷新 goroutine 生命周期 | 已缓解 | 主链刷新任务已接入请求级/服务级可取消上下文；剩余 `context.Background()` 主要在本地调试 helper 与脚本入口兜底 |
| 文档与代码漂移 | 已完成 | 主文档已按“82 条活跃 path / 82 条集成覆盖”统一；其余历史章节按时间快照保留，后续逐步补注“历史口径”标记 |

### 10.4 外部依赖风险

以下能力依赖外部服务或外部资源完整性：

- Drawing API
- ImageCache
- Sekai API
- Toolbox
- Tracker
- 区服资产目录 / masterdata / 本地谱面文件

## 11. 当前推荐优先级

P0-R28、阶段 A-E、Context 注入迁移、P1-P7 清理、以及 6 项收尾治理均已完成：

1. ✅ 文档漂移修正（phantom disabled handler 列表、legacy 目录引用清理）
2. ✅ 快照链路正式化（`FallbackSnapshotProvider` + `allowFallback` 标志）
3. ✅ MySekai 数据源收口（`MasterdataOptions` 结构体 + `AllowFallback` 配置化）
4. ✅ Deck 服务治理（HTTP 重试 + 断路器 + 结构化日志）
5. ✅ CI 模板（`ci.yml` + `integration.yml`）
6. ✅ `context.Background()` 兜底清理（imagecache + bridge 全链路 ctx 传递）

当前推荐优先：

1. ~~为 `secure.go` 增加 Noise 对端静态公钥白名单校验~~ — 已不适用（NK 模式无客户端公钥）。
2. 视调用方现状决定是否进一步统一内部服务鉴权字段。
3. 启用 CI integration workflow（当前为手动触发模板，待基础设施就绪后改为自动触发）。
4. 持续联调 bug 修复和回归测试覆盖扩展。

### 2026-04-10 收尾治理完成后最终状态

| 指标 | 数量 |
|------|------|
| context.TODO() in provider | 0（从 56 减至 0）|
| interface{} | 0（394 处已迁移为 any）|
| contextual 包装层 | 已删除（~400 行）|
| 所有生产文件 | <375 行 |
| go build / vet / test | 全部通过（35 包）|
| CI workflows | 2（ci.yml + integration.yml）|
| 快照 / MySekai fallback | 可配置（AllowFallback 标志）|
| Deck 重试 / 断路器 | 已实装（max_retries + circuit breaker）|
| imagecache + bridge ctx | 已完成（全链路 rc.Ctx）|

### 2026-04-14 实战联调稳定化（v17.5）

在 v17.4（Noise NK + Auth AES 固定密钥）之后，04-12 ~ 04-14 产出 33 个提交，集中在客户端联调 bug 修复和安全加固。项目已从"架构重构收尾"进入"实战联调稳定化"阶段。

**本轮主要变更分类**：

| 分类 | 数量 | 代表性变更 |
|------|------|-----------|
| 安全加固 | 3 | 请求去重锁（`808439d`）、URL 脱敏（`acce4c4`）、账号 ID 隐藏（`5cc5c19`）|
| 认证/注册 | 2 | Auth IP 上报 + 注册开关（`326c90b`）、provision_bot CLI（`aeb1ec9`）|
| Profile/绑定修复 | 5 | 跨区服编号（`6c02169`）、跨区服解绑（`e87c1a9`）、背景缓存（`44c5c43`）等 |
| 渲染/快照修复 | 10 | suite snapshot（`5d7f9b2`）、card parameter（`b29445b` `fc47512`）、rewards 多形态解码（`7893c90`）等 |
| Deck 修复 | 4 | recommend fallback（`3c44203`）、masterdata 同步（`131ddfd`）、binding 选择器（`d1fb1a0`）等 |
| Education/MySekai 区域 | 7 | CN area item（`b9da188`）、region-scoped masterdata（`4751db9`）、education region 传递等 |
| Event 修复 | 1 | WL 冲榜记录单榜拆分（`7893c90`）|
| 回归测试 | 3 | card parameter 解码（`c2d84ed`）、rewards snapshot 解码（`7893c90`）、event WL 拆分（`7893c90`）|
| 文档 | 1 | known-bugs.cn.md 追踪表（`ea3d595`）|

**已知 bug 追踪**：新增 `docs/known-bugs.cn.md`，当前 8 个已追踪 bug 全部已修复。

**完成度更新**：

| 指标 | v17.4（04-12） | v17.5（04-14） |
|------|----------------|----------------|
| 重构进度 | ~97-98% | ~98-99% |
| 整体交付 | ~93-95% | ~95-97% |
| 协议/鉴权分档 | A- | A |
| 实战联调 bug 修复 | — | 33 commits |
| 已知 bug 追踪 | 无 | 8/8 已修复 |


## 12. 维护说明

后续更新本文件时，建议遵守：

1. 新增 Bot path 时，同时更新第 4 节、第 5 节和第 6 节。
2. 某 path 被移除或改为 disabled 时，从第 6 节移出，并挪到第 7 节。
3. `go test ./...` 状态发生变化时，更新第 9 节。
4. 新的主风险或架构决策，统一补到第 10 节和第 11 节。

---

**维护建议**：后续若继续保留 [project-status-summary.cn.md](project-status-summary.cn.md) 作为长篇阶段日志，则本文件应保持“短周期更新、高密度跟踪”的定位，不再重复记录逐轮详细联调流水。  
