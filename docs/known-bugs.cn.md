# Known Bugs & Issues

记录近期排查过的 bug、待解决问题及修复信息。

## 待解决问题

> 以下问题由 2026-04-14 API 层审计发现，按严重度排列。

| ID | 严重度 | 模块 | 描述 | 状态 | 发现时间 |
|----|--------|------|------|------|----------|
| ISSUE-001 | P0 | bot/pjsk/handler | `parseBotRequest()` 中两行 `logger.Infof` 将每个请求的完整 OneBot 消息内容打印到 INFO 日志，生产环境存在用户隐私泄露风险和日志噪音 | 📋 测试期保留 | 2026-04-14 |
| ISSUE-002 | P1 | bot/auth/helper | `checkRateLimit()` 每次 `Set` 都重置 Redis key TTL，导致高频用户的限流窗口永远不过期 | ✅ 已修复 | 2026-04-14 |
| ISSUE-003 | P1 | api/helper | `VerifyAPIAuthorization()` 当仅配置 `AcceptUserAgent` 而未配置 `AcceptAuthorization` 时，内部 API 仅靠可伪造的 User-Agent 鉴权 | ✅ 已修复 | 2026-04-14 |
| ISSUE-004 | P1 | server | `createFiberApp()` 未注册 `recover` 中间件，handler panic 会导致空响应而非 500 错误 | ✅ 已修复 | 2026-04-14 |
| ISSUE-005 | P2 | bot/auth/user | `Logout` 端点注册在 public group 无需 session 验证，任意知道 bot_id 的人可删除 session（DoS） | ✅ 已修复 | 2026-04-14 |
| ISSUE-006 | P2 | bot/auth/statistics | `/bot/statistics/record/:botID` 路由前缀与其他内部路由（`/internal/bot/*`）不一致 | ✅ 已修复 | 2026-04-14 |
| ISSUE-007 | P2 | bot_session_middleware | 同一 bot_id 只能有一个活跃 session（后登录踢前一个），该行为未在客户端对接文档中说明 | ✅ 已文档化 | 2026-04-14 |

## 已修复 Bug

| ID | 模块 | 描述 | 状态 | 修复人 | 修复时间 | 相关提交 |
|----|------|------|------|--------|----------|----------|
| BUG-001 | profile/binding | `/绑定列表` 不带区服前缀时按区服分别编号，导致同一序号指向不同区服账号，与 `/设置默认绑定 u[i]` 的全局定位逻辑不一致 | ✅ 已修复 | Copilot | 2026-04-14 | `6c02169` |
| BUG-002 | profile/binding | `/设置默认绑定 u[i]` 不带区服前缀时 Server 参数默认为 jp，导致跨区服选号失败（"超出范围"） | ✅ 已修复 | Copilot | 2026-04-14 | `6c02169` |
| BUG-003 | profile/binding | `/取消绑定 u[i]` 不带区服前缀时 Server 参数默认为 jp，导致跨区服解绑失败 | ✅ 已修复 | Copilot | 2026-04-14 | `e87c1a9` |
| BUG-004 | sekai/api | SekaiAPI / Toolbox / Tracker 网络错误未脱敏，原始内部 URL 泄露至客户端响应 | ✅ 已修复 | Copilot | 2026-04-14 | `acce4c4` |
| BUG-005 | bot/dedup | Bot 请求未去重，同一用户同一指令在短时间内可触发多次响应 | ✅ 已修复 | Copilot | 2026-04-14 | `7a06685` |
| BUG-006 | profile/bg | 自定义背景图片文件名固定（非随机），更新图片后参数不变导致客户端使用旧缓存 | ✅ 已修复 | Copilot | 2026-04-13 | — |
| BUG-007 | music/rewards | `/打歌奖励` 在 CN 等区服即使有可用 Suite 数据，也可能因 `userMusicAchievements` 字段形态差异误判为“无法读取 Suite 数据”并退回预估模式 | ✅ 已修复 | Codex | 2026-04-14 | `7893c90` |
| BUG-008 | event/record | `/冲榜记录` 会把 WL 单榜记录按活动聚合，导致单榜 PT/Rank 与总榜混淆，或只显示总榜不显示单榜 | ✅ 已修复 | Codex | 2026-04-14 | `7893c90` |
| BUG-009 | card/masterdata | CN 卡牌参数存在对象形态时，Cloud 仅按数组解码，导致查卡、卡牌一览等接口报 `decode card parameters` | ✅ 已修复 | Codex | 2026-04-14 | `fc47512` |
| BUG-010 | deck/config | alpha 原生部署未稳定同步 deck masterdata 与 WL support 数据，导致跨服活动组卡配置不完整 | ✅ 已修复 | Codex | 2026-04-14 | `131ddfd` |
| BUG-011 | deck/fallback | 无活动组卡默认事件/歌曲回退逻辑不稳定，最强组卡和长草组卡在部分输入下会退到错误查询分支 | ✅ 已修复 | Codex | 2026-04-14 | `3c44203` |
| BUG-012 | card/region | 查卡与卡牌一览在 bridge 合并参数后会被旧 `region` 覆盖回去，跨服查询可能错误落到 JP | ✅ 已修复 | Codex | 2026-04-14 | `36a950b` |
| BUG-013 | profile/honor | `皆传`、`FC`、`AP` 牌子数字直接读取 honor level，而不是玩家真实 FC/AP 聚合计数，导致显示与实际不符 | ✅ 已修复 | Codex | 2026-04-14 | `36a950b` |
| BUG-014 | music/list | 难度排行闭区间等级解析只支持单 token，`31 32`、`31到32`、`[31,32]` 等写法无法使用 | ✅ 已修复 | Codex | 2026-04-14 | `36a950b` |
| BUG-015 | deck/world-bloom | 组卡章节参数最初只支持 `wl1`/`wl2`，`wl3`/`wl4` 无法解析；当前 WL 活动下裸 `wl3`、`歌曲名 wl3`、`活动ID 歌曲名 wl3` 这类顺序还会掉回歌曲参数 | ✅ 已修复 | Codex | 2026-04-14 | `24cb148` |
| BUG-016 | music/list | 难度排行分组内排序依赖发布时间，Drawing 侧再次按 `release_at` 重排后，视觉顺序仍然会乱 | ✅ 已修复 | Codex | 2026-04-14 | `24cb148` |
| BUG-017 | card/music visibility | 查卡、查歌会返回未上线 masterdata；`查歌 -1` 也会把未上线歌曲计入“最近一首/倒数第 N 首” | ✅ 已修复 | Codex | 2026-04-14 | `24cb148` |
| BUG-018 | music/detail | 查歌详情图的 `Alias` 字段只展示本地标题/读音/tag，没有并入已审核歌曲别名，导致“查歌别名没接上”的观感问题 | ✅ 已修复 | Codex | 2026-04-14 | 本次提交 |
| BUG-019 | mysekai/blueprint | `mysekai` 角色昵称表缺少 `akt`、`khn`、`tks` 等 compact alias 与若干常见名字写法，导致 `/msb akt` 一类输入无法识别角色并错误回退到家具列表 | ✅ 已修复 | Codex | 2026-04-14 | 本次提交 |
| BUG-020 | mysekai/resource | `msa` 顶部天气/现象卡片过度信任 `updatedResources.now`，当该时间戳落后于 `upload_time` 时会错误保留生日刷新槽，导致只有部分账号显示出异常天气时间段 | ✅ 已修复 | Codex | 2026-04-14 | 本次提交 |
| BUG-021 | card/parser | `/查卡 -1` 会把负号后的 `1` 误当成筛选 token，落到一星卡过滤而不是“当前区服最新已上线卡”语义 | ✅ 已修复 | Codex | 2026-04-14 | 本次提交 |
| BUG-022 | music/detail | 查歌详情图请求体预留了 `bpm` 字段，但构建阶段没有填充，导致详情图长期不展示 BPM | ✅ 已修复 | Codex | 2026-04-14 | 本次提交 |
