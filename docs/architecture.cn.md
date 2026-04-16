# Haruki-Cloud 项目架构文档

> 最后更新：2026-04-09（v1.7）
>
> 2026-04-09 补充：
> 1. `api/legacy/pjsk/` 已从仓库与运行时移除。
> 2. PJSK Bot 主协议已经收口到 `POST /api/v2/bot/:botId/pjsk/<path>`。
> 3. `internal/pjsk/render/deck/deck_cgo/` 历史目录已从仓库移除，deck recommend 运行时仅保留 HTTP 外部服务。
> 4. 当前活跃 Bot path 数量与模块分档，请优先参考 [项目完成度跟踪](project-completion-tracker.cn.md)。

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
| 认证 | JWT (golang-jwt/v5) + AES-256-GCM + Noise IK |
| JSON | bytedance/sonic（高性能替代 encoding/json） |
| Go 版本 | 1.26.1 |

---

## 2. 目录结构总览

```
Haruki-Cloud/
├── cmd/                          # ── 入口程序 ──
│   ├── server/main.go            #   主服务入口（唯一运行的进程）
│   ├── migrate/main.go           #   数据库迁移工具
│   └── extractor/main.go         #   Schema 提取工具
│
├── api/                          # ── API 层（路由 + Handler） ──
│   ├── helper.go                 #   通用响应构建、VerifyAPIAuthorization 中间件
│   ├── struct.go                 #   通用结构体、错误常量
│   ├── bot_session_middleware.go  #   VerifyBotSession 中间件（JWT+Redis）
│   ├── public/                   #   公开端点（无鉴权）
│   │   ├── pjsk/                 #     PJSK 别名查询 → /api/v2/public/pjsk/alias/*
│   │   └── chunithm/             #     CHUNITHM 别名 + 曲目查询 → /api/v2/public/chunithm/*
│   └── bot/                      #   Bot 专属端点
│       ├── auth/                 #     Bot 注册/登录/会话验证/统计
│       └── pjsk/                 #     Bot 指令端点（由 handler registry 动态注册）→ /api/v2/bot/:botId/pjsk/*
│
├── internal/                     # ── 内部业务逻辑（不对外暴露） ──
│   ├── core/crypto/              #   Noise 协议加密工具
│   ├── middleware/secure/        #   安全中间件
│   └── pjsk/                     #   PJSK 核心子系统
│       ├── parser/               #     指令解析与提取能力
│       ├── handler/              #     命令注册、端点归属、执行桥接
│       └── render/               #     渲染与执行子系统
│
├── config/                       # ── 配置 ──
│   └── config.go                 #   YAML 配置加载，10 个顶级配置块
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
│   ├── drawing/                  #   Drawing API 客户端 + 缓存
│   ├── redis/                    #   Redis 缓存管理
│   ├── query/                    #   统一查询门面（跨4个DB的 Client，含输入校验+哨兵错误）
│   ├── logger/                   #   日志
│   ├── smtp/                     #   邮件发送
│   ├── turnstile/                #   Cloudflare Turnstile 验证
│   ├── censor/                   #   百度内容审核客户端
│   ├── crypto/                   #   通用加密工具
│   ├── toolbox/                  #   Toolbox API 客户端
│   ├── command/                  #   指令加密工具
│   └── types/                    #   共享类型定义
│
├── Data/                         # ── 静态数据（不入 Git） ──
│   ├── master/                   #   5 区服的游戏 Masterdata（600+ JSON/区服）
│   ├── accounts/                 #   游戏账号凭据
│   └── structures/               #   结构定义
│
├── docs/                         # ── 文档 ──
├── integration/                  # ── 集成测试 ──
├── exports/                      # ── 导出产物/临时数据 ──
│
├── cmd/extractor/                #   数据提取脚本入口
├── cmd/migrate/                  #   Sekai Ent 迁移脚本入口
├── cmd/server/                   #   主服务入口
├── schema_info.json              #   Sekai 表结构元数据
├── go.mod / go.sum               #   Go 模块定义
└── haruki-cloud.example.yaml  #   配置文件模板
```

---

## 3. 配置系统

配置通过 `haruki-cloud.yaml` 加载，顶级结构如下：

```yaml
profile: "dev"             # 部署环境: production / beta / dev
                           # 影响 log_level / api_cache_ttl 默认值、recover stack trace 可见性
                           # production 强制关闭 allow_insecure_internal_api
                           # 可通过 HARUKI_PROFILE 环境变量覆盖

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
  local_masterdata: {}     # 本地 Masterdata 路径

sekai:                     # Sekai Masterdata 数据库
  db_url: "..."

chunithm:                  # CHUNITHM（两个独立数据库）
  music_db_url: "..."
  binding_db_url: "..."

haruki_bot_db:             # Bot 管理数据库
  db_url: "..."
  credential_sign_token: ""  # JWT 签名密钥（注册凭据）
  session_sign_token: ""     # JWT 签名密钥（会话令牌）
  session_ttl_days: 7
  smtp_host: ""            # 验证码邮件
  turnstile_secret_key: "" # Turnstile 人机验证

users_db:                  # 通用用户数据库（当前仅 DB 层保留，API 层未使用）
  db_url: "..."

toolbox:                   # Toolbox 外部服务
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

### 4.2 VerifyBotSession — Bot 客户端直调

```
适用路径：/api/v2/bot/:botId/*
请求头：
  - X-Haruki-Bot-Id       → Bot 数字 ID
  - X-Haruki-Bot-Session-Token → JWT 会话令牌
检查项：
  1. 两个头存在
  2. X-Haruki-Bot-Id == URL :botId
  3. JWT 签名有效 + 未过期
  4. JWT bot_id claim == X-Haruki-Bot-Id
  5. Redis (hdb:bot:session:<botId>) 中 token 一致
实现：api/bot_session_middleware.go
```

### 4.3 无鉴权 — 公开接口

```
适用路径：/api/v2/public/pjsk/alias/*,  /api/v2/public/chunithm/*,
          /bot/send-mail, /bot/register, /bot/:bot_id/auth
```

---

## 5. API 端点完整列表

### 5.1 Bot 会话管理（公开，无鉴权）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/bot/send-mail` | 发送 QQ 邮箱验证码 |
| POST | `/bot/register` | 注册 Bot 账号 |
| POST | `/bot/:bot_id/auth` | 登录（AES 加密凭据 → 返回 session_token） |

### 5.2 Bot 指令端点（VerifyBotSession 鉴权）

当前 Bot 端点由 `internal/pjsk/handler` registry 动态派生，标准协议如下：

1. `GET /api/v2/bot/:botId/command/manifests`
2. `POST /api/v2/bot/:botId/pjsk/<path>`

协议头示例：

X-Haruki-Bot-Id: 11451419
X-Haruki-Bot-Session-Token: <jwt>

其中：

1. Manifest 端点始终为 `GET + JSON`
2. PJSK Bot 业务端点为 `POST`
3. 请求体使用 `BotCommandRequest`
4. 当服务端配置了 `noise_private_key` 时，请求体为 `Noise IK + MsgPack(BotCommandRequest)`
5. 当服务端未配置 `noise_private_key` 时，退回 `JSON(BotCommandRequest)` 明文模式

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

### 5.6 Bot 内部端点（VerifyAPIAuthorization 鉴权）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/internal/bot/verify-session` | 验证 bot_id + session_token |
| POST | `/bot/statistics/record/:botID` | 统计数据上报 |

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
       Headers:
         X-Haruki-Bot-Id
         X-Haruki-Bot-Session-Token
       Body:
         Noise IK + MsgPack(BotCommandRequest)
         或 JSON(BotCommandRequest)
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

1. Bot 端点直接进入 `handler -> Execute -> render/userdata`
2. 渲染控制器仍然存在，但仅作为代码内部执行层
3. 图片命令最终通过 Drawing API + ImageCache 返回 OneBot11 `image` segment

### 6.3 Bot 注册/登录流程

```
1. POST /bot/send-mail  →  验证码 → QQ 邮箱
2. POST /bot/register   →  验证码验证 → 创建用户 → 返回 JWT credential
3. POST /bot/:bot_id/auth  →  AES 解密 → JWT credential 验证
                            → 生成 session_token → 存 Redis → 返回
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
├── global_resolver.go    # 兼容型全局解析器，供测试、调试和历史辅助逻辑使用
├── extractor.go          # Extractor：从文本中提取区服、角色、稀有度、属性、年份、uidArg 等
├── event_parser.go       # EventParser + EventQueryInfo
├── command_parser.go     # 其他命令解析辅助
├── utils.go              # isNumeric 等工具函数
└── parser_test.go        # 测试（聚焦 Extractor）
```

> 历史上曾存在 `parser.go` (`CardParser`/`CardQueryInfo`)，已在 R38 删除：功能由 `internal/pjsk/render/card/parser.go` 独立实现覆盖，原版本零外部调用方。

**核心概念：**
- `GlobalCommandResolver.Resolve(text)` 当前主要服务测试、调试与历史辅助逻辑，不是运行时 Bot 主协议入口
- `ResolvedCommand` 包含：Module, Mode, Query, Region, Params, IsHelp, IsVerbose, IsPreview
- parser 包同时向各 path 绑定的 handler 提供通用提取器和类型化解析能力
- `Extractor.ExtractUid` 当前支持 `u[i]`、游戏 UID、`@qq` 三类账号指定参数

### 8.2 handler — 指令处理 + Bridge

```
internal/pjsk/handler/
├── bridge.go             # Execute(ctx, resolved, app) → onebot11.Message + error
├── bot_route.go          # Bot route registry：聚合 module/path/commands/method
├── context.go            # Event, Context 接口, HandlerContext；从消息段提取文本和 at.qq 列表
├── handler.go            # Trie 注册、命令匹配、参数截取
├── profile_mode.go       # Profile 渲染模式常量
├── result.go             # bridge 内部辅助结果类型（当前主要供 executeProfile 等路径使用）
└── sekai/                # 各功能 handler
    ├── handler.go        # SekaiCommandHandler 注册；含 ParseUIDArg / uidArg 公共处理
    ├── helpers.go        # 工具函数
    ├── card.go ... vlive.go  # 各功能处理器
    └── handler_test.go
```

**Bridge 设计：** `bridge.go` 是指令解析与执行层之间的零开销桥梁，将 `ResolvedCommand` 直接路由到对应执行入口，并直接返回 `onebot11.Message`。图片执行器在 bridge 内完成缓存落盘和 `image` segment 封装，文本执行器直接返回 `text` segment，无 HTTP 往返。

`context.go` 当前只识别 OneBot `at` 段中的 `qq` 字段，不包含任何额外兼容字段。

### 8.3 render — 渲染子系统

```
internal/pjsk/render/
├── app/app.go            # App 结构体：包含 14 个 Controller 字段
├── region/region.go      # Value 类型（JP/CN/TW/EN/KR）
├── source/registry.go    # 数据源注册中心
├── masterdata/types.go   # Masterdata 类型定义
├── userdata/local.go     # 本地用户快照读取
├── common/               # 共享工具（卡图缩略图）
├── assets/               # 素材管理
│
│   ── 14 个功能模块（其中 vlive 为文本模块） ──
├── card/                 # 卡片（detail, list, box）— 11 文件
├── music/                # 曲目（detail, list, chart, progress, rewards）— 8 文件
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

### 8.4 chardata — 角色数据

```
internal/pjsk/chardata/
└── loader.go             # 从 Sekai DB 加载角色昵称映射，后台定时刷新
```

---

## 9. 已知问题 & 技术债

### ⚠ 结构问题

| 问题 | 位置 | 说明 |
|------|------|------|
| `internal/core/` 半空 | `internal/core/pjsk/`, `internal/core/middleware/` | 目录存在但无实际代码或为空 |
| `api/legacy/pjsk/` 历史兼容层 | `api/legacy/pjsk/` | 已于 2026-04-09 从仓库与运行时移除 |
| `exports/` 混合导出与临时产物 | `exports/` | 当前包含 alias 导出与 DB dump，尚未形成清晰约束与归档规则 |

### ⚠ 技术债

| 项目 | 说明 |
|------|------|
| 本地用户快照 | `render/userdata/local.go` 读取本地 JSON 文件（user.json, music_metas.json, mysekai.json），应迁移至 DB 驱动 |
| MySekai Masterdata | 依赖本地文件，未完全转为 DB 驱动 |
| Deck 引擎 | 简化版实现，原生 CGo 引擎未迁入 |
| Profile 扩展命令未完成 | `internal/pjsk/handler/sekai/profile.go` | 绑定/解绑/默认绑定已接入；`swap bind`、隐藏/展示抓包、隐藏/展示 ID、注册时间、服务状态、抓包模式仍为 disabled/TODO |

---

## 10. 构建 & 测试

```bash
# 构建主服务
go build ./cmd/server/...

# 运行全部测试
go test ./...

# 单独测试各子系统
go test ./api/public/...                     # 公开 API（pjsk alias, chunithm）
go test ./api/bot/...                        # Bot 端点（auth + pjsk）
go test ./internal/pjsk/parser/...          # 指令解析器
go test ./internal/pjsk/handler/sekai/...   # Handler 子系统
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
| `route.go` | Bot 注册/登录路由注册 | `/bot/send-mail`, `/bot/register`, `/bot/:bot_id/auth` |
| `handler.go` | 注册/登录/邮件 Handler | — |
| `struct.go` | 请求/响应结构体 | — |
| `helper.go` | JWT/AES 工具 | — |
| `statistics.go` | 统计上报 Handler | `/bot/statistics/record/:botID` |
| `verify.go` | 内部 session 验证 | `/internal/bot/verify-session` |
| `route_test.go` | Bot 认证流程集成测试 | — |

### api/bot/pjsk/（package pjsk）

| 文件 | 职责 | 关联路由 |
|------|------|----------|
| `handler.go` | `makeBotHandler`、MsgPack/JSON 请求解码、handler registry 派生路由注册、manifest 端点 | `/api/v2/bot/:botId/pjsk/*`, `/api/v2/bot/:botId/command/manifests` |
| `seed.go` | 从 handler registry 同步 command manifest 到 bot DB | — |
| `struct.go` | `BotCommandRequest`、`ManifestEntry`、`ManifestResponse` | — |
| `handler_test.go` | 覆盖 OneBot 解码、POST/Noise 往返、端点匹配、文本/图片返回、manifest 行为 | — |
| `testhelpers_test.go` | 测试辅助：`testRenderApp`、`renderEnvelope` | — |

### api/legacy/pjsk/（已移除）

该目录已于 2026-04-09 删除，仅保留历史文档记录。

---

## 12. 相关文档索引

| 文档 | 说明 |
|------|------|
| `docs/utils-query.cn.md` | `utils/query` 包说明（统一查询门面） |
| `docs/database-schemas.cn.md` | 数据库 Schema 详解（全 7 个 DB 模块） |
| `docs/pjsk-command-system.cn.md` | PJSK 指令解析 + 请求构建系统技术文档 |
| `docs/pjsk-profile-binding-implementation.cn.md` | PJSK 账号绑定与执行链路收口说明 |
| `docs/pjsk-vlive-text-plan.cn.md` | Virtual Live 文本链路实现说明 |
| `docs/README.cn.md` | 项目 README |
| `docs/service-test-merge-plan.cn.md` | Service-Test 合并方案 |
| `docs/service-test-merge-status.cn.md` | Service-Test 合并状态 |
| `docs/pjsk-user-snapshot-provider-design.cn.md` | 用户快照 Provider 设计 |
| `docs/zerobot-render-followup.cn.md` | ZeroBot 接入说明 |

---

**维护者**：Haruki-Cloud Team  
**文档版本**：v1.7  
**创建日期**：2026-03-23
