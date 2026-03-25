# Haruki-ZeroBot 与 Haruki-Cloud 联调方案

> 最后更新：2026-03-25
>
> 本文档描述的是当前目标联调协议。若现有实现仍保留部分旧链路，以本文定义的边界为后续收口目标。

## 1. 联调目标

本轮联调要达成的结果是：

1. `Haruki-Cloud` 下发 manifest。
2. `Haruki-ZeroBot` 基于 manifest 构建本地前缀树。
3. 客户端收到消息后先本地命中 `path`。
4. 客户端按命中的 `path` 请求对应 `/api/v2/bot/*` 端点，并上传 `matched_command`。
5. 云端端点校验 `matched_command` 属于当前 `path` 后，再在 handler 内部解析原始文本、提取参数，并进入统一执行链路返回 OneBot11 消息。

## 2. 协议边界

当前边界明确如下：

1. 客户端负责前缀树命中和候选端点选择。
2. 云端负责端点内部的原文解析和最终业务执行。
3. 云端不再把“应该去哪个端点”这个问题重新交给一个全局路由器处理。
4. 云端只校验“客户端上传的 `matched_command` 是否属于当前端点”。
5. 如果 `matched_command` 与端点不一致，云端返回错误，而不是静默改路由。

## 3. 客户端必须对接的接口

### 3.1 Bot 基础能力

客户端联调前提仍然是以下接口可用：

1. `POST /bot/send-mail`
2. `POST /bot/register`
3. `POST /bot/:bot_id/auth`

### 3.2 Manifest 接口

客户端启动后必须先请求：

`GET /api/v2/bot/:botId/command/manifests`

该接口用于：

1. 获取命令前缀集合
2. 获取优先级
3. 获取命中后的业务路径

### 3.3 Bot 业务端点

客户端命中后必须请求：

`GET /api/v2/bot/:botId/pjsk/<path>?command_payload=<base64(ob11 pack)>`

例如：

1. `/api/v2/bot/:botId/pjsk/card/detail`
2. `/api/v2/bot/:botId/pjsk/card/list`
3. `/api/v2/bot/:botId/pjsk/event`
4. `/api/v2/bot/:botId/pjsk/music/chart`

这里的 `<path>` 由 manifest 的 `command_path` 决定。

## 4. Manifest 字段语义

`GET /api/v2/bot/:botId/command/manifests` 返回的每条记录当前应按下面的语义理解：

| 字段 | 当前语义 |
|------|---------|
| `command_prefixes` | 命中同一 Bot 端点的前缀集合 |
| `command_priority` | 前缀树冲突时的优先级 |
| `command_mode` | 该端点允许的 HTTP 方法；当前 PJSK Bot 协议使用 `GET` |
| `command_module` | 顶层业务模块，例如 `pjsk`、`chunithm` |
| `command_path` | 客户端命中后要请求的端点路径，例如 `card/detail` |
| `command_additional_params` | 端点额外接受的查询参数名；当前 PJSK 标准协议通常为空 |

需要特别强调：

1. Bot 端点总路径形状是 `/api/v2/bot/:botId/<module>/<path>`。
2. 对当前 PJSK 指令来说，`command_module` 固定为 `pjsk`，因此实际路径是 `/api/v2/bot/:botId/pjsk/<path>`。
3. `command_path` 是模块内相对路径，不是云端内部 render target。
4. `command_mode` 是 HTTP 方法，不是解析器内部语义 mode。

## 5. 客户端命中规则

客户端应按 manifest 构建本地前缀树，并执行：

1. 按 `command_prefixes` 建树。
2. 冲突时按 `command_priority` 决策。
3. 优先级相同时，更长的有效前缀优先。
4. 命中结果要同时产出 `command_path` 和 `matched_command`。
5. 命中结果只决定“请求哪个候选端点”。

客户端不应：

1. 自己完成最终业务语义解释。
2. 先在本地把命令重写成另一种内部语义再发给云端。
3. 假定云端会无条件接受客户端命中的路径。

## 6. 命中后的请求格式

### 6.1 标准调用方式

标准调用方式为：

```http
GET /api/v2/bot/:botId/pjsk/card/detail?command_payload=<base64(ob11 pack)>
X-Haruki-Bot-Platform: qq
X-Haruki-Bot-Platform-User-Id: 12345
X-Haruki-Bot-Platform-Group-Id: 67890
X-Haruki-Bot-Pjsk-Server: jp
X-Haruki-Bot-Matched-Command: /查卡
```

字段说明：

1. `command_payload` 是客户端从 OneBot V11 拿到的消息原文包，经 Base64 后作为查询参数上传。
2. `X-Haruki-Bot-Matched-Command` 必须传客户端前缀树实际命中的那条命令。
3. `X-Haruki-Bot-Pjsk-Server` 用于显式覆盖区服。
4. `X-Haruki-Bot-Platform-Group-Id` 在私聊场景可为空。
5. 当前协议只消费这里列出的查询参数和请求头，不包含 `group_name`、`username`、`sender_name` 这类扩展字段。

### 6.2 `server` 字段

如果客户端本地已经识别出区服，可以同时传 `X-Haruki-Bot-Pjsk-Server`。

要求是：

1. `server` 必须与原始文本含义一致。
2. 不要传与原文冲突的区服。

## 7. 云端收到请求后的目标行为

对于任意一个 Bot 业务端点，目标行为应当是：

1. 校验 `VerifyBotSession`
2. 从 `command_payload` 恢复原始 `command`
3. 校验 `matched_command` 是否属于当前端点
4. 用命中的 handler 在当前端点语义范围内解析原文
5. 提取当前端点需要的业务参数
6. 若 `matched_command` 不属于当前端点，返回 `400`
7. 若解析失败，返回 `400`
8. 若解析成立，调用 `commandhandler.Execute(...)`
9. 返回 JSON 包装的 `onebot11.Message`（图片为 `image` segment，文本为 `text` segment）

以 `card/detail` 为例：

1. 客户端命中 `/api/v2/bot/:botId/pjsk/card/detail`
2. 客户端上传 `command_payload` 和 `X-Haruki-Bot-Matched-Command`
3. 云端先检查这个 `matched_command` 是否属于 `card/detail`
4. 命中对应 handler 后再继续解析原文
5. 若当前 handler 无法继续处理，则直接报错
6. 若解析成立，则继续执行

## 8. 错误语义

### 8.1 401 / 403

表示：

1. session token 无效
2. Bot ID 与 token 不一致
3. 请求头不符合 `VerifyBotSession`

### 8.2 400

表示：

1. `command_payload` 缺失
2. `matched_command` 缺失
3. `matched_command` 不属于当前端点
4. 原文无法被当前 handler 解析

联调阶段最重要的是第 3 类，它说明：

1. 客户端前缀树命中逻辑与 manifest 不一致
2. 客户端对原始命令做了不该做的重写
3. 云端端点边界和客户端理解不一致

### 8.3 500

表示：

1. 云端后续渲染或数据访问失败
2. 这类问题应和前缀树命中问题分开排查

## 9. 当前不接受的客户端做法

以下做法不再作为联调目标：

1. 使用 `GET /get_configs?modules=...`
2. 使用 `/{bot_id}/{module}{commandPath}` 这种旧普通业务调用协议
3. 直接调用 `/internal/pjsk/render`
4. 直接调用 `/internal/pjsk/command`
5. 让客户端承担最终解析职责
6. 假定云端会根据原文自动改派到另一个端点

当前 PJSK 指令联调应以 `/api/v2/bot/*` 为唯一业务协议族。

## 10. 建议联调顺序

### 阶段 1：鉴权打通

确认：

1. `bot_id`
2. `session_token`
3. `X-Haruki-Bot-Id`
4. `X-Haruki-Bot-Session-Token`

### 阶段 2：Manifest 打通

确认：

1. 可以稳定拉取 `GET /api/v2/bot/:botId/command/manifests`
2. 客户端能正确消费 `command_prefixes / command_priority / command_module / command_path / command_mode`

### 阶段 3：前缀树命中打通

至少验证：

1. 详情类命令
2. 列表类命令
3. 同前缀冲突命令

### 阶段 4：端点内解析校验

至少验证：

1. 命中正确端点时能成功返回 OneBot11 图片/文本消息
2. 故意请求错误端点时云端返回 `400`

### 阶段 5：上下文字段联调

逐步验证：

1. `X-Haruki-Bot-Platform`
2. `X-Haruki-Bot-Platform-User-Id`
3. `X-Haruki-Bot-Platform-Group-Id`
4. `X-Haruki-Bot-Pjsk-Server`
5. `X-Haruki-Bot-Matched-Command`

## 11. 验收标准

本轮联调完成的最低标准是：

1. 客户端可以稳定拉取 manifest。
2. 客户端可以按 manifest 构建前缀树。
3. 客户端命中后可以正确产出 `path + matched_command`。
4. 客户端可以正确请求 `/api/v2/bot/:botId/pjsk/*`。
5. 云端端点能按原文在本端点内解析参数，并返回 OneBot11 图片/文本消息。
6. 客户端请求错误端点时，云端能稳定返回 `400`。

## 12. 相关文档

1. [PJSK 指令系统设计](pjsk-command-system.cn.md)
2. [项目进展总结](project-status-summary.cn.md)
3. [ZeroBot 渲染接入后续事项](zerobot-render-followup.cn.md)
