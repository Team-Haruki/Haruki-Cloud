# Haruki-Cloud PJSK Event Tracker 对接说明

> 最后更新：2026-03-25
>
> 本文档记录当前 `event tracker` 在 Haruki-Cloud 中的接入位置、调用链路、参数协议、能力边界与测试覆盖，作为本轮对接的落地说明。

## 1. 文档目的

这轮改造不是“加一个独立 API”，而是把 SK 相关能力统一接入同一条 tracker 数据链路，并保持 Bot 入口和旧 render 路由的兼容性。

本文重点回答以下问题：

1. tracker 在哪里接入
2. 现在哪些命令已经走 tracker
3. `@用户` 查询是如何落地的
4. 目前哪些模式仍然是“仅排名模式”

## 2. 当前结论

截至 2026-03-25，Event Tracker 对接状态如下：

1. SK 主命令链路已经接入 tracker
2. 旧的 `/internal/pjsk/sk/*` 路由已补齐 tracker build/render 入口
3. Bot `/sk`、`/skl` 已支持 `@用户` 查询（通过绑定解析目标 UID）
4. `sk-speed`、`sk-check-room`、`sk-rank-trace` 仍保持仅排名模式（设计如此）

## 3. 接入位置

### 3.1 tracker client

`utils/sekai/client_tracker.go`

提供对 tracker 服务的 HTTP 封装，包括：

1. `GetLatestRankingByRank` / `GetLatestRankingByUser`
2. `GetLatestWorldBloomRankingByRank` / `GetLatestWorldBloomRankingByUser`
3. `GetRankingScoreGrowth` / `GetWorldBloomRankingScoreGrowth`
4. `TraceRankingByRank` / `TraceWorldBloomRankingByRank`

配置来源：

1. `config.Cfg.Tracker.BaseURL`
2. `config.Cfg.Tracker.UserAgent`

### 3.2 SK Controller

`internal/pjsk/render/sk/controller.go`

新增/统一的 tracker 入口：

1. `BuildLineRequestFromTracker`
2. `BuildQueryRequestFromTracker`
3. `BuildCheckRoomRequestFromTracker`
4. `BuildSpeedRequestFromTracker`
5. `BuildRankTraceRequestFromTracker`

其中 `TrackerRankQuery` 作为统一输入参数结构，关键字段为：

1. `event_id`
2. `region`
3. `ranks`
4. `user_id`
5. `wl_character_id`
6. `full`
7. `target_platform` / `target_user_id`（用于 `@用户` 绑定解析）
8. `event_name` / `event_start_at` / `event_aggregate_at` / `banner_img_path`（可选覆盖）

### 3.3 运行时注入

`internal/pjsk/render/app/app.go`

初始化时会将 tracker client 注入 SK controller：

1. `skController.SetTrackerIntegration(sekaiutil.GetTrackerClient(), nil, assetHelper)`
2. 有 Sekai masterdata source 时再注入 event source，用于事件元数据和当前活动推断

### 3.4 执行桥

`internal/pjsk/handler/bridge.go`

`executeSK(...)` 会优先尝试将 `ResolvedCommand.Params` 反序列化为 `TrackerRankQuery`：

1. 命中则走 `Build*FromTracker(...)`
2. 未命中则走历史渲染请求结构（兼容旧调用）

这保证了新旧两种参数协议可以共存。

### 3.5 命令参数构建

`internal/pjsk/handler/sekai/sk.go`

`buildSKTrackerParams(...)` 已统一产出 tracker 风格参数：

1. 支持 `event101` / `e101`
2. 支持 `wl5` / `cid5` / `chara5` 等 WB 角色参数
3. 支持单排名、多排名、区间排名
4. 对允许 UID 的命令支持用户查询
5. 处理 `UIDArg` 并在 `@` 场景写入 `target_platform` / `target_user_id`

## 4. 调用链路

### 4.1 Bot 指令链路（主用）

```text
/api/v2/bot/:botId/pjsk/sk/*
  -> sekai SK handler 构建 tracker params
  -> Execute -> executeSK
  -> Build*FromTracker
  -> drawing client
```

### 4.2 legacy build/render 链路（兼容）

`api/legacy/pjsk/render_route.go` 已补齐以下端点：

1. `/internal/pjsk/sk/line/tracker/build` + `/render`
2. `/internal/pjsk/sk/query/tracker/build` + `/render`
3. `/internal/pjsk/sk/check-room/tracker/build` + `/render`
4. `/internal/pjsk/sk/speed/tracker/build` + `/render`
5. `/internal/pjsk/sk/rank-trace/tracker/build` + `/render`

## 5. `@用户` 查询落地方案

当前实现不是“直接把 `@qq` 当游戏 UID”，而是两阶段：

1. 命令层提取 `@qq`，写入 `target_platform` + `target_user_id`
2. 执行桥根据目标平台用户的绑定关系，按区服选出游戏 UID，再填入 `user_id`

绑定选择优先级：

1. 该区服默认绑定
2. 该区服全局默认绑定
3. 该区服首个可见绑定

前提条件：

1. `app.Bindings` 已配置且 `IsReady() == true`
2. 被 `@` 用户在目标区服有可用绑定

## 6. 能力矩阵

当前 SK 相关模式能力如下。

| 模式 | tracker 接入 | 用户查询（UID / @） |
|---|---|---|
| `sk-query` | 已接入 | 支持 |
| `sk-line` | 已接入 | 支持 |
| `sk-speed` | 已接入 | 不支持（仅排名） |
| `sk-check-room` | 已接入 | 不支持（仅排名） |
| `sk-rank-trace` | 已接入 | 不支持（仅排名） |
| `sk-player-trace` | 维持原有逻辑 | N/A |
| `sk-winrate` | 维持原有逻辑 | N/A |

说明：

1. `sk-speed` / `sk-check-room` / `sk-rank-trace` 的“仅排名”限制是既有设计，不是本轮临时限制。

## 7. 已覆盖测试

当前对接已覆盖以下维度：

1. Bot 端 `sk` tracker 请求构建与渲染调用
2. Bot 端 UID 查询
3. Bot 端 `@用户` 查询（绑定解析）
4. SK 参数解析与 tracker 参数构建
5. legacy render tracker 路由 build/render

主要测试文件：

1. `api/bot/pjsk/handler_test.go`
2. `internal/pjsk/handler/sekai/sk_tracker_params_test.go`
3. `api/legacy/pjsk/render_route_test.go`

## 8. 注意事项

1. 若部署环境未配置 tracker `base_url`，SK tracker 路径会报 `tracker client is not configured`。
2. `@用户` 查询依赖绑定服务可用；若绑定服务未注入，会提示改用游戏 UID。
3. World Bloom 场景下若未给 `wl_character_id`，会返回 `world bloom event requires wl_character_id`。
4. 当请求未提供 `event_id` 且无法从 event source 推断当前/下一期活动时，会要求显式传 `event_id`。

