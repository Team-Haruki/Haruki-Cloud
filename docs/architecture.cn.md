# Haruki-Cloud 项目架构文档

> 最后更新：2026-09-14（v2.1）
>
> 2026-09-14 补充（Phase-2 存储抽象）：
> 1. 新增 `internal/storage`（`Store` 接口：`Get/Put/Stat/Delete/List/ListDir`，`fs` 与 `s3` 两个后端，
>    五个固定槽位 `assets` / `user_upload` / `static` / `cache` / `image_cache`）与
>    `internal/core/urlhost`（按 Drawing 节点选择公开 image-cache / assets 主机）。
> 2. 渲染缓存索引迁往 PostgreSQL（`image_cache_entries` + `render_cache_index`）；Drawing 以
>    ArtifactRef 回传已上传的对象，Cloud 只输出 URL。旧的 `internal/cache/drawingcache`（SQLite +
>    `/cache` API）、Cloud 侧渲染缓存写入与谱面静态缓存已在门控删除任务（T16）中移除，见
>    `docs/storage-migration.cn.md`。
> 3. 新增图片缓存 GC（`image_cache.gc_*`，默认关闭、开启后默认 dry-run）与 `/ic/*` 301 开关
>    （`image_cache.legacy_redirect.enabled`）。
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
> 4. `internal/` 新增 `cache/drawingcache`（绘图缓存，已于 2026-09 Phase-2 T16 删除）、`cluster`（节点只读模式）、
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
│   ├── cluster/                  #   集群节点角色 / 只读模式（config.Cfg.Node）
│   ├── core/crypto/              #   Noise NK 协议加密工具（含多 key 密钥环）
│   ├── core/urlhost/             #   按节点选择公开主机（image_cache.hosts / assets_base_urls）
│   ├── core/trustsign/           #   Ed25519 分离载荷签名契约（keyset / manifest）
│   ├── core/upstream/            #   上游连接池 / Transport
│   ├── handler/                  #   统一命令注册表（handler.go + bot_route.go）
│   ├── identity/                 #   平台用户身份解析
│   ├── jsonutil/                 #   JSON 门面（json/v2 引擎 + v1 兼容语义）
│   ├── middleware/secure/        #   安全中间件
│   ├── observability/commandtrace/ # 命令执行追踪
│   ├── onebot11/                 #   OneBot11 协议工具（消息段、CQ 码、错误）
│   ├── storage/                  #   文件读写抽象：Store 接口、fs / s3（Garage）后端、槽位配置
│   └── pjsk/                     #   PJSK 核心子系统
│       ├── accountdata/          #     账号绑定与 Profile 服务
│       ├── alias/                #     别名系统
│       ├── chartstyle/           #     谱面风格工具
│       ├── displaytime/          #     时间展示工具
│       ├── drawing/              #     Drawing API 客户端、C13 指令头、ArtifactRef、渲染缓存
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
│   ├── imagecache/               #   图片缓存（image_cache 槽位、PG 索引、GC）
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
  asset_dirs: {}           # 公开素材主机 assets_base_urls（必填）；primary 已弃用（E1）
  storage: {}              # 五个存储槽位（fs / s3），缺省时从旧目录派生
  asset_probe: {}          # 无本地素材根时按 assets 槽位选路径（startapp/ondemand、大小写）；positive_ttl 6h / listing_ttl 30m / negative_ttl 5m / timeout 3s / warm_prefixes []
  image_cache: {}          # pg_url、hosts、render_index.*、gc_*、legacy_redirect
  drawing_artifact: {}     # Artifact 模式放量白名单
  local_masterdata: {}     # legacy/dev 本地 Masterdata fallback；生产默认关闭。所有 master 读取（含库存道具表、自定义名片资源、MySekai 大门皮肤/自定义谱面标签、JP musicCategories）都先走 DB，只有开启 enabled+allow_fallback（或 allow_leaks）且某张表为空/不可用时才读本地 JSON
  masterdata_registry: {}  # url / poll_interval / settle_delays：轮询 master registry 的 /v1/master/{region}/current，contentHash（缺省时 ETag）变化时立即重置该区服 DB provider 缓存，并在 settle_delays（默认 5m、15m）后再重置以等待 DB ingest 落库；url 为空时依次取 deck_recommend.registry_url、music_meta.base_url（source=registry）

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

#### 请求耗时日志

`internal/observability/commandtrace` 将固定名称的计时聚合到请求 context。访问日志开启时，共用 Fiber 入口创建 trace，并在既有 `http_request` 日志的 `operation_stats` 中输出各环节的 `count`、`total_ms`、`max_ms`；因此 CHUNITHM 和 PJSK 的公开查询也能关联 Redis、数据库与响应编码耗时。Bot 指令继续输出 `bot_command` 汇总，并复用同一 trace；关联两种日志应使用 `request_id`，不能将它们当成两次执行相加。

Bot 的 `phase_stats` 表示互不重叠的顶层阶段，可与 `server_duration_ms` 比较；`operation_stats` 是包含子操作的诊断计时，并行或嵌套的总值不能相加作为请求总耗时。`http_request` 的 `duration_scope=fiber_handler` 到响应体设置结束，不包含 Bot 下载图片、平台发图或客户端接收完成的时间。`response.body_set` 仅测 Fiber 响应体设置，`drawing.http` 是上游请求完整往返，不是 Drawing 的纯绘制时间。

主要诊断层次如下（仅经过相应路径时出现）：

| 环节 | operation 前缀或名称 |
|---|---|
| 请求与指令解析 | `request.*`、`command.context_build`、`command.match`、`command.parse` |
| 运行时与绑定 | `runtime.*`、`binding.*`、`target.resolve` |
| 用户快照 | `snapshot.*`、`toolbox.http`、`toolbox.decompress`、`mysekai.snapshot_*` |
| 数据库与主数据等待 | `db.query`、`db.mutate`、`masterdata.cachefill_*`、`mysekai.provider_*` |
| 卡组与歌曲数据准备 | `deck.userdata_*`、`deck.music_meta_lookup`、`music.achievement_*` |
| 资源读取 | `asset.store_get`、`asset.read_file`、`asset.store_stat`、`asset.stat`、`asset.store_directory_wait` |
| 绘图与图片结果 | `drawing.*`、`image.result_url`、`image.result_bytes`、`image.*` |
| 公开查询缓存与响应 | `api.cache_*`、`api.data_fetch`、`response.*` |

Bot 的共享执行会给内部操作添加 `command.shared.operation.` 前缀；内部阶段以 `command.shared.phase.` 进入 operations，它们同样是包含式诊断数据，不能再加到外层 `phase_stats`。上表列出的是加前缀之前的名称。

缓存命中、未命中、绕过、共享等待与取消等固定事件也聚合为 operation；这类事件的 `count` 有意义，耗时为零。原始快照的缓存命中只在 Toolbox 鉴权成功并确认版本未变后记录。快照共享构建、绘图和图片下载使用独立 trace，完成后才把操作统计合入仍在等待的请求；取消的等待者不接收后续完成统计。`cachefill.Group` 保留同步等待约定：每个调用者记录 `masterdata.cachefill_wait`，内部查询与解码归首发请求，其他等待者不重复记录内部工作；取消也须等待这一同步调用结束。计时名称不包含账号、查询内容、缓存键、SQL 或资源路径，也不为每次子操作单独写日志。

### 6.3 图片缓存与图片结果

渲染缓存基础键由 `internal/pjsk/drawing` 的 `cache_helpers.go`、`cache_hash.go`、`cache_rules.go` 计算，原有算法由 `cache_key_golden_test.go` 固定。启用 `drawing_cache_versions.enabled` 后，在基础键外加入请求引用的素材分片 revision 与 Drawing 节点集合的 renderer epoch，独立形成新版本键；关闭时保留原键。

**索引与对象。** 渲染索引在 PostgreSQL：`image_cache_entries`（按内容 hash 去重，记录 `cdn_path`、`storage_backend`（`garage` / `legacy_disk`）、`media_type`、`last_referenced_at`）与 `render_cache_index`（`request_key` → `content_hash`，带 `ttl_seconds` / `expires_at`）。DDL 只由 Cloud 执行（`image_cache.render_index.ddl_enabled`），Drawing 不执行 DDL，遵循事务锁与上传意图协议。对象存放在 `image_cache` 槽位（生产为 Garage 的 `image-cache` 桶，`root=""`）；启用完整生命周期 schema 的新内容写入使用带随机 generation 的对象 key：Drawing 为 `pjsk/api/<api_path>/<sha256>-<generation>.<ext>`，Cloud 为 `pjsk/<sha256>-<generation>.<ext>`；去重始终按 PG 中的内容 hash 与已记录 `cdn_path`，兼容旧路径，两者共用同一表与桶。无 PG 或旧 schema 的兼容模式保留确定性 key 与存在性探测，不能开启物理 GC。只针对 Drawing 产物的运维工具必须使用前缀 `pjsk/api/`。

**请求路径。** `drawing_artifact.endpoints` 白名单内的端点发送完整 C13 指令头（`X-Haruki-Artifact: 1`、`X-Haruki-Cache-Key`、`-Key-Version`、`-TTL`、`-Group`、`-Api-Path`、`-User-Id`、`-Store`）；Drawing 返回 ArtifactRef（含 `node_name`），Cloud 用 `urlhost` 优先选该节点的公开主机输出 URL，不读取图片字节。Drawing 仍可能返回 `image/*` 字节（`Cache-Store: 0`、写入降级、未升级的 Drawing），这是永久分支。`render_index.lookup_enabled` 打开后命中直接来自 PG，命中续期（`TouchRender`）按 `touch_interval` 限流。Cloud 不再持久化渲染字节：未进入白名单的端点（以及上述字节分支）只进入进程内暂存缓存，没有索引行即视为未命中；`/cache`、`/cache/stats` 与 SQLite 绘图缓存均已删除。谱面（`/api/pjsk/chart`）不再使用 `image_cache.charts_uri` 静态缓存，而是走同一渲染路径，渲染规则 TTL 为 7 天（该规则只影响 TTL，不改变缓存键）。

**不存储路径。** `drawing_artifact.no_store_paths`（按整段前缀匹配，只作用于已在 `endpoints` 白名单内的路径；缺省为 `api/pjsk/deck`、`api/pjsk/event/planner`、`api/pjsk/sk` 以及 mysekai 的 `map`/`resource`/`talk-list`/`music-record`/`door-upgrade`，显式 `[]` 关闭）用于复用率接近零的按用户渲染：Cloud 跳过 pending 缓存与 `render_cache_index` 查询，指令头带 `X-Haruki-Cache-Store: 0`，Drawing 直接返回字节、不上传 Garage 也不写索引；返回的字节与其他字节结果一样经 image cache 存储后以 URL 发给 bot，绝不以 `base64://` 内联：bot v2 响应是单条 Noise 消息（上限 65535 字节），内联图片必然加密失败（3.7.3 事故）。这类结果也不进入进程内缓存：Drawing 对缺素材图同样回 `Cache-Store: 0`，Cloud 无法区分“主动不存”和“缺素材不可缓存”。

**GC。** `utils/imagecache.GC` 由应用生命周期管理，`gc_enabled` 默认关闭，`gc_dry_run` 默认开启。过期渲染索引的 DELETE 原子复核选取时的过期条件，避免删除已续期的行。物理对象回收另外受 `gc_object_delete_enabled` 控制，只有所有 Cloud 与 Drawing 写入者采用同一生命周期协议后才能开启。内容写入与回收按内容 hash 使用 PostgreSQL 事务锁；回收先提交索引退役与持久 outbox，再在新事务中复核引用并删除旧 generation 对象。删除失败的债务保存在数据库，重启后继续处理。随机 generation key 使超时后晚到的旧 DELETE 无法命中新写入的对象。上传前独立提交意图记录，上传与索引成功后消费；进程崩溃或索引失败留下的意图可在期限后回收。

`cmd/image-cache-reconcile` 分页检查索引指向的对象，默认只检查；`--repair` 仅移除已确认缺失对象的索引引用，供后续请求重画，不删除对象。该工具使用只读 schema 探测，不执行 DDL；只接受无本地目录覆盖、bucket 根路径的显式 S3 槽位，避免检查错误存储目标。正常图片缓存命中仍保持零 HEAD/GET。

**旧路由 `/ic/*`（E3）。** 默认仍由 `static.New(image_cache.dir)` 原样提供文件；`image_cache.legacy_redirect.enabled` 且配置了图片主机时改为 301 到同一 key 的主机 URL（非法 key 返回 404，重定向本身 `Cache-Control: public, max-age=86400`）。开启后至少保留 30 天。

**字节与暂存。** 渲染缓存只在 `render_index.lookup_enabled` 打开且索引可用时启用；`ImageResult` 要么携带字节，要么携带 ArtifactRef（只有字节消费者调用 `Bytes(ctx)` 时才从 `image_cache` 槽位或主机读回）。暂存缓存最多 128 项、64 MiB，有效期不超过 120 秒与业务 TTL；新 ArtifactRef 暂存节点提示，即使 Drawing 已写入索引也保留到该期限。索引保存短期有效的 writer node 提示，使其他 Cloud 实例优先访问刚写入的节点。Drawing 响应明确 `Cache-Store: 0` 时不进入持久或暂存缓存。

Bot 命令和生日推送使用控制器的 `Render*Image` 与 Drawing 客户端的 `Generate*Image` 入口，将 `ImageResult` 保留到消息构建阶段；多图结果逐张选择公开 URL。旧 `[]byte` 入口保留兼容，通过 `Bytes(ctx)` 读取图片。没有可用公开主机时也会读取字节，沿用图片存储回退。活动详情与别名列表继续使用不缓存的字节响应。

命令计时中，`drawing.http` 是 Drawing 请求的完整往返；`drawing.artifact_fetch` 单独统计引用转为字节时的等待与下载，`drawing.artifact_store` 与 `drawing.artifact_public` 分别统计对象存储读取和公开主机读取。共享下载的内部操作会并入各等待请求的 trace，因此这些操作时长可能重叠，不能直接相加作为总耗时。直接返回图片引用的路径没有图片下载操作。

绘图缓存键准备对已规范化请求一次完成复制和字段清理，忽略字段的子树不参与复制；保留的 map/slice 与渲染请求分离，允许后续渲染准备修改原请求。标准 JSON 子树使用与 hashstructure FormatV2 相同的 FNV-1、数值表示和集合/序列组合规则直接计算哈希，特殊类型回退到原实现；关闭显式版本协议时，外围键结构和版本保持不变，已有持久缓存键继续兼容。请求 JSON 规范化和时间/时区处理仍沿用原入口。

对象存储路径选择会按完整、有序的候选序列缓存成功结果，使用有界 LRU（最多 65,536 项、键与结果字符串共 16 MiB）。后续候选已经命中时，不再为前面的缺失路径重复列目录。成功选择从本次查找开始计时，最多保留 `min(listing_ttl, positive_ttl)`（默认 30 分钟），命中不会续期；到期重新按原优先顺序查找。`ClearResolutionCache` 同时清除选择结果，并阻止清理前的在途查询回填。全部候选缺失或查询出错不写入这层缓存，仍沿用既有负缓存/错误重试周期。更高优先级的新资源在选择缓存有效期内可能暂时不可见，可通过清缓存立即更新。`asset.store_selection_cache_hit/miss` 记录这层缓存的命中情况。

### 6.3.1 资源清单、并发与后台持久化

`render/assets` 的候选解析与 `AssetReader.StatResult` 共享 found / missing / unknown 元数据、目录缓存和 singleflight。活动、卡牌、扭蛋、歌曲与虚拟 Live 的列表先筛选展示范围，再以最多 8 个 worker 预热纯资源查询；DB 与业务构建仍按原顺序执行。背包直接传递有序图片候选，商店工具与材料先过滤再构建图标。

`storage.Set` 持有跨槽位的 I/O runtime。`storage.io` 默认最多 16 个在途请求、每 origin 8 个、后台 2 个；后台扫描不会占满前台容量。排队可取消，单次节点尝试按剩余总预算分配时间，429/5xx 有界退避，分页检测重复 token 与页数上限。`storage.*` operation 记录逻辑操作、排队、尝试、分页、连接复用、TTFB、body 与字节数；后台无指令 trace 的操作也进入进程统计，每五分钟记录变化量。并发操作的累计耗时不是用户等待时间。

Asset-Updater 在完整区域上传和谱面同步成功后、区域任务锁内发布 `indexes/assets/v1/<region>/current.json`。指针引用带 SHA-256 的不可变分片和 BPM 索引，发布前重新核对完整 inventory；取消、部分失败或数据不完整不会推进指针。Cloud 的 `asset_index` 定期读取指针，完整验证后原子安装索引并清空旧解析缓存。索引保留精确大小写；未就绪、损坏或过期时回退到有限存储查询，不能据此认定资源不存在。`cmd/asset-index` 提供人工引导工具，默认仅扫描；`--publish` 前须暂停该区域更新任务。

`drawing_cache_versions.enabled` 要求 `drawing_artifact.endpoints` 包含 `"*"`；配置加载与启动会拒绝部分启用，避免字节模式跳过资源版本。开启后 Cloud 定期查询各 Drawing 节点的 `/cache/identity`。请求携带 `X-Haruki-Asset-Revision` 与所选节点的 `X-Haruki-Renderer-Epoch`；Drawing 按请求资源版本隔离文件镜像与解析缓存，并在实际 epoch 不匹配或缺素材时返回不可缓存结果。节点身份未知/过期时 Cloud 绕过缓存；稳定的异构节点可以有不同 epoch。滚动发布期间可以使用当前池中任一节点生成的图片，节点集合 epoch 更新后缓存键随之改变，可见性受配置的轮询周期约束。

BPM 查询优先读发布端生成的区域索引，不在线下载整批 SUS。没有有效索引时，同谱面请求合并、读取有界并发，结果按区域资源版本隔离；不把临时读取或解析错误记成不存在。

用户背景修改采用账户 revision CAS 与持久清理队列；先提交数据库状态，旧图再由主写节点回收。预测与房屋统计的可重建 JSON 缓存采用单 worker 合并后台写入，应用关闭时有界 flush。`cache_persistence_namespace` 应为每个同时运行的 Cloud 实例指定不同的稳定名称，缺省使用主机名加进程 ID，避免多个进程覆盖同一 key；首次读取可兼容旧共享 key，写入只落新命名空间。

### 6.4 Toolbox 快照缓存

Toolbox 在内容变化时推进 `upload_time`，Cloud 沿用该值标识版本。原始数据缓存按区服、数据类型、游戏账号和 Suite 字段集共享；每个独立请求仍携带自己的平台身份向 Toolbox 发起条件读取，只有鉴权成功并确认版本未变后才复用数据。读取失败不会返回缓存中的私有数据。

原始 JSON 在接收时复制为内部不可变 payload，并保存一次解析的 `upload_time`。请求缓存和构建缓存查询共享该 payload，热命中不再复制完整 JSON 或重新扫描时间戳。对外返回可修改字节的接口仍返回独立副本；Snapshot 工厂将输入 JSON 视为只读，现有模型访问器的复制约定保持不变。

构建缓存按区服、游戏账号、Suite 字段集与版本以及是否需要 MySekai 和其版本保存 Snapshot。同一键的并发构建通过 singleflight 合并，合并范围不包括绑定查询和 Toolbox 鉴权。等待者可独立取消；共享构建使用独立的 30 秒超时上下文，构建错误不进入缓存。需要 music meta 或缺少有效源版本时仍直接构建。原始数据和已构建 Snapshot 的容量、TTL 与淘汰边界保持原有配置。

商店、对话列表和大门升级通过 `ResolveOptions.SuiteFields` 选择已核对的 Suite 顶层字段；其他命令读取完整 Suite，MySekai 始终完整读取。字段集排序去重，并包含身份和 `upload_time`，参与请求缓存和共享构建键。普通 Suite + MySekai JSON 以 `RawMessage` 按字段合并，保留既有覆盖和空数组规则；Extended JSON、数组导出及重复键文档仍使用原规范化路径。

### 6.5 卡牌全集与 Deck 上传缓存

数据库卡牌 provider 对无筛选条件的全集查询（可带 Limit）按区服缓存已转换模型，保留 release_at 排序；带筛选条件的查询沿用原有 SQL 和内存筛选。全集冷加载通过 singleflight 合并，同时填充单卡缓存，对外返回模型副本。缓存沿用主数据索引的 5 分钟 TTL，并由 ResetMasterdataCache 显式失效；generation 检查阻止刷新前的旧加载覆盖新缓存。

Deck 的每个远端目标状态持有独立 userdata 缓存，以最终上传字节的 SHA-256 为键，仅保存远端 hash。每个目标最多 256 项、TTL 10 分钟，单个 hash 最多 4 KiB；缓存不保留用户数据或压缩包。同目标同内容上传通过 singleflight 合并，只有发起上传的请求复制和压缩数据。更改满配或筛卡预设导致最终字节变化时会重新上传。

远端报告 userdata_hash 丢失时，只失效本次使用的缓存条目，重新上传并重试一次；再次失败保留原有旧协议回退。旧请求的延迟失败不会删除新上传条目。上传使用受超时约束的上下文，并关联发起请求的取消；发起请求取消后等 HTTP 清理完成再释放并发名额，其他仍有效的等待者可在自己的请求下重试上传，普通等待者可独立取消。

### 6.6 Bot 开通/登录流程

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
├── cachefill/            # DB 缓存回填协调：同 key 并发共享一次查询，失败后 5s 内不重试
├── releasecheck/         # 资源版本检查
├── common/               # 共享工具（卡图缩略图）
├── assets/               # 素材管理：路径选择先探本地根，无本地根（或全部未命中）时按 assets 槽位的目录列举（缓存 listing_ttl、宽目录一次递归列举）判存在并纠正大小写，HEAD 仅作无法列举目录的兜底；连续 3 次失败后熔断 30s
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
