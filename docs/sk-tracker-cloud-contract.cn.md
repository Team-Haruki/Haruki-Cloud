# SK Tracker / Cloud 语义契约

> 最后更新：2026-06-18

本文记录 Haruki-Cloud 的 SK 渲染链路对 Haruki-Event-Tracker v2 cloud API 的语义需求，以及 Tracker 侧需要保证的行为。它不是临时排障记录，而是 Cloud、Tracker、Web 协同演进时的接口契约。

## 背景

Cloud 已经把 SK 榜线、查房、时速、玩家轨迹等路径迁移到 Tracker v2 cloud API。Cloud 不再希望用大量低层 latest / trace 小接口拼装语义，而是消费 Tracker 返回的排行榜语义结果。

因此 Tracker 返回的字段必须表达清楚"这是档线增长"还是"当前玩家周回"。两者都可能叫 speed，但业务含义不同，不能互相覆盖。

## v1 → v2 API 映射

Cloud 从 v2.5.12 起逐步淘汰 Tracker v1 低层接口，迁移到 v2 语义化端点。下表列出 v1 旧接口及其 v2 替代：

| v1 旧接口 | v1 HTTP 路径 | v2 替代端点 | v2 HTTP 路径 |
|-----------|-------------|------------|-------------|
| `GetLatestRankingByRank` | `/event/{server}/{eventID}/latest-ranking/rank/{rank}` | `/sk/query` | `/api/v2/cloud/events/{server}/{eventID}/leaderboards/{scope}/sk/query` |
| `GetLatestRankingByUser` | `/event/{server}/{eventID}/latest-ranking/user/{userID}` | `/sk/query` (userId 参数) | 同上 |
| `GetLatestWorldBloomRankingByRank` | `/event/{server}/{eventID}/latest-world-bloom-ranking/character/{characterID}/rank/{rank}` | `/sk/query` (scope=world-bloom/{cid}) | 同上 |
| `GetLatestWorldBloomRankingByUser` | `/event/{server}/{eventID}/latest-world-bloom-ranking/character/{characterID}/user/{userID}` | `/sk/query` (userId + scope) | 同上 |
| `TraceRankingByRank` | `/event/{server}/{eventID}/trace-ranking/rank/{rank}` | `/sk/trace` (subjectType=rank) | `/api/v2/cloud/events/{server}/{eventID}/leaderboards/{scope}/sk/trace` |
| `TraceRankingsByRanks` | `/event/{server}/{eventID}/trace-ranking/ranks?rank=...` | `/sk/line` (多 rank 批量) 或 `/sk/trace` (subjectType=rank) | `/sk/line` 或 `/sk/trace` |
| `TraceRankingByUser` | `/event/{server}/{eventID}/trace-ranking/user/{userID}` | `/sk/trace` (subjectType=user) | 同上 |
| `TraceWorldBloomRankingByRank` | `/event/{server}/{eventID}/trace-world-bloom-ranking/character/{cid}/rank/{rank}` | `/sk/trace` (scope + subjectType=rank) | 同上 |
| `TraceWorldBloomRankingsByRanks` | `/event/{server}/{eventID}/trace-world-bloom-ranking/character/{cid}/ranks?rank=...` | `/sk/line` (scope + 多 rank) | `/sk/line` |
| `TraceWorldBloomRankingByUser` | `/event/{server}/{eventID}/trace-world-bloom-ranking/character/{cid}/user/{userID}` | `/sk/trace` (scope + subjectType=user) | 同上 |
| `GetRankingLines` | `/event/{server}/{eventID}/ranking-lines` | `/sk/line` | `/api/v2/cloud/events/{server}/{eventID}/leaderboards/{scope}/sk/line` |
| `GetWorldBloomRankingLines` | `/event/{server}/{eventID}/world-bloom-ranking-lines/character/{cid}` | `/sk/line` (scope) | 同上 |
| `GetRankingScoreGrowth` | `/event/{server}/{eventID}/ranking-score-growth/interval/{interval}` | `/sk/speed` | `/api/v2/cloud/events/{server}/{eventID}/leaderboards/{scope}/sk/speed` |
| `GetWorldBloomRankingScoreGrowth` | `/event/{server}/{eventID}/world-bloom-ranking-score-growth/character/{cid}/interval/{interval}` | `/sk/speed` (scope) | 同上 |
| `GetUserEventData` | `/event/{server}/{eventID}/user-data/{userID}` | `/sk/query` (userId 参数) | `/sk/query` |
| `GetEventStatus` | `/event/{server}/{eventID}/status` | `/sk/status` | `/api/v2/cloud/events/{server}/{eventID}/leaderboards/total/sk/status` |

**关键变化**：

1. v1 的 World Bloom 端点通过独立 `/character/{cid}/` 路径段区分角色榜；v2 统一使用 `{scope}` 路径段（`total` 或 `world-bloom/{cid}`）。
2. v1 的 latest-ranking 返回单条快照（`LatestRankingResponse`）；v2 的 `/sk/query` 返回语义化结果（`CloudRankQueryResponse`），支持多 rank、adjacent、周回指标等。
3. v1 的 trace-ranking 返回原始点数组；v2 的 `/sk/trace` 通过 `subjectType` 显式区分玩家轨迹和档线轨迹，避免语义混淆。
4. v1 的 ranking-lines / score-growth 是独立小接口；v2 的 `/sk/line` 和 `/sk/speed` 把它们的语义合并到统一端点。
5. v1 `TraceRankingsByRanks` 的批量 trace 在 v2 中被 `/sk/line`（榜单快照语义）和 `/sk/trace`（轨迹语义）替代，不再需要批量 raw trace 拼装 line 数据。

## 路径参数约定

所有 v2 端点共享以下路径结构：

```text
/api/v2/cloud/events/{server}/{event_id}/leaderboards/{scope}/sk/{endpoint}
```

### `{server}`

区服标识：`jp`、`en`、`tw`、`kr`、`cn`。

### `{event_id}`

活动 ID（整数），由 Cloud 从 Sekai master data 解析。

### `{scope}`

排行榜 scope，决定查询对象是全服榜还是角色榜：

| scope 值 | 含义 |
|----------|------|
| `total` | 全服榜（normal event 默认） |
| `world-bloom/{character_id}` | World Bloom 角色榜（`character_id` 为游戏角色 ID 整数） |

Cloud 侧通过 `trackerLeaderboardScope(characterID)` 函数生成：无 `characterID` 或 `characterID <= 0` → `total`；否则 → `world-bloom/{characterID}`。

## Cloud 消费路径

### query / 查榜

Cloud 入口：

- `BuildQueryRequestFromTracker`
- `internal/pjsk/render/sk/controller_query_requests.go`

Tracker API：

```text
GET /api/v2/cloud/events/{server}/{event_id}/leaderboards/{scope}/sk/query
```

请求参数：

| 参数 | 类型 | 必须 | 说明 |
|------|------|------|------|
| `rank` | int (重复) | 至少一个 rank 或 userId | 目标排名列表 |
| `userId` | int64 | 至少一个 rank 或 userId | 目标用户 ID |
| `interval` | int64 | 否 | 速度计算窗口秒数，默认 3600 |
| `includeAdjacent` | bool | 否 | 是否返回 previous/next 相邻档位 |
| `skipMissing` | bool | 否 | 是否跳过无数据的 rank |

响应类型 `CloudRankQueryResponse`：

```json
{
  "meta": { "server": "jp", "eventId": 101, "scope": "total", "fetchedAt": 1704067200 },
  "ranks": [ { ... CloudRankInfo ... } ],
  "previous": { ... CloudRankInfo ... },  // 仅 includeAdjacent=true
  "next": { ... CloudRankInfo ... }        // 仅 includeAdjacent=true
}
```

Cloud 用途：

- 生成 `sk/query` drawing payload。
- 多 rank 查询时展示每个 rank 当前快照。
- 单 rank 查询时需要目标 rank，以及相邻档位。
- 按用户查询时需要解析到当前 rank，并给出相邻档位。

字段语义：

- `rank`、`userId`、`name`、`score`、`timestamp`：当前榜快照。
- `previous` / `next`：相邻档位快照，只需要 latest 快照语义，不要求周回指标。
- `speed` 如果返回，必须是与该 endpoint 约定一致的当前玩家近 1 小时速度；如果 Tracker 无法保证，不应返回误导性值。

### check-room / 查房

Cloud 入口：

- `BuildCheckRoomRequestFromTracker`
- `internal/pjsk/render/sk/controller_query_requests.go`

Tracker API：

```text
GET /api/v2/cloud/events/{server}/{event_id}/leaderboards/{scope}/sk/check-room
```

请求参数：

| 参数 | 类型 | 必须 | 说明 |
|------|------|------|------|
| `rank` | int (重复) | 至少一个 rank 或 userId | 目标排名列表 |
| `userId` | int64 | 至少一个 rank 或 userId | 目标用户 ID |
| `interval` | int64 | 否 | 速度计算窗口秒数，默认 3600 |
| `includeAdjacent` | bool | Cloud 固定传 true | 是否返回 previous/next |
| `skipMissing` | bool | 否 | 是否跳过无数据的 rank |

响应类型 `CloudCheckRoomResponse`：

```json
{
  "meta": { "server": "jp", "eventId": 101, "scope": "total", "fetchedAt": 1704067200 },
  "rank": { ... CloudRankInfo ... },       // 单人查房时的主体
  "ranks": [ { ... CloudRankInfo ... } ],  // 多人查房时的主体列表
  "previous": { ... CloudRankInfo ... },
  "next": { ... CloudRankInfo ... }
}
```

Cloud 用途：

- 生成 `sk/check-room` drawing payload。
- `cncf 1-10` 这类多 rank 查房会展示每个目标 rank 的玩家周回情况。
- 查房只支持前 100 名，Cloud 会继续做 rank 范围校验。

字段语义：

- `rank`、`userId`、`name`、`score`、`timestamp`：当前榜快照。
- `speed`：当前玩家最近约 1 小时速度，按点差和实际 elapsed 归一到每小时。
- `latestPt`：当前玩家最新一次正向分数增量。
- `averageRound`：当前玩家最近若干次正向增量的局数计数。
- `averagePt`：当前玩家最近若干次正向增量的平均单局 pt。
- `hourRound`：当前玩家最近约 1 小时窗口内的正向增量次数。
- `min20Times3Speed`：当前玩家最近约 20 分钟增量乘 3。
- `recordStartAt`：用于这些周回指标的 trace 样本起点。
- `previous` / `next`：相邻档位 latest 快照，不要求玩家周回指标。

这些周回字段必须来自"当前 rank 对应的当前玩家 user trace"，不是 rank trace。World Bloom 档位频繁换人时，rank trace 会混入不同玩家历史点，不能用于查房周回。

### line / 榜线

Cloud 入口：

- `BuildLineRequestFromTracker`
- `BuildPredictLineRequestFromTracker`
- `internal/pjsk/render/sk/controller_line_requests.go`
- `internal/pjsk/render/sk/controller_line_tracker_fast.go`

Tracker API：

```text
GET /api/v2/cloud/events/{server}/{event_id}/leaderboards/{scope}/sk/line
```

请求参数：

| 参数 | 类型 | 必须 | 说明 |
|------|------|------|------|
| `rank` | int (重复) | 至少一个 rank 或 userId | 目标排名列表 |
| `userId` | int64 | 否 | 目标用户 ID（按用户查榜线） |
| `interval` | int64 | 否 | 速度计算窗口秒数，默认 3600 |
| `skipMissing` | bool | 否 | 是否跳过无数据的 rank |

响应类型 `CloudLineResponse`：

```json
{
  "meta": { "server": "jp", "eventId": 101, "scope": "total", "fetchedAt": 1704067200 },
  "ranks": [ { ... CloudRankInfo ... } ]
}
```

Cloud 用途：

- 生成 `sk/line` drawing payload（榜单快照图）。
- v1 中 Cloud 用 `TraceRankingsByRanks` / `TraceWorldBloomRankingsByRanks` 的批量 trace 拼装 line 数据：取每个 rank 的 trace 最后一个点作为快照，再用 trace 点计算本地 metrics。
- v2 中 line endpoint 直接返回语义化的 rank 快照列表，Cloud 不再需要从 trace 拼装。
- line 渲染重点关注分数档位，Cloud 会将 `name` 清空以减少视觉噪音。

字段语义：

- `rank`、`score`、`timestamp`：当前档位快照。
- `speed` 如果返回，应为档线增长语义（与 `/sk/speed` 一致），不是玩家周回语义。
- `name`：Cloud line 渲染中清空不展示，但 Tracker 应正常返回。

### speed / 时速

Cloud 入口：

- `BuildSpeedRequestFromTracker`
- `internal/pjsk/render/sk/controller_speed_requests.go`

Tracker API：

```text
GET /api/v2/cloud/events/{server}/{event_id}/leaderboards/{scope}/sk/speed
```

请求参数：

| 参数 | 类型 | 必须 | 说明 |
|------|------|------|------|
| `rank` | int (重复) | 是 | 目标排名列表 |
| `interval` | int64 | 是 | 观察窗口秒数 |
| `unitSeconds` | int64 | 是 | 归一化单位秒数（3600=时速，86400=日速） |
| `skipMissing` | bool | 否 | 是否跳过无数据的 rank |

响应类型 `CloudSpeedResponse`：

```json
{
  "meta": { ... },
  "speeds": [ { ... CloudRankInfo ... } ],
  "intervalSeconds": 3600,
  "unitSeconds": 3600
}
```

Cloud 用途：

- 生成 `sk/speed` 和 `sk/daily-speed` drawing payload。
- 不支持按用户查询，只支持 rank 列表。
- `interval` 是观察窗口，`unitSeconds` 是归一化单位，例如 3600 表示时速，86400 表示日速。
- v1 中 Cloud 用 `GetRankingScoreGrowth` / `GetWorldBloomRankingScoreGrowth` 获取 growth 点，再本地 fallback 到 trace 计算；v2 中 Tracker 直接返回归一化后的 speed。

字段语义：

- `speed`：档线在 `interval` 内的分数增长，按 `unitSeconds / interval` 归一化。
- `speedWindow`：实际用于计算的窗口秒数。
- `score`、`timestamp`：当前 rank 快照。

该 endpoint 表示"rank 档线增长"，不是"当前玩家周回"。它可以与 check-room 的 `speed` 数值不同。

### trace / 轨迹与 CSB

Cloud 入口：

- player trace / rank trace / CSB 相关 controller
- `internal/pjsk/render/sk/controller_tracker_v2.go`
- `internal/pjsk/render/sk/controller_trace.go`
- `internal/pjsk/render/sk/controller_trace_rank.go`
- `internal/pjsk/render/sk/controller_trace_user.go`

Tracker API：

```text
GET /api/v2/cloud/events/{server}/{event_id}/leaderboards/{scope}/sk/trace
```

请求参数：

| 参数 | 类型 | 必须 | 说明 |
|------|------|------|------|
| `subjectType` | string | 是 | `user` 或 `rank` |
| `subject` | string | 是 | 用户 ID（`subjectType=user`）或排名（`subjectType=rank`） |
| `limit` | int | 否 | 返回点数量上限；Cloud 默认不传，表示请求完整 trace |

响应类型 `CloudTraceResponse`：

```json
{
  "meta": { ... },
  "subject": { "subjectType": "user", "subject": "123456789", "resolvedUserId": "123456789", "resolvedRank": 42 },
  "rankData": [ { ... CloudRankInfo ... } ]
}
```

Cloud 用途：

- `subjectType=user`：画玩家轨迹。
- `subjectType=rank`：画档线轨迹。
- CSB 应追踪当前玩家，而不是 rank 档线历史。Cloud 现在会优先解析当前 rank owner，再用 user trace。
- v1 中 Cloud 调 `TraceRankingByRank` / `TraceWorldBloomRankingByRank` 获取 rank trace 后本地计算 metrics；v2 中 Cloud 对 check-room/query 使用 Tracker 返回的周回字段，仅当周回字段缺失时才 fallback 到 `/sk/trace`。

字段语义：

- `subjectType=user` 返回同一玩家的历史点。
- `subjectType=rank` 返回该 rank 档线历史点，允许不同 `userId` 混合。
- `subjectType=rank` 不应被 Cloud 当成玩家周回数据。

### status / 心跳

Cloud 入口：

- tracker warmup / stale record 检测
- `internal/pjsk/render/sk/controller_stale_record.go`

Tracker API：

```text
GET /api/v2/cloud/events/{server}/{event_id}/leaderboards/total/sk/status
```

响应类型 `EventStatusResponse`：

```json
{
  "timestamp": 1704067200000,
  "status": 1,
  "statusDesc": "running",
  "timeAgo": 30000
}
```

Cloud 用途：

- 检测 Tracker 对指定活动是否仍在正常运行。
- `timeAgo` 超过阈值时标记为 stale record，提醒用户数据可能不新鲜。
- v1 路径为 `/event/{server}/{eventID}/status`，v2 路径改为 `/sk/status`。

注意：status endpoint 固定 scope 为 `total`，因为 Tracker 心跳与 scope 无关。

## 共享响应类型 CloudRankInfo

所有 v2 端点的核心数据单元是 `CloudRankInfo`，字段定义如下：

```json
{
  "rank": 100,
  "userId": "1234567890",
  "name": "PlayerName",
  "score": 1234567,
  "timestamp": 1704067200000,

  // 玩家周回指标（query / check-room 语义）
  "averageRound": 10,
  "averagePt": 50000,
  "latestPt": 55000,
  "speed": 1800000,
  "min20Times3Speed": 5400000,
  "hourRound": 8,
  "recordStartAt": 1704063600000,

  // 档线增长指标（speed 语义）
  "speedWindow": 3600,

  // World Bloom 角色榜标识
  "characterId": 6
}
```

字段语义区分：

| 字段 | 语义来源 | 适用 endpoint | 说明 |
|------|---------|--------------|------|
| `rank`、`userId`、`name`、`score`、`timestamp` | 快照 | 全部 | 当前榜快照基础字段 |
| `speed` | 玩家周回 | `/sk/query`、`/sk/check-room` | 当前玩家近 1 小时速度（归一到每小时） |
| `speed` | 档线增长 | `/sk/speed` | rank 档线在 interval 内的分数增长（归一到 unitSeconds） |
| `speedWindow` | 档线增长 | `/sk/speed` | 实际计算窗口秒数 |
| `latestPt` | 玩家周回 | `/sk/query`、`/sk/check-room` | 最近一次正向分数增量 |
| `averageRound` | 玩家周回 | `/sk/query`、`/sk/check-room` | 最近若干正向增量的局数 |
| `averagePt` | 玩家周回 | `/sk/query`、`/sk/check-room` | 最近若干正向增量的平均单局 pt |
| `hourRound` | 玩家周回 | `/sk/query`、`/sk/check-room` | 近 1 小时正向增量次数 |
| `min20Times3Speed` | 玩家周回 | `/sk/query`、`/sk/check-room` | 近 20 分钟增量乘 3 |
| `recordStartAt` | 玩家周回 | `/sk/query`、`/sk/check-room` | 周回指标样本起点时间 |
| `characterId` | scope 标识 | World Bloom 端点 | 角色榜角色 ID |

**重要**：`speed` 字段在不同 endpoint 有不同语义。Tracker 和 Cloud 必须在契约层面明确这一点，不能假设 `speed` 总是同一种含义。

## Tracker 必须保证的行为

### 1. 明确区分玩家周回和档线增长

Tracker 需要把以下两类指标分开：

- 玩家周回指标：用于 `sk/query` / `sk/check-room` 的 `latestPt`、`averageRound`、`averagePt`、`hourRound`、`min20Times3Speed`、`recordStartAt`、近 1 小时 `speed`。
- 档线增长指标：用于 `sk/speed` 的 `speed` 和 `speedWindow`。

不要用 rank trace 覆盖玩家周回指标，也不要用玩家 trace 覆盖 `/sk/speed` 的档线增长指标。

### 2. check-room / query 的周回指标必须按当前玩家 trace 计算

计算流程建议：

1. 先批量解析目标 rank 当前快照，得到当前 `userId`、`score`、`timestamp`。
2. 对每个当前 `userId` 获取最近 trace 样本。
3. 样本窗口以当前快照时间为锚：
   - `end_time = current.timestamp`
   - `start_time = end_time - lookback`
   - lookback 可以是 6 到 12 小时，必须足以覆盖近 1 小时、近 20 分钟和最近若干局。
4. 样本按 timestamp 升序输入指标计算。
5. 只用该玩家自己的点，不混入同 rank 的其他玩家点。

高频玩家 trace 可能远超 5000 条。内部周回计算不能使用"从活动开始按 timestamp 升序 limit 5000"的页面 trace 查询；那会截掉最新窗口，把仍在周回的玩家算成 0。

### 3. 周回字段要么成组可信，要么不要返回

Cloud 当前会把 Tracker 返回的周回字段视为权威结果。若 Tracker 返回了 `recordStartAt`、`latestPt`、`min20Times3Speed` 等字段，Cloud 可能不会再触发本地 fallback。

因此 Tracker 侧不要返回"半成品周回字段"。例如：

- `latestPt > 0` 但 `speed = 0`、`min20Times3Speed = 0`、`hourRound = 0`，同时玩家 trace 最新窗口实际有增长，这是错误结果。
- 如果 trace 不足以计算近 1 小时或近 20 分钟，应该省略相关字段，或通过明确字段表示不可计算，而不是返回误导性 0。
- 真正停火时可以返回 0，但必须由当前玩家最近窗口样本支持。

### 4. `/sk/speed` 继续使用档线 growth 语义

`/sk/speed` 应继续从 rank snapshot / score growth 计算。它表达的是榜线增长，不是玩家周回。

要求：

- 尊重请求的 `interval`。
- 尊重请求的 `unitSeconds`。
- `speedWindow` 应反映实际窗口秒数。
- 对 World Bloom scope 必须使用角色榜表和角色榜 cache key，不与 total 混用。

### 5. user query 也需要当前玩家语义

当 Cloud 用 `userId` 调 `sk/query` 或 `sk/check-room` 时，Tracker 需要：

1. 解析该用户当前快照。
2. 返回该用户当前 rank。
3. 若 include adjacent，返回相邻 rank latest 快照。
4. 周回字段仍按该用户自己的 trace 计算。

### 6. adjacent 只需要 latest 快照

Cloud 不需要 `previous` / `next` 带完整周回指标。相邻项用于定位和展示档位关系，返回 latest rank、name、score、timestamp 即可。

### 7. cache key 必须包含语义维度

Tracker 的 cache key 至少需要区分：

- server
- event_id
- scope：`total` 或 `world-bloom/{character_id}`
- endpoint：query / check-room / line / speed / trace / status
- ranks 或 userId
- interval / unitSeconds
- includeAdjacent / skipMissing
- 周回指标窗口策略版本

当周回计算算法或窗口策略变更时，建议显式 bump cache key suffix，避免旧错误结果继续被 Cloud 命中。

### 8. `/sk/line` 不需要周回指标

`/sk/line` 的用途是展示榜线快照，Cloud 在渲染时会清空 `name` 字段。Tracker 应返回 `rank`、`score`、`timestamp`，可以附带 `speed`（档线增长语义），但不需要完整周回指标组。

### 9. `/sk/status` 固定使用 total scope

Tracker 心跳与 leaderboard scope 无关。`/sk/status` endpoint 的路径固定使用 `total` scope，不需要支持 `world-bloom/{cid}` scope。

## Cloud 侧周回指标 fallback 行为

当 Tracker v2 的 query / check-room 没有返回完整周回指标组时，Cloud 有本地 fallback 逻辑：

### enrichRankInfoFromCloudV2Trace

位于 `internal/pjsk/render/sk/controller_tracker_v2.go`，流程：

1. 检查 `CloudRankInfo` 是否已有完整周回指标（`hasRankInfoRoundMetrics`）。
2. 若已有，跳过 fallback。
3. 若缺失且 `CloudRankInfo.UserID` 存在，以 `subjectType=user` 调 `/sk/trace` 计算本地 metrics。
4. 若 user trace fallback 也未补齐指标，以 `subjectType=rank` 调 `/sk/trace` 再尝试。
5. 本地 metrics 计算逻辑（`applyRankInfoMetrics`）：
   - 取 trace 样本，按 timestamp 升序排列。
   - `recordStartAt` = 当前玩家最后一次有效停车后恢复周回的时间；若没有识别到停车恢复，则使用可见 trace 的首个点。
   - `latestPt` = 最后一个正向增量。
   - `averageRound` / `averagePt` = 最近 10 个正向增量的局数和平均值。
   - `speed` = 近 1 小时窗口内归一到每小时的分数增长。
   - `hourRound` = 近 1 小时窗口内正向增量次数。
   - `min20Times3Speed` = 近 20 分钟窗口内分数增长乘 3。

### fallback 的限制

- `/sk/trace` 的 `limit` 参数 Cloud 默认不传。CSB/player trace 场景需要完整轨迹，不能默认截断为 5000 条；只有调用方明确需要分页或采样时才传 `limit`。
- fallback 只在 Tracker 未返回周回指标时触发。Tracker 返回了完整指标组（即使部分为 0）时 Cloud 不再 fallback。

长期目标是 Tracker 的 v2 cloud API 直接返回正确的语义结果，Cloud 的 fallback 仅作为保护层。

## 已知风险

### query/check-room 与 speed 数值不同不一定是 bug

如果 check-room 的 `speed` 是当前玩家近 1 小时速度，而 `/sk/speed` 的 `speed` 是 rank 档线增长，它们可以不同。

真正的问题是：

- check-room 显示玩家仍在周回却 `speed = 0`
- 或 query/check-room 的玩家周回字段来自 rank trace，混入其他玩家历史点
- 或 Tracker 返回半成品周回字段，让 Cloud 无法 fallback

### World Bloom rank trace 不能代表玩家 trace

World Bloom 角色榜换人频繁，`subjectType=rank` trace 是档线历史，不是当前玩家历史。CSB 和 check-room 都不应把 rank trace 当玩家 trace。

### v1 的 TraceRankingsByRanks 语义陷阱

v1 中 Cloud 用 `TraceRankingsByRanks` 批量获取 trace 并拼装 line 数据。这个接口返回的是 rank trace（档线历史点），对 normal event 大部分时间同一玩家占位所以看起来像 user trace，但对 World Bloom 角色榜会混入不同玩家点。

v2 的 `/sk/line` endpoint 直接返回语义化的最新快照列表，消除了这个陷阱。但如果 Tracker 在 `/sk/line` 内部仍使用 rank trace 计算周回指标，就会重蹈 v1 的覆辙。

### Cloud 对旧 v1 接口的兼容代码残留

当前 HEAD 的 `types_tracker.go` 中保留了 `LatestRankingResponse`、`TraceRankingResponse`、`ScoreGrowthPoint` 等旧类型，但注释为 "Legacy raw tracker shapes kept for older SK controller test fixtures"。生产路径已不再使用这些接口。Tracker 不需要继续维护这些旧端点。

## 建议测试用例

Tracker 侧应覆盖：

- 当前玩家 trace 超过 5000 条时，query/check-room 仍使用最新窗口计算非 0 speed。
- 高频玩家在最近 20 分钟仍有增长时，`min20Times3Speed` 不应被旧窗口算成 0。
- World Bloom rank 换人时，check-room 使用当前 user trace，不混入旧 rank owner。
- `/sk/speed` 与 `/sk/check-room` 在同 rank 下可返回不同 speed，并且测试名称明确区分"line speed"和"player round speed"。
- trace 不足时不返回半成品周回字段，或返回可判定的不可计算状态。
- cache key 在窗口策略变更后不会复用旧错误缓存。
- `/sk/line` 多 rank 请求返回正确的最新快照（不含过时 trace 点）。
- `/sk/line` scope=world-bloom 使用角色榜数据，不与 total 混用。
- `/sk/status` 固定 scope=total，返回正确的 Tracker 心跳状态。

Cloud 侧应覆盖：

- check-room 多 rank 消费 Tracker 返回的完整玩家周回字段。
- check-room 对相邻项不要求 trace enrich。
- Tracker 周回字段缺失时，Cloud fallback 仍能补齐。
- CSB 继续使用当前 player trace，而不是 rank trace。
- line 渲染清空 name 字段，不依赖 trace 拼装。
- speed endpoint `unitSeconds=3600` 和 `unitSeconds=86400` 分别正确生成时速和日速 payload。
