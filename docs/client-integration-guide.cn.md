# Haruki-Cloud Bot 客户端对接指南

本文档描述 Bot 客户端如何接入 Haruki-Cloud 后端 API，涵盖认证流程、指令分发、请求/响应格式和 Noise IK 加密传输。

## 1. 整体架构

```
Bot Client (QQ/Discord/...)
    │
    ├── 1. POST /bot/:bot_id/auth          → 获取 session_token
    ├── 2. GET  /api/v2/bot/:botId/command/manifests  → 获取指令清单
    └── 3. POST /api/v2/bot/:botId/pjsk/:path         → 执行指令
                      │
               [Noise IK 加密层 — 可选]
```

**基础 URL**：`http://<server>:<port>`（Alpha 环境：`http://INTERNAL_IP_1:6667`）

---

## 2. 凭证说明

客户端需持有以下凭证（由注册流程或管理员生成，存储在 `client.json`）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `bot_id` | int | 8 位数字 Bot ID |
| `credential` | string | Base64 编码的 JWT，由 `credential_sign_token` 签名，包含 `bot_id` 和原始 credential |
| `noise_server_pubkey` | string | 服务端 Noise IK X25519 公钥（hex），用于加密传输层（可选） |

> 说明：当前客户端只需要显式配置**服务端公钥**。客户端静态公钥会在 Noise IK 握手中隐式参与，但服务端侧的“客户端公钥登记 / 白名单授权”体系目前仍是后续 TODO，不是当前接入前提。

### 2.1 Credential JWT 结构

credential 是一个 HS256 签名的 JWT，payload 为：

```json
{
  "bot_id": "1000000002",
  "credential": "<原始 credential base64>"
}
```

签名密钥为服务端的 `credential_sign_token`（客户端不需要知道）。

---

## 3. 认证流程

### 3.1 登录获取 Session Token

**端点**：`POST /bot/:bot_id/auth`

**请求体**：

```json
{
  "encrypted_payload": "<base64 encoded AES-256-GCM ciphertext>"
}
```

**加密流程**：

1. 构造明文 JSON payload：
   ```json
   {
     "credential": "<JWT credential 字符串>",
     "timestamp": 1711584000
   }
   ```
   - `timestamp`：当前 Unix 时间戳（秒），与服务端时差不超过 **300 秒**

2. 派生 AES 密钥：取原始 credential（非 JWT，而是数据库中存储的 base64 字符串）的 **前 32 字节**（UTF-8 编码）作为 AES-256 密钥

3. AES-256-GCM 加密：
   - 生成 12 字节随机 nonce
   - 加密 payload JSON
   - 拼接 `nonce || ciphertext || tag`
   - Base64 标准编码输出

**成功响应**（200）：

```json
{
  "status": 200,
  "message": "ok",
  "data": {
    "session_token": "eyJhbGciOiJIUzI1NiIs...",
    "expires_at": 1712188800
  }
}
```

- `session_token`：JWT 格式，有效期默认 7 天
- `expires_at`：过期时间 Unix 时间戳

**错误响应**：

| 状态码 | message | 原因 |
|--------|---------|------|
| 400 | `invalid bot_id` | bot_id 非数字 |
| 400 | `invalid encrypted payload` | 解密失败或格式错误 |
| 400 | `auth request expired` | 时间戳偏差 > 300s |
| 400 | `invalid credential` | JWT 验证失败 |
| 400 | `bot_id mismatch` | JWT 中 bot_id 与 URL 不匹配 |
| 400 | `authentication failed` | credential 不匹配 |

### 3.2 AES-256-GCM 加密伪代码

```python
import json, os, base64, time
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

# 原始 credential（base64 字符串，非 JWT）
raw_credential = "BGdlRJ+nhaW6AZK3Fsk1udewt/YbjlGWqgoirM28cm0="
jwt_credential = "<JWT signed credential>"

# 1. 派生 AES key：取 raw_credential 的 UTF-8 前 32 字节
key = raw_credential.encode('utf-8')[:32]

# 2. 构造 payload
payload = json.dumps({
    "credential": jwt_credential,
    "timestamp": int(time.time())
}).encode('utf-8')

# 3. 加密
nonce = os.urandom(12)
aesgcm = AESGCM(key)
ciphertext = aesgcm.encrypt(nonce, payload, None)

# 4. 编码
encrypted_payload = base64.b64encode(nonce + ciphertext).decode()
```

---

## 4. 指令清单（Command Manifest）

### 4.1 获取指令清单

**端点**：`GET /api/v2/bot/:botId/command/manifests`

**鉴权 Headers**：

| Header | 值 |
|--------|------|
| `X-Haruki-Bot-Id` | Bot ID 字符串（如 `"1000000002"`） |
| `X-Haruki-Bot-Session-Token` | 登录获取的 session_token |

**成功响应**（200）：

```json
{
  "status": 200,
  "message": "ok",
  "data": {
    "entries": [
      {
        "command_prefixes": ["/查卡", "/card", "/cards", "/jpcard", ...],
        "command_priority": 0,
        "command_mode": "POST",
        "command_module": "pjsk",
        "command_path": "card/detail",
        "command_additional_params": []
      }
    ]
  }
}
```

> `entries` 数量随服务端当前注册路由变化（2026-04-10 当前活跃路径为 82 条）。

### 4.2 Manifest 字段说明

| 字段 | 说明 |
|------|------|
| `command_prefixes` | 触发该端点的所有指令前缀列表 |
| `command_priority` | 匹配优先级，数值越高越优先匹配 |
| `command_mode` | HTTP 方法（固定 `"POST"`） |
| `command_module` | 模块名（目前固定 `"pjsk"`） |
| `command_path` | 端点路径，拼接后为 `/api/v2/bot/:botId/pjsk/:path` |
| `command_additional_params` | 额外参数列表（预留） |

### 4.3 客户端指令匹配流程

1. 启动时拉取完整 manifest
2. 按 `command_priority` 降序排列所有 entries
3. 用户发送消息时，遍历所有 `command_prefixes` 找到最长前缀匹配
4. 记录匹配到的 prefix 作为 `matched_command`，向对应 `command_path` 发请求

---

## 5. 执行指令

### 5.1 请求格式

**端点**：`POST /api/v2/bot/:botId/pjsk/:command_path`

**鉴权 Headers**（同 §4.1）：

| Header | 值 |
|--------|------|
| `X-Haruki-Bot-Id` | Bot ID |
| `X-Haruki-Bot-Session-Token` | Session Token |

**请求体**（JSON）：

```json
{
  "platform": "qq",
  "platform_user_id": "QQ_ID_REDACTED",
  "platform_group_id": "12345678",
  "server": "jp",
  "matched_command": "/查卡",
  "message": [
    {"type": "text", "data": {"text": "/查卡 miku"}}
  ]
}
```

### 5.2 请求字段说明

| 字段 | 必填 | 说明 |
|------|------|------|
| `platform` | ✅ | 平台标识：`"qq"`, `"discord"` 等 |
| `platform_user_id` | ✅ | 平台用户 ID |
| `platform_group_id` | ❌ | 群组 ID（私聊时留空） |
| `server` | ❌ | 区服覆盖：`"jp"`, `"en"`, `"tw"`, `"kr"`, `"cn"`。留空则从指令前缀或参数推断 |
| `matched_command` | ✅ | 客户端匹配到的指令前缀（必须在该端点的 `command_prefixes` 内） |
| `message` | ✅ | OneBot v11 消息段数组 |

### 5.3 Message 消息段格式

采用 OneBot v11 消息段协议：

```json
[
  {"type": "text", "data": {"text": "/查卡 miku"}},
  {"type": "image", "data": {"file": "https://example.com/image.png"}},
  {"type": "at", "data": {"qq": "123456"}}
]
```

支持的消息段类型：

| type | data 字段 | 说明 |
|------|-----------|------|
| `text` | `text: string` | 文本消息（包含完整指令文本） |
| `image` | `file: string` | 图片 URL |
| `at` | `qq: string` | @某人 |

**重要**：`message` 中的 text 段需包含**完整的指令文本**（含前缀和参数），服务端会重新解析。

### 5.4 成功响应

```json
{
  "status": 200,
  "message": "ok",
  "data": [
    {"type": "image", "data": {"file": "base64://iVBORw0KGgoAAAA..."}},
    {"type": "text", "data": {"text": "卡面信息: ..."}}
  ]
}
```

`data` 为 OneBot v11 消息段数组（`Message`）：

- **图片**：`type: "image"`，`data.file` 为 `base64://<png data>` 格式
- **文本**：`type: "text"`，`data.text` 为纯文本

### 5.5 错误响应

```json
{
  "status": 500,
  "message": "render failed",
  "data": {
    "error": "具体错误信息",
    "mode": "card-detail"
  }
}
```

| 状态码 | 含义 |
|--------|------|
| 400 | 请求格式错误、缺少必填字段、matched_command 不匹配 |
| 401 | 未认证或 session 过期 |
| 403 | bot_id 不匹配 |
| 500 | 渲染失败（Drawing API 错误、数据库异常等） |

---

## 6. Noise IK 加密传输（可选）

当服务端启用 Noise IK 时，`/api/v2/bot/:botId/pjsk/*` 路径下的所有指令请求**必须**使用 Noise IK 协议加密。Manifest 端点**不受** Noise 保护。

### 6.1 协议说明

- **模式**：Noise IK（客户端已知服务端静态公钥）
- **密钥交换**：X25519
- **AEAD**：AES-GCM
- **哈希**：SHA256

补充说明：

1. 当前 Noise 的主要作用是**传输层加密**。
2. 当前服务端尚未把客户端静态公钥做成正式的设备身份白名单因子。
3. 因此，Bot 身份的正式认证仍然以 `X-Haruki-Bot-Id + X-Haruki-Bot-Session-Token` 为准。

### 6.2 请求流程

1. 客户端使用服务端公钥（`noise_server_pubkey`）构建 Noise IK handshake
2. 将 MsgPack 编码的 `BotCommandRequest` 作为 handshake payload
3. 发送 Noise Message 1 作为 HTTP body
4. 服务端解密后处理指令，将响应以 Noise Message 2（MsgPack）返回

### 6.3 编码格式

- **请求**：Noise IK Message 1 → HTTP body（`Content-Type: application/octet-stream`）
- **响应**：Noise IK Message 2 → HTTP body（`Content-Type: application/msgpack`）

当不使用 Noise IK 时，请求体为标准 JSON，响应也为 JSON。

---

## 7. 区服（Region）处理

### 7.1 区服确定优先级

1. 请求体 `server` 字段（最高优先级）
2. 指令前缀中的区服标识（如 `/jp查卡`、`/cn查曲`）
3. 指令参数中的区服关键词
4. 默认：`jp`

### 7.2 MySekai 国服限制

MySekai 相关指令（`mysekai/*` 路径）在 `region=cn` 时默认关闭。

服务端通过 `allow_cn_mysekai` 配置白名单：仅当请求的 `platform` + `platform_group_id` 匹配白名单条目时才允许执行。未匹配时返回文本消息 `"MySekai 功能暂不支持国服区域"`。

---

## 8. 完整对接流程示例

### 8.1 启动阶段

```
1. 读取 client.json 获取 bot_id、credential、noise_server_pubkey
2. POST /bot/{bot_id}/auth 登录获取 session_token
3. GET /api/v2/bot/{bot_id}/command/manifests 拉取指令清单
4. 按 priority 排序，构建本地指令匹配表
```

### 8.2 消息处理阶段

```
1. 收到用户消息 "/查卡 miku"
2. 遍历匹配表，找到最长匹配 "/查卡" → path: "card/detail"
3. 构造请求:
   POST /api/v2/bot/{bot_id}/pjsk/card/detail
   Headers:
     X-Haruki-Bot-Id: {bot_id}
     X-Haruki-Bot-Session-Token: {session_token}
   Body:
     {
       "platform": "qq",
       "platform_user_id": "QQ_ID_REDACTED",
       "platform_group_id": "12345678",
       "matched_command": "/查卡",
       "message": [{"type":"text","data":{"text":"/查卡 miku"}}]
     }
4. 解析响应中的 message segments，发送给用户
```

### 8.3 Session 续期

- Session 有效期默认 7 天
- 客户端应在 `expires_at` 前重新调用 `/bot/:bot_id/auth` 获取新 token
- 收到 401 响应时也应自动重新认证

### 8.4 单 Session 限制

- 同一 `bot_id` 同一时间只能有 **一个活跃 session**
- 每次调用 `/bot/:bot_id/auth` 登录成功后，之前的 session token 会被覆盖（新 token 替换旧 token）
- 如果客户端 A 和客户端 B 使用同一 `bot_id` 登录，后登录的客户端会导致先登录的客户端 session 失效（收到 401）
- 注销端点 `DELETE /bot/:bot_id/logout` 需要在请求头中携带 `X-Haruki-Bot-Session-Token`，服务端验证通过后才会删除 session

---

## 9. 端点路径速查

以下为当前已注册的主要指令路径（完整列表从 manifest 获取）：

| 路径 | 功能 | 典型指令 |
|------|------|----------|
| `card/detail` | 查卡牌详情 | `/查卡 miku` |
| `card/image` | 查卡面原图 | `/卡面 res001_no001` |
| `card/list` | 卡牌列表 | `/查卡牌 miku` |
| `card/box` | 卡牌一览（箱子） | `/查箱` |
| `deck/event` | 活动组队推荐 | `/组队` |
| `deck/challenge` | 挑战组队 | `/挑战组队` |
| `education/area` | 区域道具升级 | `/区域道具 miku` |
| `education/bonds` | 羁绊查询 | `/羁绊` |
| `education/leader` | 队长统计 | `/队长统计` |
| `education/power` | 综合力查询 | `/综合力` |
| `event` | 活动信息 | `/活动` |
| `event/bonus` | 活动加成 | `/活动加成` |
| `music` | 歌曲查询 | `/查曲 Tell Your World` |
| `music/bpm` | BPM 查询 | `/查BPM 200` |
| `misc/birthday` | 角色生日 | `/生日 miku` |
| `score` | 控分计算 | `/控分 360` |
| `score/custom-room` | 自定义房间控分 | `/自定义房间控分 50` |
| `sk/event-rank` | 活动排名 | `/排名` |
| `sk/player-trace` | 个人追踪 | `/追踪` |
| `sk/border` | 档线查询 | `/档线` |
| `sk/trend` | 趋势图 | `/趋势` |
| `sk/cutoff-detail` | 预测详情 | `/预测详情` |
| `sk/cutoff-compare` | 预测对比 | `/预测对比` |
| `sk/winrate` | 胜率预测 | `/胜率` |
| `profile/info` | 个人信息 | `/个人信息` |
| `profile/bind` | 绑定账号 | `/绑定` |
| `mysekai/resource` | MySekai 资源 | `/msa` |
| `mysekai/map` | MySekai 地图 | `/msm 1` |
| `mysekai/fixture-list` | 家具列表 | `/家具列表` |
| `mysekai/fixture-detail` | 家具详情 | `/家具 xxx` |
| `mysekai/door-upgrade` | 门升级 | `/门升级` |
| `mysekai/music-record` | 音乐记录 | `/msr` |
| `mysekai/talk-list` | 对话列表 | `/对话列表` |
| `mysekai/photo` | 拍照 | `/拍照` |
| `mysekai/blueprint` | 蓝图 | `/蓝图` |
| `stamp` | 表情贴纸 | `/表情` |

MySekai 快捷命令约定（2026-03-28）：

- `msa` -> `mysekai/resource`
- `msm` -> `mysekai/map`
- `msr` -> `mysekai/music-record`
- `/msm <编号>` 支持按顺序编号选图：`1/2/3/4` 分别映射地图 ID `5/7/6/8`（其中 `2` 对应花园）
- `/msm 13` 支持紧凑组合写法（等价于 `1 3`）
- 追加 `all`（如 `/msm 1 all`）会显示已采集内容（`show_harvested=true`）

---

## 10. 通用响应信封

所有 API 响应均遵循统一格式：

```json
{
  "status": <HTTP状态码>,
  "message": "<描述>",
  "data": <具体数据或null>
}
```

当使用 Noise IK 传输时，响应体为 MsgPack 编码的相同结构。
