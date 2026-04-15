# Haruki-ZeroBot 与 Haruki-Cloud 联调方案

> 最后更新：2026-03-26（v2.0 — POST + Noise IK + MsgPack 协议）
>
> 本文档描述的是当前目标联调协议。v1.0 中描述的 GET + Header + Base64 协议已废弃。

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
3. 云端不再把"应该去哪个端点"这个问题重新交给一个全局路由器处理。
4. 云端只校验"客户端上传的 `matched_command` 是否属于当前端点"。
5. 如果 `matched_command` 与端点不一致，云端返回错误，而不是静默改路由。

## 3. 协议总览

### 3.1 传输层架构

```
┌───────────────────────────────────────────────┐
│  JWT Session Auth (Header 层)                 │
│  X-Haruki-Bot-Id + X-Haruki-Bot-Session-Token │
├───────────────────────────────────────────────┤
│  Noise IK Transport Encryption (Body 层)      │
│  Noise_IK_25519_AESGCM_SHA256                │
│  每次请求一次完整握手（无状态）               │
├───────────────────────────────────────────────┤
│  MsgPack Encoding (Payload 层)                │
│  请求 = MsgPack(BotCommandRequest)            │
│  响应 = MsgPack(HarukiAPIDataResponse)        │
└───────────────────────────────────────────────┘
```

**分层职责**：

| 层级 | 职责 | 说明 |
|------|------|------|
| JWT Session | 身份认证 | Header 明文传输，`VerifyBotSession` 中间件校验 |
| Noise IK | 传输加密 | Body 加密，防窃听/篡改，per-request 握手 |
| MsgPack | 编码格式 | 二进制编码，Noise 信封内部使用 |

### 3.2 端点一览

| 端点 | 方法 | 认证 | 加密 | 编码 |
|------|------|------|------|------|
| `POST /bot/send-mail` | POST | 无 | 无 | JSON |
| `POST /bot/register` | POST | 无 | 无 | JSON |
| `POST /bot/:bot_id/auth` | POST | AES-256-GCM | AES | JSON |
| `GET /api/v2/bot/:botId/command/manifests` | GET | JWT | 无 | JSON |
| `POST /api/v2/bot/:botId/pjsk/<path>` | POST | JWT | Noise IK | MsgPack |

## 4. 客户端必须对接的接口

### 4.1 Bot 鉴权流程

客户端联调前提仍然是以下接口可用：

1. `POST /bot/send-mail` — 发送验证邮件
2. `POST /bot/register` — 注册 Bot 账号
3. `POST /bot/:bot_id/auth` — 认证并获取 JWT session token

鉴权完成后客户端获得：
- `bot_id`：Bot 的唯一标识
- `session_token`：JWT 格式，含 `bot_id` claim + 过期时间

### 4.2 Manifest 接口

客户端启动后必须先请求：

```http
GET /api/v2/bot/:botId/command/manifests
X-Haruki-Bot-Id: <bot_id>
X-Haruki-Bot-Session-Token: <session_token>
```

响应（JSON）：

```json
{
  "status": 200,
  "message": "ok",
  "data": {
    "entries": [
      {
        "command_prefixes": ["/卡面", "/card"],
        "command_priority": 100,
        "command_mode": "POST",
        "command_module": "pjsk",
        "command_path": "card/detail"
      }
    ]
  }
}
```

> **注意**：Manifest 端点 **不在** Noise IK 保护范围内，始终返回 JSON。

### 4.3 Bot 业务端点

客户端命中后请求：

```http
POST /api/v2/bot/:botId/pjsk/<path>
X-Haruki-Bot-Id: <bot_id>
X-Haruki-Bot-Session-Token: <session_token>
Content-Type: application/octet-stream

Body: <Noise IK Message 1>
```

其中 `<Noise IK Message 1>` 内部携带 MsgPack 编码的 `BotCommandRequest`。

响应：

```
Content-Type: application/octet-stream
Body: <Noise IK Message 2>
```

其中 `<Noise IK Message 2>` 内部携带 MsgPack 编码的响应信封。

## 5. Manifest 字段语义

| 字段 | 语义 |
|------|------|
| `command_prefixes` | 命中同一 Bot 端点的前缀集合 |
| `command_priority` | 前缀树冲突时的优先级 |
| `command_mode` | 该端点允许的 HTTP 方法；当前固定为 `POST` |
| `command_module` | 顶层业务模块，例如 `pjsk`、`chunithm` |
| `command_path` | 客户端命中后要请求的端点路径，例如 `card/detail` |
| `command_additional_params` | 保留字段；当前 PJSK 标准协议为空 |

需要特别强调：

1. Bot 端点总路径形状是 `/api/v2/bot/:botId/<module>/<path>`。
2. 对当前 PJSK 指令来说，`command_module` 固定为 `pjsk`，因此实际路径是 `/api/v2/bot/:botId/pjsk/<path>`。
3. `command_path` 是模块内相对路径，不是云端内部 render target。
4. `command_mode` 固定为 `POST`。

## 6. 客户端命中规则

客户端应按 manifest 构建本地前缀树，并执行：

1. 按 `command_prefixes` 建树。
2. 冲突时按 `command_priority` 决策。
3. 优先级相同时，更长的有效前缀优先。
4. 命中结果要同时产出 `command_path` 和 `matched_command`。
5. 命中结果只决定"请求哪个候选端点"。

客户端不应：

1. 自己完成最终业务语义解释。
2. 先在本地把命令重写成另一种内部语义再发给云端。
3. 假定云端会无条件接受客户端命中的路径。

## 7. BotCommandRequest 结构

所有参数统一通过 POST body 传递（不再使用 Header 或 Query 参数）：

```
BotCommandRequest {
    platform          string            // 必填。来源平台，如 "qq"、"discord"、"kook"
    platform_user_id  string            // 必填。该平台下的用户 ID
    platform_group_id string            // 可选。群组 ID，私聊场景留空
    server            string            // 可选。显式区服覆盖，如 "jp"、"en"、"tw"、"kr"、"cn"
    matched_command   string            // 必填。客户端前缀树实际命中的那条命令
    message           []Segment         // 必填。OneBot V11 消息段数组
}

Segment {
    type  string                        // 段类型："text"、"image"、"at" 等
    data  map<string, string>           // 段数据
}
```

### 7.1 JSON 编码示例（明文模式 / 开发调试）

```json
{
  "platform": "qq",
  "platform_user_id": "12345",
  "platform_group_id": "67890",
  "server": "jp",
  "matched_command": "/卡面",
  "message": [
    {"type": "text", "data": {"text": "/卡面 1001"}}
  ]
}
```

### 7.2 MsgPack 编码说明（Noise 模式）

MsgPack 字段名与 JSON 字段名完全一致（结构体标注了 `msgpack:"..."` 标签）。客户端应使用对应语言的 MsgPack 库进行编码。

## 8. 响应格式

### 8.1 成功响应

```json
{
  "status": 200,
  "message": "ok",
  "data": [
    {"type": "image", "data": {"file": "https://image-cache.haruki.example.com/pjsk/abc123.png"}},
    {"type": "text", "data": {"text": "附加说明文字"}}
  ]
}
```

`data` 字段是 `onebot11.Message`（`[]Segment`），客户端应直接转发为平台消息。

### 8.2 错误响应

```json
{
  "status": 400,
  "message": "command does not match this endpoint",
  "data": {
    "error": "matched_command belongs to path card/list",
    "expected_path": "card/detail",
    "matched_command": "/查卡"
  }
}
```

> **Noise 模式下**：错误响应同样是 MsgPack 编码且 Noise 加密的。客户端必须先解密再解码。

## 9. Noise IK 客户端实现指南

### 9.1 前置条件

客户端需要：

1. **服务端静态公钥**：部署时由运维提供（hex 编码的 32 字节 X25519 公钥）
2. **客户端静态密钥对**：客户端启动时生成一次 X25519 密钥对（重启可重新生成）
3. **Noise 协议库**：支持 `Noise_IK_25519_AESGCM_SHA256` 模式
4. **MsgPack 库**：用于请求/响应编码

### 9.2 密钥管理

```
服务端配置 (haruki-cloud.yaml):
  haruki_bot:
    noise_private_key: "<64 hex chars = 32 bytes>"

服务端公钥（客户端配置）:
  从私钥推导的 X25519 公钥，hex 编码
  服务端启动时日志会输出: "Noise IK transport encryption enabled (pubkey=<hex>)"
```

### 9.3 每次请求的握手流程

Noise IK 是 **per-request** 无状态模式。每次 HTTP 请求都执行一次完整 IK 握手。

```
IK 模式（2 条消息）：
  <- s                              (预共享：客户端已知服务端静态公钥)
  ...
  -> e, es, s, ss, payload          (Message 1: 客户端 → 服务端)
  <- e, ee, se, payload             (Message 2: 服务端 → 客户端)
```

### 9.4 客户端发送请求（伪代码）

```python
# 1. 准备请求数据
request = BotCommandRequest(
    platform="qq",
    platform_user_id="12345",
    matched_command="/卡面",
    message=[Segment(type="text", data={"text": "/卡面 1001"})]
)
plaintext = msgpack_encode(request)

# 2. 初始化 Noise IK 握手（客户端是 initiator）
handshake = NoiseHandshake(
    pattern=IK,
    cipher_suite=Noise_IK_25519_AESGCM_SHA256,
    initiator=True,
    static_keypair=client_static_keypair,
    remote_static=server_public_key          # 预共享的服务端公钥
)

# 3. 写 Message 1（包含加密后的请求体）
ciphertext = handshake.write_message(plaintext)

# 4. 发送 HTTP 请求
response = http_post(
    url=f"/api/v2/bot/{bot_id}/pjsk/{command_path}",
    headers={
        "X-Haruki-Bot-Id": bot_id,
        "X-Haruki-Bot-Session-Token": session_token,
        "Content-Type": "application/octet-stream"
    },
    body=ciphertext
)

# 5. 读 Message 2（解密响应）
decrypted = handshake.read_message(response.body)

# 6. MsgPack 解码响应
result = msgpack_decode(decrypted)
# result = {"status": 200, "message": "ok", "data": [...]}
```

### 9.5 常见 Noise 库推荐

| 语言 | 库 | 备注 |
|------|-----|------|
| Go | `github.com/flynn/noise` | Haruki-Cloud 服务端使用 |
| Python | `noise` (pypi: noiseprotocol) | `from noise.connection import NoiseConnection` |
| Rust | `snow` | `snow::Builder::new("Noise_IK_25519_AESGCM_SHA256".parse()?)` |
| Node.js | `noise-protocol` | npm package |
| Kotlin/JVM | `com.southernstorm:noise-java` | Android / JVM |

### 9.6 降级模式（开发/调试用）

当服务端 **未配置** `noise_private_key`（值为空）时，Noise 中间件不注册。此时客户端应使用明文 JSON 模式：

```http
POST /api/v2/bot/:botId/pjsk/card/detail
X-Haruki-Bot-Id: <bot_id>
X-Haruki-Bot-Session-Token: <session_token>
Content-Type: application/json

{
  "platform": "qq",
  "platform_user_id": "12345",
  "matched_command": "/卡面",
  "message": [{"type": "text", "data": {"text": "/卡面 1001"}}]
}
```

响应也是明文 JSON。

**客户端建议**：优先尝试 Noise 模式，若连接失败（解密错误 / 400），可自动降级为 JSON 模式。或通过配置开关切换。

## 10. 云端收到请求后的处理流程

```
Client                                         Server
  │                                              │
  │── POST /pjsk/<path> ─────────────────────────>│
  │   Header: Bot-Id + Session-Token              │
  │   Body: Noise Message 1                       │
  │                                              │
  │                         VerifyBotSession ────>│ (JWT + Redis 校验)
  │                         Noise Decrypt ───────>│ (Message 1 → MsgPack plaintext)
  │                         MsgPack Decode ──────>│ (BotCommandRequest)
  │                         校验 matched_command ─>│ (是否属于当前 path)
  │                         Handler 解析原文 ────>│ (提取参数、构造 ResolvedCommand)
  │                         Execute ─────────────>│ (bridge → render/数据查询)
  │                         MsgPack Encode ──────>│ (HarukiAPIDataResponse)
  │                         Noise Encrypt ───────>│ (Message 2)
  │                                              │
  │<──── 200 + Noise Message 2 ──────────────────│
```

## 11. 错误语义

### 11.1 401 / 403

JWT 相关错误（Noise 解密之前发生，明文 JSON 响应）：

1. `X-Haruki-Bot-Id` 或 `X-Haruki-Bot-Session-Token` 缺失
2. Bot ID 与 URL 参数不一致
3. JWT 签名无效或过期
4. Session 在 Redis 中已被吊销

### 11.2 400（Noise 解密前）

Noise 握手失败时返回明文 JSON：

1. 请求 body 为空
2. Noise 解密失败（密钥不匹配、数据损坏）

### 11.3 400（Noise 解密后，加密响应）

业务层错误，响应为 Noise 加密 + MsgPack：

1. MsgPack 解码失败
2. `message` 为空
3. `matched_command` 缺失
4. `matched_command` 不属于当前端点
5. 原文无法被当前 handler 解析

> 联调阶段最重要的是第 4 类，它说明客户端前缀树命中逻辑与 manifest 不一致。

### 11.4 500

渲染或数据访问失败。响应为 Noise 加密 + MsgPack。与前缀树命中问题分开排查。

## 12. 联调顺序

### 阶段 1：鉴权打通

确认以下 4 个值配对正确：

1. `bot_id`
2. `session_token`（JWT，从 `/bot/:bot_id/auth` 获得）
3. `X-Haruki-Bot-Id` header
4. `X-Haruki-Bot-Session-Token` header

### 阶段 2：Manifest 打通

确认：

1. 可以稳定拉取 `GET /api/v2/bot/:botId/command/manifests`
2. 客户端能正确消费 `command_prefixes / command_priority / command_module / command_path / command_mode`
3. `command_mode` 全部为 `POST`

### 阶段 3：明文 JSON 模式联调（建议先做）

跳过 Noise，先用 JSON 明文模式走通全链路：

1. 服务端 `noise_private_key` 留空
2. 客户端发送 `POST + JSON body`
3. 验证命中正确端点能成功返回 JSON 响应
4. 验证错误端点返回 400

### 阶段 4：Noise IK 握手打通

1. 服务端配置 `noise_private_key`
2. 客户端配置服务端公钥
3. 发送一次最简单的 Noise 请求，确认握手成功
4. 确认 MsgPack 编解码一致性

### 阶段 5：Noise 全链路验证

至少验证：

1. 详情类命令（图片响应）
2. 列表类命令（文本响应）
3. 同前缀冲突命令
4. 故意请求错误端点时云端返回加密 400

### 阶段 6：上下文字段联调

逐步验证 `BotCommandRequest` 各字段：

1. `platform` / `platform_user_id` — 影响 ban check 和绑定查询
2. `platform_group_id` — 影响群聊/私聊判断
3. `server` — 影响区服选择
4. `matched_command` — 影响端点命中校验

## 13. 验收标准

本轮联调完成的最低标准是：

1. 客户端可以稳定拉取 manifest。
2. 客户端可以按 manifest 构建前缀树。
3. 客户端命中后可以正确产出 `path + matched_command`。
4. 客户端可以正确发送 Noise IK 加密的 POST 请求。
5. 云端端点能解密、解码、解析参数，并返回 Noise 加密的 OneBot11 消息。
6. 客户端可以正确解密并解码响应。
7. 客户端请求错误端点时，云端能稳定返回加密的 400。
8. 明文 JSON 降级模式可用（开发环境）。

## 14. 当前不接受的客户端做法

以下做法不再作为联调目标：

1. 使用 `GET /get_configs?modules=...`
2. 使用 `/{bot_id}/{module}{commandPath}` 旧调用协议
3. 直接调用 `/internal/pjsk/render`
4. 直接调用 `/internal/pjsk/command`
5. 使用旧 GET + Header + Base64 协议
6. 让客户端承担最终解析职责
7. 假定云端会根据原文自动改派到另一个端点

当前 PJSK 指令联调应以 `POST /api/v2/bot/*` 为唯一业务协议族。

## 15. 相关文档

1. [PJSK 指令系统设计](pjsk-command-system.cn.md)
2. [项目进展总结](project-status-summary.cn.md)
3. [ZeroBot 渲染接入后续事项](zerobot-render-followup.cn.md)
4. [Haruki Toolbox API 客户端文档](toolbox-api.cn.md)
