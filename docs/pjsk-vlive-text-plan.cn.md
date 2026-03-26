# PJSK Virtual Live 文本版实现方案

> 最后更新：2026-03-26

## 1. 目标

本轮只实现 `Virtual Live` 的最小文本查询链路，不做图片，不做提醒，不做奖励明细，不做任何 `Misc` 历史 stub。

目标效果：

- 客户端命中 `/pjsk live`、`/pjsk vlive`、`/vlive`、`/虚拟live`
- 请求 `GET /api/v2/bot/:botId/pjsk/vlive`
- 云端返回 `onebot11.Message{text}` 文本结果

## 2. 范围

### 本轮实现

- 新增 bot path：`vlive`
- 新增 `Virtual Live` 模块执行链路
- 从 `sekai.virtuallives` 查询指定区服数据
- 按 refer 约定迁移最近活动过滤规则
- 纯文本格式化输出
- 无活动时返回固定文本：`当前没有虚拟Live`

### 本轮不实现

- 图片渲染
- 自动提醒 / 订阅
- 奖励箱解析
- Banner / 角色图 / 素材图展示
- `Misc` 下的 `Help`、`Update`、`NgWord`、`UploadHelp`、`ExtractCard`

## 3. 路由与命令

### Handler 命令

- `/pjsk live`
- `/虚拟live`
- `/pjsk vlive`
- `/vlive`

### Bot Path

- `vlive`

即客户端调用：

```text
GET /api/v2/bot/:botId/pjsk/vlive?command_payload=...
```

## 4. 数据来源

直接使用 `Haruki-Cloud` 已存在的 `sekai` 数据库表：

- `virtuallives`

本轮使用的字段：

- `server_region`
- `game_id`
- `name`
- `start_at`
- `end_at`
- `virtual_live_schedules`

可选使用但本轮不强依赖：

- `assetbundle_name`
- `virtual_live_rewards`
- `virtual_live_information`

## 5. 过滤规则

与 `refer/vlive.py` 对齐的最小规则：

1. 当前时间 `now < endAt`
2. `startAt - now < 7天`
3. `endAt - startAt < 30天`

按 `startAt` 升序输出。

## 6. 文本状态推导

若 `virtual_live_schedules` 可解析，则按 schedule 推导：

- `current`: 第一条满足 `now < schedule.endAt` 的场次
- `living`: `current.startAt <= now < current.endAt`
- `rest_num`: 满足 `now < schedule.startAt` 的剩余场次数

输出语义：

- `living=true`：`当前Live进行中`
- `living=false` 且存在 `current`：`下一场: YYYY-MM-DD HH:MM`
- 否则：`已结束`

若 `virtual_live_schedules` 不可解析，则降级为：

- 若 `now < startAt`：`下一场: startAt`
- 否则若 `now < endAt`：`当前Live进行中`
- 否则：`已结束`

## 7. 文本输出格式

建议格式：

```text
JP 虚拟Live列表

【1001】Virtual Live Name
开始: 2026-03-27 19:00
结束: 2026-03-28 21:00
状态: 下一场: 2026-03-27 20:00 | 剩余场次: 3

【1002】Another Live
开始: 2026-03-29 18:00
结束: 2026-03-29 20:00
状态: 当前Live进行中 | 剩余场次: 0
```

说明：

- 第一行带区服
- 各活动之间空一行
- 暂不输出奖励、角色、会场等扩展信息

## 8. 代码落点

### 8.1 Parser

- `internal/pjsk/parser/global_resolver.go`

新增：

- `ModuleVLive`
- `/pjsk live` / `/pjsk vlive` / `/vlive` / `/虚拟live` 的 regex 路由

### 8.2 Handler

- `internal/pjsk/handler/sekai/vlive.go`

调整为：

- 去掉 `Disabled: true`
- 增加 `Path: "vlive"`
- 只返回 `makeResolvedCmd(ctx, parser.ModuleVLive, "vlive-list")`

### 8.3 Render / Query

新增目录：

- `internal/pjsk/render/vlive/`

建议文件：

- `controller.go`
- `query.go`
- `source_cloud.go`

职责：

- `source_cloud.go`：从 `sekaiDB.Client` 查询 `virtuallives`
- `controller.go`：过滤、排序、解析 schedule、格式化文本

### 8.4 App

- `internal/pjsk/render/app/app.go`

新增：

- `App.VLive *vlive.Controller`

仅在 `sekaiClient != nil` 时初始化。

### 8.5 Bridge

- `internal/pjsk/handler/bridge.go`

新增：

- `case parser.ModuleVLive`
- `executeVLive(...)`

输出：

- `onebot11.Message{onebot11.Text(text)}`

## 9. 测试

本轮至少补三类测试：

1. handler test
   - 命令是否生成 `ResolvedCommand`
   - path 是否为 `vlive`

2. controller test
   - 7 天过滤
   - 已结束过滤
   - 30 天以上过滤
   - schedule 解析和状态文本
   - 空结果时返回 `当前没有虚拟Live`

3. bridge / execute test
   - 命中 `vlive-list` 后返回单条文本消息

## 10. 验收标准

满足以下条件即视为完成：

1. `/api/v2/bot/:botId/pjsk/vlive` 已进入 bot route / manifest
2. 命令可通过 handler 注册并解析
3. 可从 DB 查询并过滤近期 `Virtual Live`
4. 返回纯文本而不是 TODO
5. 无活动时返回 `当前没有虚拟Live`
6. 定向测试通过
