# Haruki-Cloud 项目架构文档

> 最后更新：2026-08-26（v2.0）
>
> 2026-08-26 补充：
> 1. Go 升级至 1.27；`bytedance/sonic` 已移除，JSON 统一走 `internal/jsonutil`
>    （`encoding/json/v2` 引擎 + v1 兼容语义）。
> 2. Bot 自助注册链路（`/bot/send-mail`、`/bot/register`、SMTP、Turnstile）已
>    移除；Bot 账号由运维通过 `scripts/provision_bot` 手动开通，公开端点仅剩
>    登录/注销。统计上报移至 `/internal/bot/statistics/record/:botID`（内部鉴权）。
> 3. `BotCommandRequest` 新增可选字段 `event_time`/`event_id`（事件时间去重）与
>    `timestamp`/`nonce`（Noise 通道重放保护，默认强制校验；仅
>    `haruki_bot.allow_requests_without_nonce=true` 时放宽）。
> 4. `internal/` 新增 `cache/drawingcache`（绘图缓存）、`cluster`（节点只读模式）、
>    `jsonutil`、`observability/commandtrace`、`core/upstream`；`internal/pjsk/`
>    新增 `filteralias`、`subscription`；render 新增 `costume`、`inventory` 模块。
>
> 2026-04-18 补充：
> 1. `internal/pjsk/handler/sekai/` 子包已扁平化至 `internal/pjsk/handler/`；所有 `bridge_*.go` 分发文件已删除，执行逻辑合并进各命令文件。
> 2. `internal/pjsk/onebot11/` 已上移至 `internal/onebot11/`（通用工具包，不专属 pjsk）。
> 3. `internal/handler/` 新增统一命令注册表（`handler.go` + `bot_route.go`）。
> 4. `internal/pjsk/parser/global_resolver.go` 已删除。
>
> 2026-04-09 补充：
> 1. `api/legacy/pjsk/` 已从仓库与运行时移除。
> 2. PJSK Bot 主协议已经收口到 `POST /api/v2/bot/:botId/pjsk/<path>`。
> 3. `internal/pjsk/render/deck/deck_cgo/` 历史目录已从仓库移除，deck recommend 运行时仅保留 HTTP 外部服务。
> 4. `internal/pjsk/render/snapshot/` 已由 `render/userdata/` 重命名，快照来源统一走 Toolbox（生产不回落本地）。

---

## 1. 项目概览

**Haruki-Cloud** 是 HarukiBot 生态的核心后端服务，负责：

- 为 Bot 提供 **指令解析 → 执行 → 返回 OneBot11 消息** 的完整链路
- 管理 Project SEKAI（プロジェクトセカイ）和 CHUNITHM 两个音游的查询数据
- 提供 Bot 注册/鉴权/会话管理

**技术栈：**

| 组件 | 技术 |
|------|------|
| HTTP 框架 | Fiber v3 |
| ORM | Ent (entgo.io) |
| 数据库 | PostgreSQL / MySQL / SQLite |
| 缓存 | Redis |
| 认证 | JWT (golang-jwt/v5) + Noise NK（AuthV3，无共享密钥） |
| JSON | `encoding/json/v2`（经 `internal/jsonutil` 统一封装，保持 v1 兼容语义） |
| Go 版本 | 1.27 |

---

## 2. 目录结构总览

```
Haruki-Cloud/
├── main.go                       # ── 主服务入口（唯一运行的进程）──
│
├── cmd/                          # ── 一次性 CLI 工具 ──
│   ├── trust-signer/             #   离线 Ed25519 签名工具（keyset / manifest）
│   ├── importer/main.go          #   旧数据迁移工具（历史数据导入）
│   └── extractor/main.go         #   Schema 提取工具
│
├── api/                          # ── API 层（路由 + Handler） ──
│   ├── helper.go                 #   通用响应构建、VerifyAPIAuthorization 中间件
│   ├── struct.go                 #   通用结构体、错误常量
│   ├── bot_session_middleware.go  #   VerifyBotSession 中间件（JWT+Redis）
│   ├── groupguard/               #   群组管理端点
│   ├── public/                   #   公开端点（无鉴权）
│   │   ├── pjsk/                 #     PJSK 别名查询 → /api/v2/public/pjsk/alias/*
│   │   └── chunithm/             #     CHUNITHM 别名 + 曲目查询 → /api/v2/public/chunithm/*
│   └── bot/                      #   Bot 专属端点
│       ├── auth/                 #     Bot 注册/登录/会话验证/统计
│       └── pjsk/                 #     Bot 指令端点（由 handler registry 动态注册）→ /api/v2/bot/:botId/pjsk/*
│
├── internal/                     # ── 内部业务逻辑（不对外暴露） ──
│   ├── cache/drawingcache/       #   绘图图片缓存（存储、GC、统计、管理 API）
│   ├── cluster/                  #   集群节点角色 / 只读模式（config.Cfg.Node）
│   ├── core/crypto/              #   Noise NK 协议加密工具（含多 key 密钥环）
│   ├── core/trustsign/           #   Ed25519 分离载荷签名契约（keyset / manifest）
│   ├── core/upstream/            #   上游连接池 / Transport
│   ├── handler/                  #   统一命令注册表（handler.go + bot_route.go）
│   ├── identity/                 #   平台用户身份解析
│   ├── jsonutil/                 #   JSON 门面（json/v2 引擎 + v1 兼容语义）
│   ├── middleware/secure/        #   安全中间件
│   ├── observability/commandtrace/ # 命令执行追踪
│   ├── onebot11/                 #   OneBot11 协议工具（消息段、CQ 码、错误）
│   └── pjsk/                     #   PJSK 核心子系统
│       ├── accountdata/          #     账号绑定与 Profile 服务
│       ├── alias/                #     别名系统
│       ├── chartstyle/           #     谱面风格工具
│       ├── displaytime/          #     时间展示工具
│       ├── drawing/              #     Drawing API 客户端 + 缓存
│       ├── eventutil/            #     活动工具
│       ├── filteralias/          #     属性/筛选关键词别名表
│       ├── handler/              #     命令注册、端点归属、执行桥接
│       ├── meta/                 #     元数据工具
│       ├── parser/               #     指令解析与提取能力
│       ├── region/               #     区服类型定义
│       ├── requestbuilder/       #     请求构建器
│       ├── sekai/                #     上游 Sekai/Toolbox HTTP 客户端
│       ├── subscription/         #     订阅推送（MySekai 生日等）
│       └── render/               #     渲染与执行子系统
│
├── config/                       # ── 配置 ──
│   └── config.go                 #   YAML 配置加载，16 个顶级配置块
│
├── database/                     # ── 数据库层（Ent 自动生成） ──
│   ├── bot/                      #   Bot 用户、统计、Command Manifest
│   ├── censor/                   #   ⚠ 审核记录（API 层已删除，DB 表仍保留）
│   ├── chunithm/                 #   CHUNITHM 主库 + 曲目库
│   ├── pjsk/                     #   PJSK 别名、卡片、活动等
│   ├── sekai/                    #   Sekai 全量 Masterdata（511+ 文件）
│   └── users/                    #   通用用户表
│
├── ent/                          # ── Ent Schema 定义 ──
│   ├── bot/schema/               #   Bot 相关表定义
│   ├── censor/schema/            #   审核相关表定义
│   ├── chunithm/                 #   CHUNITHM 表定义
│   ├── pjsk/schema/              #   PJSK 表定义
│   ├── sekai/schema/             #   Sekai Masterdata 表定义
│   └── users/schema/             #   用户表定义
│
├── utils/                        # ── 工具库 ──
│   ├── redis/                    #   Redis 缓存管理
│   ├── imagecache/               #   图片缓存
│   ├── logger/                   #   日志
│   ├── censor/                   #   内容审核客户端
│   └── usererror/                #   面向用户的错误类型
│
├── data/                         # ── 静态数据（结构定义等） ──
├── deploy/                       # ── 部署相关文件 ──
├── scripts/                      # ── 运维脚本（provision_bot 等） ──
├── version/                      # ── 版本信息 ──
│
├── docs/                         # ── 文档 ──
├── integration/                  # ── 集成测试 ──
│
├── go.mod / go.sum               #   Go 模块定义
└── haruki-cloud.example.yaml     #   配置文件模板
```

---

## 3. 配置系统

配置通过 `haruki-cloud.yaml` 加载，顶级结构如下：

```yaml
profile: "dev"             # 部署环境: production / beta / temp / dev
                           # 影响 log_level / api_cache_ttl 默认值、recover stack trace 可见性
                           # production 强制关闭 allow_insecure_internal_api
                           # 可通过 HARUKI_PROFILE 环境变量覆盖

node:                      # 集群节点身份
  name: ""
  role: ""
  read_only: false         # true 时拒绝修改用户数据（internal/cluster）

backend:                   # 服务基础配置
  host: "0.0.0.0"
  port: 3000
  accept_authorization: "" # 内部 API 鉴权令牌
  accept_user_agent: ""    # 内部 API User-Agent 过滤
  allow_insecure_internal_api: false # 仅 dev/beta 时可开启；production 下强制关闭

redis:                     # Redis 连接
  addr: "localhost:6379"

pjsk:                      # PJSK 数据库
  db_url: "..."
pjsk_render:               # 渲染引擎配置
  drawing:
    base_url: ""           # Drawing API 地址
    timeout: 30
  asset_dirs: {}           # 素材目录
  local_masterdata: {}     # legacy/dev 本地 Masterdata fallback；生产默认关闭

sekai:                     # Sekai Masterdata 数据库
  db_url: "..."
  remote_sync: {}          # 可选：从远程维护的 PostgreSQL masterdata DB 定期同步到本地库

chunithm:                  # CHUNITHM（两个独立数据库）
  music_db_url: "..."
  binding_db_url: "..."

haruki_bot:                # Bot 管理数据库 + Bot 通道配置
  db_url: "..."
  credential_sign_token: ""  # JWT 签名密钥（登录凭据）
  session_sign_token: ""     # JWT 签名密钥（会话令牌）
  internal_api_token: ""     # 内部 API 回退令牌
  auth_v3_session_ttl: "1h"  # AuthV3 session 有效期，限定 [1m, 30d]
  noise_private_key: ""      # Noise NK 服务端私钥（legacy 单 key，key_id 为 default，必配其一）
  noise_keys: []             # 轮换用附加 key：[{key_id, private_key}]，全部可解密
  manifest_signing_key: ""   # 在线 Ed25519 seed（hex），签 command manifest；由 keyset 授权
  manifest_signing_key_id: ""
  trust_keyset_path: ""      # 离线签名的 keyset 文件，原样服务于 GET /api/v3/trust/keyset
  response_election_window: 0 # 多 bot 响应选举窗口
  response_election_roster: false
  allow_requests_without_nonce: false # 默认强制 timestamp/nonce；true 仅作应急回退
  request_nonce_window: 0

users_db:                  # 通用用户数据库（身份、绑定与全局封禁）
  db_url: "..."

moderation:                # 高权限全局管理命令
  admin_qq_ids: []         # 可执行 /kill 与 /back 的 QQ 白名单；空列表默认拒绝全部

censor:                    # 内容审核（百度/腾讯凭据 + censor DB）
  censor_db_url: "..."

toolbox:                   # Toolbox 外部服务
  base_url: ""

hmes:                      # HMES 外部服务（public/internal base_url + token）
  public_base_url: ""

sekai_api:                 # 上游 Sekai API 客户端
  base_url: ""

tracker:                   # SK Tracker 客户端
  base_url: ""
```

---

## 4. 鉴权体系

项目中存在 **三套鉴权机制**，适用于不同场景：

### 4.1 VerifyAPIAuthorization — 内部服务间调用

```
适用路径：/internal/bot/*（以及未来其他内部服务路由）
检查项：
  - Authorization 头 == config.backend.accept_authorization
    - 若未配置 `backend.accept_authorization`，则回退到 `haruki_bot.internal_api_token`，并按 `Bearer <token>` 组装
  - User-Agent 头包含 config.backend.accept_user_agent
默认行为：
  - 当 Authorization 与 User-Agent 两种约束都未配置时，默认拒绝访问
  - 只有显式设置 `backend.allow_insecure_internal_api=true` 时，才允许“无内部鉴权”放行
实现：api/helper.go
```

### 4.2 Bot 会话鉴权 — Bot 客户端直调

会话校验规则只有一套（`api.VerifyBotSessionToken`）：JWT 签名有效且未过期、
JWT `bot_id` claim == URL `:botId`、Redis (`hdb:bot:session:<botId>`) 中 token 一致。
token 的携带方式按路由分两种：

```
POST /api/v2/bot/:botId/pjsk/*      （Noise 之内）
  token 位于请求体顶层字段 session_token，服务端在 Noise 解密后读取；
  拒绝响应同样经 Noise 加密返回（MsgPack 信封）。
  实现：api/bot/pjsk/session_payload.go

GET  /api/v2/bot/:botId/command/manifests （无请求体，不在 Noise 之内）
  请求头：
    - X-Haruki-Bot-Id            → Bot 数字 ID，必须 == URL :botId
    - X-Haruki-Bot-Session-Token → JWT 会话令牌
  实现：api/bot_session_middleware.go（VerifyBotSession）
```

### 4.3 无鉴权 — 公开接口

```
适用路径：/api/v2/public/pjsk/alias/*,  /api/v2/public/chunithm/*,
          /api/v3/bot/:bot_id/auth, /api/v3/bot/:bot_id/logout
```

---

## 5. API 端点完整列表

### 5.1 Bot 会话管理（公开，无鉴权）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v3/bot/:bot_id/auth` | AuthV3 登录（Noise NK 通道，客户端只需预置服务端公钥 → 返回短期 session） |
| DELETE | `/api/v3/bot/:bot_id/logout` | 注销当前会话（携带 `X-Haruki-Bot-Session-Token`） |

本线 Cloud 没有明文或共享密钥的登录路径；旧的 `/api/v2/bot/:bot_id/auth`（共享 AES-256-GCM）
只存在于 2.11.x 旧 Cloud，双跑期结束后随旧 Cloud 一起下线。

AuthV3 契约（请求体 Noise NK Message 1，响应体 Message 2，payload 均为 MsgPack）：

| 方向 | 字段 | 说明 |
|------|------|------|
| 请求 | `bot_id`, `credential`, `timestamp` | 与 V2 相同；timestamp 窗口 ±300s |
| 请求 | `nonce` | 16 字节随机数的 hex（32 字符），按 bot_id + nonce 一次性消费 |
| 请求 | `method`, `path` | 必须等于实际 HTTP 方法与路径，防止密文搬到其他接口 |
| 请求 | `client_version`, `build_id` | 记录用途，当前不阻断 |
| 请求 | `noise_key_id` | 握手所用服务端公钥 ID；为空不校验，非空必须与实际匹配 |
| 响应 | `session_token`, `expires_at`, `session_id` | session 有效期由 `auth_v3_session_ttl` 决定，默认 1h |
| 响应 | `echo_nonce`, `server_time`, `accepted_build_id` | 回显与服务端时间 |

服务端可配置多把 Noise 静态密钥（`noise_private_key` + `noise_keys`），每把有 key_id。
客户端可通过 `X-Haruki-Noise-Key-Id` 请求头提示所用公钥；缺省时服务端依次尝试全部密钥。
响应头 `X-Haruki-Noise-Key-Id` 回传实际匹配的 key_id。auth 限流为每 bot_id 每分钟 10 次。

#### 信任密钥集与签名（trustsign）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v3/trust/keyset` | 离线签名的信任密钥集，Cloud 原样转发 `haruki_bot.trust_keyset_path` 指向的文件 |

签名契约（`internal/core/trustsign`，与客户端共享）：只签原始字节、分离载荷、域分隔。

```
签名输入 = domain || 0x00 || payload
Envelope = { alg:"ed25519", domain, key_id, encoding:"json"|"msgpack",
             payload:<base64 原始字节>, signature:<base64> }
domain ∈ { "haruki-cloud/keyset/v1", "haruki-cloud/manifest/v1" }
```

客户端先用对应公钥验签 `payload` 原始字节，再按 `encoding` 解码。两级密钥：

1. **离线根密钥**（仅运维持有，`cmd/trust-signer keygen`）签发 keyset 文档
   （`trustsign.KeysetDocument`：version / issued_at / expires_at / noise_keys /
   manifest_signing_keys / endpoints / minimum_client_version）。客户端内置根公钥，
   校验签名、版本递增与有效期。
2. **在线 manifest 签名密钥**（`haruki_bot.manifest_signing_key` + `_id`）由 keyset 授权，
   Cloud 用它签 command manifest。配置后 manifest 响应的 `data` 变为上述 Envelope，
   `payload` 为 `ManifestResponse` 的 JSON 字节。

> Bot 自助注册链路（send-mail / register / SMTP 验证码 / Turnstile）已移除；
> Bot 账号由运维通过 `scripts/provision_bot` 手动开通。

### 5.2 Bot 指令端点（Bot 会话鉴权）

当前 Bot 端点由 `internal/pjsk/handler` registry 动态派生，标准协议如下：

1. `GET /api/v2/bot/:botId/command/manifests`
2. `POST /api/v2/bot/:botId/pjsk/<path>`

其中：

1. Manifest 端点始终为 `GET + JSON`，会话走请求头 `X-Haruki-Bot-Id` /
   `X-Haruki-Bot-Session-Token`；配置了 manifest 签名密钥时 `data` 为签名 Envelope
2. PJSK Bot 业务端点为 `POST`，请求体为 `Noise NK + MsgPack(BotCommandRequest)`
   （生产必配 Noise 密钥；未配置时仅测试用 JSON 明文）
3. 会话 token 在请求体顶层字段 `session_token` 里随密文传输，不再放请求头；
   `/pjsk` 下所有 POST（含 birthday-monitor 的 render / ack）都必须携带
4. `BotCommandRequest.enableParamEcho` 默认为 `false`；客户端只有显式传 `true` 时，参数解析错误才会回显具体参数
5. `BotCommandRequest` 另有四个可选字段：
   - `event_time` / `event_id`：平台事件时间戳（OneBot time）用于事件级去重——
     同一条消息被多个 bot 观测到时时间一致，已消费的响应选举保留 120s，可区分
     重复投递（同时间 → 拒绝）与用户重发（更新时间 → 新选举）；未带该字段的请求
     沿用旧的 3s 宽限
   - `timestamp` / `nonce`：Noise 通道的按次投递重放保护（窗口校验 + SET NX
     单次 nonce）；默认强制，仅 `haruki_bot.allow_requests_without_nonce=true` 时放宽

代表性端点包括：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v2/bot/:botId/command/manifests` | 读取 command manifest；未配置 bot DB 时返回不可用响应 |
| POST | `/api/v2/bot/:botId/pjsk/card/detail` | 卡面详情 |
| POST | `/api/v2/bot/:botId/pjsk/card/list` | 查卡列表 |
| POST | `/api/v2/bot/:botId/pjsk/music` | 歌曲详情类路径之一 |
| POST | `/api/v2/bot/:botId/pjsk/event` | 活动详情类路径之一 |
| POST | `/api/v2/bot/:botId/pjsk/profile/bind` | 账号绑定 / 绑定列表 |
| POST | `/api/v2/bot/:botId/pjsk/profile/unbind` | 账号解绑 |
| POST | `/api/v2/bot/:botId/pjsk/profile/default` | 设置默认绑定 |
| POST | `/api/v2/bot/:botId/pjsk/profile/default/clear` | 取消默认绑定 |

需要特别说明：

1. 实际可用路径以运行时 handler registry 和 manifest 数据为准
2. Bot 端点执行结果不再只限于图片，也可能返回文本

### 5.3 PJSK 内部兼容渲染端点

截至 2026-04-09：

- `/internal/pjsk/*` 已从仓库与运行时中移除
- 当前不存在活跃的 PJSK 内部兼容 render/command HTTP 路由
- `internal/pjsk/render/` 仍保留为代码内部执行层，而不是外部可调用协议

### 5.4 PJSK 公开端点（无鉴权）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v2/public/pjsk/alias/:alias_type/by-alias` | 别名 → ID 查询 |
| GET | `/api/v2/public/pjsk/alias/:alias_type/:alias_type_id` | ID → 别名列表 |

### 5.5 CHUNITHM 端点（无鉴权）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v2/public/chunithm/alias/music-id` | 别名 → 曲目 ID |
| GET | `/api/v2/public/chunithm/alias/:music_id` | 曲目 ID → 别名列表 |
| GET | `/api/v2/public/chunithm/music/all-music` | 全部曲目 |
| GET | `/api/v2/public/chunithm/music/:music_id/difficulty-info` | 难度信息 |
| GET | `/api/v2/public/chunithm/music/:music_id/basic-info` | 基本信息 |
| GET | `/api/v2/public/chunithm/music/:music_id/chart-data` | 谱面数据 |
| POST | `/api/v2/public/chunithm/music/query-batch` | 批量查询 |

### 5.6 内部端点（VerifyAPIAuthorization 鉴权）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/internal/bot/verify-session` | 验证 bot_id + session_token |
| POST | `/internal/bot/statistics/record/:botID` | 统计数据上报 |
| POST | `/api/internal/group-guard/binding/check` | 群成员绑定检查 |
| POST | `/api/internal/group-guard/binding/check-batch` | 群成员绑定批量检查 |

---

## 6. 核心数据流

### 6.1 Bot 指令执行流程

```
Bot 客户端
  │
  ├─ 启动时: GET /api/v2/bot/:botId/command/manifests
  │          下载指令前缀→端点映射表，本地缓存
  │
  ├─ 用户发送指令 "/卡面 1001"
  │
  ├─ Bot 本地用前缀匹配到端点 /pjsk/card/detail
  │
  └─ POST /api/v2/bot/:botId/pjsk/card/detail
       Body:
         Noise NK + MsgPack(BotCommandRequest)
         （session_token / timestamp / nonce 均在密文内）
       │
       ▼ VerifyBotSession middleware
       │
       ▼ parseBotRequest()  ←── MsgPack / JSON 解码 → 恢复消息段
       │
       ▼ BuildContext()   ←── 从消息段提取纯文本参数 + at 列表
       │
       ▼ MatchCommandHandler(ctx.GetArgs())
       │
       ▼ 校验 registry 命中结果 == matched_command，且 handler.path == 当前端点
       │
       ▼ handler.Handle(...)
       │  → ResolvedCommand{Module:Card, Mode:"card-detail", Query:"1001"}
       │
       ▼ handler.Execute(ctx, resolved, renderApp)  [bridge.go]
       │  → 返回 onebot11.Message
       │
       └─ 200 OK, JSON 包装的 OneBot11 message segments
```

### 6.2 当前 PJSK 执行流程说明

当前 PJSK 已不再通过 `/internal/pjsk/*` 暴露内部兼容渲染路由。

现状是：

1. Bot 端点直接进入 `handler -> Execute -> render/snapshot`
2. 渲染控制器仍然存在，但仅作为代码内部执行层
3. 图片命令最终通过 Drawing API + ImageCache 返回 OneBot11 `image` segment

### 6.3 Bot 开通/登录流程

```
1. 运维执行 scripts/provision_bot  →  创建 Bot 账号 → 下发 JWT credential
2. POST /api/v3/bot/:bot_id/auth   →  secure 中间件 Noise NK 握手解密 → method/path/nonce/时间窗校验
                                    → JWT credential 验证 → 生成短期 session → 存 Redis
                                    → 同一握手加密返回（客户端预置公钥，二进制内无共享密钥）
3. DELETE /api/v3/bot/:bot_id/logout →  校验 session header → 删除 Redis 会话
```

---

## 7. 数据库架构

项目使用 **6 个独立数据库**，每个由 Ent ORM 管理：

| 数据库 | 目录 | 主要表 | 用途 |
|--------|------|--------|------|
| **Sekai** | `database/sekai/` | 511+ 实体表 | 游戏 Masterdata（卡片、曲目、活动等） |
| **PJSK** | `database/pjsk/` | alias, card, event, gacha 等 | 别名系统 + 查询索引 |
| **Bot** | `database/bot/` | user, daily/hourly_requests, requests_ranking | Bot 账号 + 统计 |
| **Users** | `database/users/` | user | 通用用户管理（API 层未使用，DB 保留） |
| **Censor** | `database/censor/` | namelog, result, shortbio | 内容审核（API 层已删除，DB 保留） |
| **Chunithm** | `database/chunithm/` | maindb, music | CHUNITHM 曲目数据 |

Schema 定义在 `ent/<module>/schema/` 下，通过 `go generate` 自动生成 `database/<module>/` 的 CRUD 代码。

---

## 8. internal/pjsk — 核心子系统详解

### 8.1 parser — 指令解析器

```
internal/pjsk/parser/
├── extractor.go          # Extractor：从文本中提取区服、角色、稀有度、属性、年份、uidArg 等
├── event_parser.go       # EventParser + EventQueryInfo
├── command_parser.go     # 其他命令解析辅助
├── utils.go              # isNumeric 等工具函数
├── types.go              # 共享类型定义
└── parser_test.go        # 测试（聚焦 Extractor）
```

> 历史上曾存在 `parser.go` (`CardParser`/`CardQueryInfo`)，已在 R38 删除。`global_resolver.go`（兼容型全局解析器）也已在后续 handler 重构中删除。card 查询解析统一走 `internal/pjsk/render/card/parser.go`。

### 8.2 handler — 指令处理

```
internal/handler/                 # 统一命令注册表
├── handler.go                    # 路由元数据、命令注册、manifest 分发
└── bot_route.go                  # Bot route 类型定义

internal/onebot11/                # OneBot11 协议工具（已从 pjsk 上移）
├── segment.go                    # 消息段类型与构造器
├── parse.go                      # CQ 码解析
└── error.go                      # ReplayError

internal/pjsk/handler/            # PJSK 功能命令（已扁平化，无子包）
├── handler.go                    # Trie 注册、命令匹配、参数截取
├── sekai_registry.go             # 各命令注册入口
├── command_executor.go           # 执行器绑定
├── command_request.go            # 类型化命令输入
├── execute_prelude.go            # 统一执行前置（封禁检查、区服默认值、时区、上下文构建）
├── execution_helpers.go          # 执行辅助函数（缓存落盘、segment 封装）
├── context.go                    # Event, Context 接口, HandlerContext
├── runtime.go                    # 运行时装配
├── messages.go                   # 消息构建辅助
├── helpers.go                    # 工具函数
├── alias.go ... vlive.go         # 各功能命令（解析 + 执行一体化）
├── deck_*.go                     # 组卡相关（builder, config, extractor, helpers, types 等）
├── sk_*.go                       # 冲榜相关（params, parse）
├── score_*.go                    # 分数相关（board_params 等）
├── profile_*.go                  # Profile 相关（settings, bg）
├── mysekai_*.go                  # MySekai 相关（parse, gate）
├── resolver_*.go                 # 各类解析器（snapshot, profiles, targets, character 等）
└── *_test.go                     # 测试文件
```

**设计说明：** 原有的 `bridge_*.go` 分发层和 `sekai/` 子包已在 2026-04-18 的 handler 重构中合并。每个命令文件（如 `card.go`、`music.go`）现在同时包含解析逻辑和执行逻辑，不再需要中间桥梁层。`execute_prelude.go` 提供统一的执行前置处理（封禁检查、区服默认值、时区设置），各命令执行器直接返回 `onebot11.Message`。

### 8.3 render — 渲染子系统

```
internal/pjsk/render/
├── app/app.go            # App 结构体：组合根，包含所有 Controller 字段
├── source/               # 数据源注册中心
├── masterdata/           # Masterdata 类型定义
├── snapshot/             # 用户游戏快照（live + local fallback）
├── provider/             # 大型 Masterdata 数据 Provider（DB/local 双源）
├── releasecheck/         # 资源版本检查
├── common/               # 共享工具（卡图缩略图）
├── assets/               # 素材管理
│
│   ── 功能模块（其中 vlive 为文本模块） ──
├── card/                 # 卡片（detail, list, box）
├── costume/              # 3D 服装 / 预览
├── inventory/            # 库存分类查询
├── music/                # 曲目（detail, list, chart, progress, rewards）
├── event/                # 活动（detail, list, record）
├── gacha/                # 卡池（detail, list）
├── deck/                 # 组卡推荐
├── education/            # 教育系统（挑战赛, 加成, 区域道具, 羁绊, 领队统计）
├── score/                # 分数（control, custom-room, music-meta, music-board）
├── sk/                   # SK 排名（line, query, check-room, speed, trace, winrate）
├── honor/                # 称号
├── profile/              # 个人名片
├── stamp/                # 贴纸
├── misc/                 # 杂项（角色生日）
├── mysekai/              # MySekai（资源, 家具, 大门, 唱片, 对话）
└── vlive/                # Virtual Live（当前仅文本查询）
```

每个模块通常包含：
- `controller.go` — 对外暴露的 Controller 方法
- `query.go` — 查询参数结构体
- `builder.go` — 构建渲染请求 payload
- `source.go` / `source_cloud.go` — 数据获取层

### 8.4 chartstyle — 谱面风格工具

```
internal/pjsk/chartstyle/
└── style.go              # 谱面风格路径解析与映射
```

---

## 9. 已知问题 & 技术债

### ⚠ 结构问题

| 问题 | 位置 | 说明 |
|------|------|------|
| `exports/` 混合导出与临时产物 | `exports/`（本地目录，不入 Git） | 作为 `cmd/importer` 输入的 legacy JSON 快照，尚未形成清晰约束与归档规则 |

### ⚠ 技术债

| 项目 | 说明 |
|------|------|
| 本地用户快照 | `render/snapshot/local.go` 读取本地 JSON 文件（user.json, music_metas.json, mysekai.json），应迁移至 DB 驱动 |
| MySekai Masterdata | 依赖本地文件，未完全转为 DB 驱动 |
| Deck 引擎 | 简化版实现，原生 CGo 引擎未迁入 |
| Profile 扩展命令未完成 | `internal/pjsk/handler/profile.go` | 绑定/解绑/默认绑定已接入；`swap bind`、隐藏/展示抓包、隐藏/展示 ID、注册时间、服务状态、抓包模式仍为 disabled/TODO |

---

## 10. 构建 & 测试

```bash
# 构建主服务
go build .

# 运行全部测试
go test ./...

# 单独测试各子系统
go test ./api/public/...                     # 公开 API（pjsk alias, chunithm）
go test ./api/bot/...                        # Bot 端点（auth + pjsk）
go test ./internal/pjsk/parser/...          # 指令解析器
go test ./internal/pjsk/handler/...         # Handler 子系统
go test ./internal/pjsk/render/...          # 渲染子系统
```

> 说明：当前仓库默认 `go test ./...` 已可直接通过；`integration` 测试默认关闭，需显式设置 `HARUKI_RUN_INTEGRATION=1` 才执行。

---

## 11. 文件清单 — `api/` 职责对照

### api/（共享层，package api）

| 文件 | 职责 |
|------|------|
| `helper.go` | 通用响应构建、`VerifyAPIAuthorization` 中间件 |
| `struct.go` | 通用结构体、错误常量 |
| `bot_session_middleware.go` | `VerifyBotSession` 中间件（JWT+Redis），适用于 `/api/v2/bot/:botId/*` |

### api/public/pjsk/（package pjsk）

| 文件 | 职责 | 关联路由 |
|------|------|----------|
| `route.go` | 公开别名路由注册 | `/api/v2/public/pjsk/alias/*` |
| `alias.go` | 别名查询 Handler | `/api/v2/public/pjsk/alias/*` |
| `struct.go` | 别名请求/响应结构体 | — |
| `helper.go` | PJSK 公开端点通用 Helper | — |
| `alias_test.go` | 别名公开 API 测试 | — |

### api/public/chunithm/（package chunithm）

| 文件 | 职责 | 关联路由 |
|------|------|----------|
| `route.go` | CHUNITHM 公开路由注册 | `/api/v2/public/chunithm/*` |
| `alias.go` | 别名查询 Handler | `/api/v2/public/chunithm/alias/*` |
| `music.go` | 曲目查询 Handler | `/api/v2/public/chunithm/music/*` |
| `struct.go` | 请求/响应结构体 | — |
| `helper.go` | CHUNITHM 通用 Helper | — |
| `public_query_test.go` | 公开查询 API 测试 | — |

### api/bot/auth/（package auth）

| 文件 | 职责 | 关联路由 |
|------|------|----------|
| `route.go` | 路由注册入口（user / internal / statistics 三组） | — |
| `user.go` | 公开路由注册（AuthV3 登录 + 注销） | `/api/v3/bot/:bot_id/auth`, `/api/v3/bot/:bot_id/logout` |
| `credential.go` | credential 校验（bcrypt / 常量时间比较） | — |
| `session.go` | credential JWT 解析、所有者封禁检查、注销 Handler | — |
| `session_v3.go` | AuthV3 登录 Handler（Noise NK 通道，nonce 单次消费，请求上下文绑定） | — |
| `internal.go` | 内部 session 验证 | `/internal/bot/verify-session` |
| `statistics.go` | 统计上报 Handler | `/internal/bot/statistics/record/:botID` |
| `telemetry.go` / `telemetry_dispatcher.go` | Bot 遥测采集与转发 | — |
| `struct.go` / `helper.go` | 结构体与辅助函数 | — |

### api/trust/（package trust）

| 文件 | 职责 | 关联路由 |
|------|------|----------|
| `keyset.go` | 原样转发离线签名的信任密钥集，按 mtime 热重载 | `/api/v3/trust/keyset` |

### api/bot/pjsk/（package pjsk）

| 文件 | 职责 | 关联路由 |
|------|------|----------|
| `handler.go` | `makeBotHandler`、MsgPack/JSON 请求解码、handler registry 派生路由注册、manifest 端点 | `/api/v2/bot/:botId/pjsk/*`, `/api/v2/bot/:botId/command/manifests` |
| `dedup.go` / `dedup_cleanup.go` | 事件级去重（event_time/event_id） | — |
| `replay.go` | Noise 通道重放保护（timestamp/nonce 窗口 + SET NX） | — |
| `response_election*.go` | 多 bot 响应选举（窗口、key、roster、生成） | — |
| `bot_response_envelope.go` | 响应封装 | — |
| `command_trace.go` | 命令执行追踪接入 | — |
| `param_echo.go` / `param_guidance.go` | 参数回显与参数引导 | — |
| `birthday_monitor.go` | MySekai 生日订阅推送 | — |
| `seed.go` | 从 handler registry 同步 command manifest 到 bot DB | — |
| `struct.go` | `BotCommandRequest`、`ManifestEntry`、`ManifestResponse` | — |

---

## 12. 相关文档索引

| 文档 | 说明 |
|------|------|
| `docs/database-schemas.cn.md` | 数据库 Schema 详解 |
| `docs/pjsk-command-system.cn.md` | PJSK 指令解析 + 请求构建系统技术文档 |
| `docs/toolbox-api.cn.md` | 上游 Toolbox API 契约 |
| `docs/deck_refer_help.md` | `deck` 命令族用户帮助文本 |

---

**维护者**：Haruki-Cloud Team  
**文档版本**：v2.0  
**创建日期**：2026-03-23
