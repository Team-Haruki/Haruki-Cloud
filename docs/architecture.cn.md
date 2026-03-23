# Haruki-Cloud 项目架构文档

> 最后更新：2026-03-23（v1.2）

---

## 1. 项目概览

**Haruki-Cloud** 是 HarukiBot 生态的核心后端服务，负责：

- 为 Bot 提供 **指令解析 → 渲染 → 返回图片** 的完整链路
- 管理 Project SEKAI（プロジェクトセカイ）和 CHUNITHM 两个音游的查询数据
- 提供 Bot 注册/鉴权/会话管理

**技术栈：**

| 组件 | 技术 |
|------|------|
| HTTP 框架 | Fiber v3 |
| ORM | Ent (entgo.io) |
| 数据库 | PostgreSQL / MySQL / SQLite |
| 缓存 | Redis |
| 认证 | JWT (golang-jwt/v5) + AES-256-GCM |
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
│   ├── bot/                      #   Bot 专属端点
│   │   ├── auth/                 #     Bot 注册/登录/会话验证/统计
│   │   └── pjsk/                 #     Bot 指令端点（41个功能端点）→ /api/v2/bot/:botId/pjsk/*
│   └── legacy/                   #   ⚠ 内部渲染路由（发布前需删除）
│       └── pjsk/                 #     渲染分发 + 通用指令端点 → /internal/pjsk/*
│
├── internal/                     # ── 内部业务逻辑（不对外暴露） ──
│   ├── core/crypto/              #   Noise 协议加密工具
│   ├── middleware/secure/        #   安全中间件
│   └── pjsk/                     #   PJSK 核心子系统
│       ├── parser/               #     指令解析器（43 条正则路由）
│       ├── handler/              #     指令处理器 + Bridge 调用桥
│       ├── chardata/             #     角色昵称加载器
│       └── render/               #     渲染子系统（13 个模块）
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
├── exports/                      # ── 导出工具 ──
│
├── migrate.go                    # ⚠ 根目录迁移脚本（与 cmd/migrate 重复）
├── extract_tables.go             # ⚠ 根目录提取脚本（与 cmd/extractor 重复）
├── schema_info.json              #   Sekai 表结构元数据
├── go.mod / go.sum               #   Go 模块定义
└── haruki-db-configs.example.yaml  #   配置文件模板
```

---

## 3. 配置系统

配置通过 `haruki-db-configs.yaml` 加载，顶级结构如下：

```yaml
backend:                   # 服务基础配置
  host: "0.0.0.0"
  port: 3000
  accept_authorization: "" # 内部 API 鉴权令牌
  accept_user_agent: ""    # 内部 API User-Agent 过滤

redis:                     # Redis 连接
  addr: "localhost:6379"

pjsk:                      # PJSK 数据库
  db_url: "..."
  parser:                  # 指令解析器配置
    chardata_region: "jp"
    chardata_refresh_interval: "24h"

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
适用路径：/internal/pjsk/*, /internal/bot/*
检查项：
  - Authorization 头 == config.backend.accept_authorization
  - User-Agent 头包含 config.backend.accept_user_agent
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

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v2/bot/:botId/command/manifests` | **TODO 占位** — Command Manifest |
| GET/POST | `/api/v2/bot/:botId/pjsk/card/detail` | 卡面详情 |
| GET/POST | `/api/v2/bot/:botId/pjsk/card/list` | 查卡列表 |
| GET/POST | `/api/v2/bot/:botId/pjsk/card/box` | 查箱 |
| GET/POST | `/api/v2/bot/:botId/pjsk/gacha` | 卡池信息 |
| GET/POST | `/api/v2/bot/:botId/pjsk/music` | 歌曲详情 |
| GET/POST | `/api/v2/bot/:botId/pjsk/music/{list,chart,progress,rewards}` | 歌曲子功能 |
| GET/POST | `/api/v2/bot/:botId/pjsk/deck/{event,challenge,no-event,bonus,mysekai}` | 组卡推荐 |
| GET/POST | `/api/v2/bot/:botId/pjsk/event` | 活动详情 |
| GET/POST | `/api/v2/bot/:botId/pjsk/event/list` | 活动列表 |
| GET/POST | `/api/v2/bot/:botId/pjsk/education/{challenge,power,area,bonds,leader}` | 教育系统 |
| GET/POST | `/api/v2/bot/:botId/pjsk/score` | 分数查询 |
| GET/POST | `/api/v2/bot/:botId/pjsk/score/{custom-room,music-meta,music-board}` | 分数子功能 |
| GET/POST | `/api/v2/bot/:botId/pjsk/stamp` | 贴纸列表 |
| GET/POST | `/api/v2/bot/:botId/pjsk/misc/birthday` | 角色生日 |
| GET/POST | `/api/v2/bot/:botId/pjsk/sk/{line,query,check-room,speed,player-trace,rank-trace,winrate}` | SK 排名系统 |
| GET/POST | `/api/v2/bot/:botId/pjsk/mysekai/{resource,fixture-list,fixture-detail,door-upgrade,music-record,talk-list}` | MySekai |
| GET/POST | `/api/v2/bot/:botId/pjsk/profile` | 个人名片 |

共 **41 个功能端点 + 1 个 manifest 占位**。`command` 参数为 Base64 编码的 OneBot v11 JSON。

### 5.3 内部渲染端点（VerifyAPIAuthorization 鉴权）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/internal/pjsk/command` | 通用指令端点（纯文本 → 渲染 → PNG） |
| POST | `/internal/pjsk/render` | 统一渲染分发（指定 target + operation + payload） |
| POST | `/internal/pjsk/<module>/<action>/build` | 模块化构建（返回 JSON payload） |
| POST | `/internal/pjsk/<module>/<action>/render` | 模块化渲染（返回 PNG） |

模块列表：card, deck, event, gacha, honor, profile, misc, mysekai, music, education, sk, score, stamp

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

### 6.1 Bot 指令渲染流程

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
       Headers: X-Haruki-Bot-Id, X-Haruki-Bot-Session-Token
       Body: {"command": "<base64 OneBot JSON>"}
       │
       ▼ VerifyBotSession middleware
       │
       ▼ decodeCommand()  ←── Base64 解码 → OneBot JSON → 提取文本
       │
       ▼ GlobalCommandResolver.Resolve("/卡面 1001")
       │  → ResolvedCommand{Module:Card, Mode:"card-detail", Query:"1001"}
       │
       ▼ 验证 Module+Mode 与端点匹配
       │
       ▼ handler.Execute(ctx, resolved, renderApp)  [bridge.go]
       │  → renderApp.Cards.HandleDetail(region, query)
       │
       ▼ Drawing API (外部服务)
       │  → POST http://drawing-service/render  {payload}
       │
       ▼ PNG 字节流
       │
       ▼ 200 OK, Content-Type: image/png
```

### 6.2 内部渲染流程

```
内部服务 (如 ZeroBot)
  │
  └─ POST /internal/pjsk/<module>/<action>/render
       Headers: Authorization, User-Agent
       Body: {region, query, params...}
       │
       ▼ VerifyAPIAuthorization middleware
       │
       ▼ Controller.Handle*(region, query)
       │
       ▼ Drawing API → PNG
```

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
├── global_resolver.go    # GlobalCommandResolver：43 条正则路由，指令→模块+模式
├── extractor.go          # Extractor：从文本中提取角色/稀有度/属性/技能/区服/年份
├── parser.go             # CardParser + CardQueryInfo
├── music_parser.go       # MusicParser + MusicQueryInfo
├── event_parser.go       # EventParser + EventQueryInfo
├── command_parser.go     # CommandParser（SK/bind 命令）
├── utils.go              # isNumeric 等工具函数
└── parser_test.go        # 测试
```

**核心概念：**
- `GlobalCommandResolver.Resolve(text)` → `*ResolvedCommand`
- `ResolvedCommand` 包含：Module, Mode, Query, Region, Params, IsHelp, IsVerbose, IsPreview
- 支持 13 个模块（Card, Gacha, Music, Deck, Event, Education, Score, Stamp, Misc, SK, MySekai, Profile, Help）

### 8.2 handler — 指令处理 + Bridge

```
internal/pjsk/handler/
├── bridge.go             # Execute(ctx, resolved, app) → []byte — 核心桥接
├── context.go            # Event, Context 接口, HandlerContext
├── handler.go            # Trie 树指令分发器
└── sekai/                # 15 个功能 handler 文件
    ├── handler.go        # SekaiCommandHandler，反射自动注册
    ├── helpers.go        # 工具函数
    ├── card.go ... vlive.go  # 各功能处理器
    └── handler_test.go
```

**Bridge 设计：** `bridge.go` 是指令解析和渲染之间的零开销桥梁，将 `ResolvedCommand` 直接路由到对应的 render Controller 方法，无 HTTP 往返。

### 8.3 render — 渲染子系统

```
internal/pjsk/render/
├── app/app.go            # App 结构体：包含 13 个 Controller 字段
├── region/region.go      # Value 类型（JP/CN/TW/EN/KR）
├── source/registry.go    # 数据源注册中心
├── masterdata/types.go   # Masterdata 类型定义
├── userdata/local.go     # 本地用户快照读取
├── common/               # 共享工具（卡图缩略图）
├── assets/               # 素材管理
│
│   ── 13 个渲染模块 ──
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
└── mysekai/              # MySekai（资源, 家具, 大门, 唱片, 对话）
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
| 根目录存在独立 main 文件 | `migrate.go`, `extract_tables.go` | 与 `cmd/migrate/`, `cmd/extractor/` 功能重复，导致 `go build ./...` 失败（多个 main） |
| `internal/core/` 半空 | `internal/core/pjsk/`, `internal/core/middleware/` | 目录存在但无实际代码或为空 |
| `cmd/client_test/` 空目录 | `cmd/client_test/` | 未使用 |
| `api/legacy/pjsk/` 待删除 | `api/legacy/pjsk/` | 发布前需删除，Bot 客户端切换到 `/api/v2/bot/` 后即可移除 |
| `exports/` 用途不明 | `exports/` | 目录存在但未调查内容 |

### ⚠ 技术债

| 项目 | 说明 |
|------|------|
| 本地用户快照 | `render/userdata/local.go` 读取本地 JSON 文件（user.json, music_metas.json, mysekai.json），应迁移至 DB 驱动 |
| MySekai Masterdata | 依赖本地文件，未完全转为 DB 驱动 |
| Deck 引擎 | 简化版实现，原生 CGo 引擎未迁入 |
| `sekai/profile.go` 绑定指令 | bind/unbind/setMain 全为 TODO stub，暂不可用 |

---

## 10. 构建 & 测试

```bash
# 构建（必须指定 cmd/server，根目录有多个 main 会冲突）
go build ./cmd/server/...

# 运行全部测试
go test ./api/... ./internal/pjsk/...

# 单独测试各子系统
go test ./api/public/...                     # 公开 API（pjsk alias, chunithm）
go test ./api/bot/...                        # Bot 端点（auth + pjsk）
go test ./api/legacy/pjsk/...               # 内部渲染路由
go test ./internal/pjsk/parser/...          # 指令解析器
go test ./internal/pjsk/handler/sekai/...   # Handler 子系统
go test ./internal/pjsk/render/...          # 渲染子系统
```

> 注意：`api/bot/auth/` 的测试依赖数据库连接，在无 DB 环境下可能失败（这是预期行为）。

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
| `route_table.go` | 41 条路由配置表（module、mode、path、prefixes） | — |
| `handler.go` | `makeBotHandler`、Base64+OneBot 解码、`RegisterPJSKBotRoutes`、manifest 占位 | `/api/v2/bot/:botId/pjsk/*` |
| `struct.go` | `BotCommandRequest`、`ManifestEntry`、`ManifestResponse` | — |
| `handler_test.go` | 11 个测试（OneBot 解码、纯文本降级、GET/POST、端点匹配、manifest） | — |
| `testhelpers_test.go` | 测试辅助：`testRenderApp`、`testResolver`、`renderEnvelope` | — |

### api/legacy/pjsk/（package pjsk，⚠ 待删除）

| 文件 | 职责 | 关联路由 |
|------|------|----------|
| `render_route.go` | 渲染路由注册（13 模块 × build/render） | `/internal/pjsk/<module>/*` |
| `render_dispatch.go` | 统一渲染分发 Handler | `/internal/pjsk/render` |
| `render_struct.go` | 渲染请求/响应结构体 | — |
| `render_route_test.go` | 渲染路由测试 + `testRenderApp` | — |
| `render_dispatch_test.go` | 分发测试 | — |
| `command.go` | 通用指令端点 Handler | `/internal/pjsk/command` |
| `command_struct.go` | 指令请求/响应结构体 | — |
| `command_test.go` | 通用指令测试（5 个） | — |

---

## 12. 相关文档索引

| 文档 | 说明 |
|------|------|
| `docs/utils-query.cn.md` | `utils/query` 包说明（统一查询门面） |
| `docs/database-schemas.cn.md` | 数据库 Schema 详解（全 7 个 DB 模块） |
| `docs/pjsk-command-system.cn.md` | PJSK 指令解析 + 请求构建系统技术文档 |
| `docs/README.cn.md` | 项目 README |
| `docs/service-test-merge-plan.cn.md` | Service-Test 合并方案 |
| `docs/service-test-merge-status.cn.md` | Service-Test 合并状态 |
| `docs/pjsk-user-snapshot-provider-design.cn.md` | 用户快照 Provider 设计 |
| `docs/zerobot-render-followup.cn.md` | ZeroBot 接入说明 |

---

**维护者**：Haruki-Cloud Team  
**文档版本**：v1.1  
**创建日期**：2026-03-23
