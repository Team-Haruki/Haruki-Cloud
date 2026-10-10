# 客户端构建许可与安全事件

本文描述 Cloud 侧的三项机制：

1. **构建许可与撤销**：AuthV3 登录时按发布清单放行客户端构建，并能紧急撤销某个
   build、版本、bot 凭据或来源地址；已签发的会话同样受撤销约束。
2. **安全事件与告警**：登录失败、重放、限速、构建被拒、登录来源突变、客户端身份
   突变等事件统一打点，超阈值时推送 webhook。
3. **登录失败自动封禁**：同一来源地址登录失败次数过多时，自动禁止它登录一段时间。

代码位置：`internal/core/buildpolicy`（策略文档与判定）、`internal/core/secevent`
（事件与告警）、`internal/core/authban`（登录失败自动封禁）、
`api/bot/auth/session_v3.go`（登录接入）、`api/bot_session_middleware.go`（会话撤销）。

## 前提与边界

`build_id`、`client_version`、`target`、`binary_sha256` 都是客户端**自报**的。没有
TPM / TEE，这不是远程证明：改过的二进制可以伪造任何一个值。这套机制的作用是把
"改完重新分发的旧版客户端"的成本从"去掉一处检查"抬高到"持续伪造一个仍在许可窗口内
的活身份"，并且运维一发布新清单就能让被撤销的身份立刻失效。它必须与短会话
（`auth_v3_session_ttl`，默认 1h）和异常检测配合使用，不能单独依赖。

## 策略文件

```json
{
  "version": 3,
  "issued_at": 1756800000,
  "expires_at": 1759400000,
  "builds": [
    {
      "build_id": "20260902-1a2b3c",
      "version": "3.1.0",
      "target": "linux-amd64",
      "sha256": "…64 hex…",
      "not_before": 1756800000,
      "not_after": 0
    },
    { "build_id": "20260815-9f8e7d", "version": "3.0.0", "revoked": true, "reason": "leaked build" }
  ],
  "revoked_versions": ["2.*", "3.0.1"],
  "revoked_bots": ["30042042"],
  "blocked_sources": ["203.0.113.7", "198.51.100.0/24"]
}
```

| 字段 | 说明 |
|------|------|
| `version` | 每次发布递增，正整数。 |
| `issued_at` / `expires_at` | Unix 秒，可选。过期文档视为"不可用"，登录放行并记录 `policy_unavailable`，绝不会变成"拒绝所有人"。 |
| `builds[].build_id` | 客户端在 AuthV3 里上报的 `build_id`，全局唯一。 |
| `builds[].version` | 该构建发布时的版本；同一 `build_id` 报了别的版本会被拒绝。 |
| `builds[].target` / `sha256` | 可选；仅当客户端也上报 `target` / `binary_sha256` 时比对。 |
| `builds[].not_before` / `not_after` | 许可窗口，0 表示不限制。 |
| `builds[].revoked` | 立即撤销该构建，包括已签发的会话。 |
| `revoked_versions` | 精确版本或前缀加 `*`。 |
| `revoked_bots` | 撤销 bot 凭据：登录拒绝、活动会话拒绝。 |
| `blocked_sources` | 登录来源 IP 或 CIDR。 |

判定顺序：bot 撤销 → 来源封禁 → 版本撤销 → `build_id` 缺失 / 未登记 → 构建撤销 →
版本不符 → 许可窗口 → target → sha256。所有拒绝对客户端只返回同一句
`客户端未获授权`（403），具体原因只进日志。

## 模式与配置

```yaml
haruki_bot:
  build_policy_path: /config/trust/build-policy.signed.json
  build_policy_mode: log-only        # off | log-only | enforce
  build_policy_root_public_key: ""   # 设置后要求文件为 trust-signer 的 release 签名信封
```

| 模式 | 行为 |
|------|------|
| `off` | 不读取策略，`build_id` 只记录。生产环境会 warn。 |
| `log-only` | 判定并记录 `build_rejected` 事件（`enforced=false`），登录照常放行。配置了路径但没写模式时的默认值。 |
| `enforce` | 拒绝登录，并在会话中间件里拒绝已撤销身份的会话。 |

文件按 mtime 热加载，最多每 30 秒检查一次，改完不用重启。文件缺失或解析失败时
**放行**并记录 `policy_unavailable`，启动日志也会提示。

环境变量：`HARUKI_BOT_BUILD_POLICY_PATH`、`HARUKI_BOT_BUILD_POLICY_MODE`、
`HARUKI_BOT_BUILD_POLICY_ROOT_PUBLIC_KEY`。

## 签名发布

策略文件可以用离线根密钥签名，签名域为 `haruki-cloud/release/v1`：

```bash
trust-signer sign --key root.seed --key-id root-2026-09 --domain release \
  --in build-policy.json --out build-policy.signed.json
trust-signer verify --public <root-pub-hex> --in build-policy.signed.json --domain release
```

`sign` 会先按文档规则校验，非法文档不会生成信封。Cloud 配置了
`build_policy_root_public_key` 后只接受验签通过的信封；未配置时接受裸 JSON，也接受
未验签的信封（此时安全性取决于主机文件系统）。

## 会话内撤销

AuthV3 签发的 session JWT 带 `bid`（build_id）和 `cv`（client_version）声明。每个
`/pjsk` 请求和 manifest 请求在会话校验通过后再查一次策略中的撤销项：bot 撤销、版本
撤销、构建撤销或已过 `not_after`。命中时返回 403
`会话已被撤销，请更新客户端后重新登录`，并记录 `session_revoked`。未登记的构建不在
会话阶段拒绝，因为它当初是在当时的模式下被放行的。

## 安全事件

所有事件以 `event=security` 打日志，字段：`kind`、`bot_id`、`build_id`、
`client_version`、`source_ip`、`reason`、`enforced`。

| kind | 触发点 |
|------|--------|
| `auth_failed` | 凭据错误、载荷不合法、bot_id 不符等。 |
| `replay_detected` | 登录 nonce 重复，或命令请求 nonce 重复。 |
| `rate_limited` | 触发每 bot 每分钟 10 次的登录限速。 |
| `build_rejected` | 构建策略判定失败；`enforced` 区分是否真的拒绝。 |
| `session_revoked` | 活动会话被策略撤销。 |
| `login_source_changed` | 登录成功，但来源 IP 与上次不同。 |
| `client_changed` | 登录成功，但 `client_version` 或 `build_id` 与上次不同。 |
| `policy_unavailable` | 策略文件读不到或已过期，登录被放行。 |
| `ip_banned` | 来源地址因登录失败过多被封禁（见下文“登录失败自动封禁”）。每次封禁都告警，不走阈值。 |

### 告警

```yaml
security:
  alert_webhook_url: https://alerts.example.com/internal/bot-security/alerts
  alert_webhook_token: change-me   # 可选；非空时带 Authorization: Bearer <token>
  alert_threshold: 5
  alert_window: 10m
```

同一 `kind` 对同一主体（有 `bot_id` 用 bot_id，否则用来源 IP）在窗口内累计到阈值时，
记一条 ERROR `security alert` 并向 webhook POST 一次 JSON，窗口内不重复：

```json
{"kind":"auth_failed","bot_id":"30042042","build_id":"…","client_version":"…",
 "source_ip":"…","reason":"…","enforced":true,"count":5,"threshold":5,
 "window_seconds":600,"node":"cn06","time":"2026-09-02T09:00:00Z"}
```

字段名、类型和 omitempty 规则由 `secevent_test.go` 的 `TestAlertPayloadShape` 固定，
接收端按这个形状实现，改动须两边同步。`bot_id`、`build_id`、`client_version`、
`source_ip`、`reason`、`node` 为空时省略，其余字段总是存在。

`ip_banned` 告警不经计数，每次封禁发一次：`source_ip` 是被封的地址（IPv6 为 /64），
没有 `bot_id`；`count`、`threshold`、`window_seconds` 是触发封禁的失败次数、封禁阈值和
计数窗口；另带 `ban_seconds`（本次封禁时长）和 `bot_ids`（窗口内出现过的 bot，最多 20 个）。
接收端忽略不认识的字段，所以 `reason` 里也用文字写了这两项：

```json
{"kind":"ip_banned","source_ip":"203.0.113.50",
 "reason":"10 login failures in 10m0s; banned for 6h0m0s (ban 1 within 168h0m0s); bots 30042042",
 "enforced":true,"count":10,"threshold":10,"window_seconds":600,"node":"node-a",
 "time":"2026-10-10T09:00:00Z","ban_seconds":21600,"bot_ids":["30042042"]}
```

投递规则：

- 请求头 `Content-Type: application/json`；配置了 `alert_webhook_token` 时再带
  `Authorization: Bearer <token>`，未配置则不发该头。token 不会写进任何日志。
- 超时 5 秒。
- 不跟随重定向：3xx 和其他非 2xx 一样算失败，token 只会发给配置的那个地址。
- 失败记一条 WARN `security alert webhook failed`：非 2xx 只带 `status`（状态码），
  不记录响应体；连接类错误只带 `error_type`。失败不重试，该窗口内也不会再发。

计数放在 Redis（`haruki:sec:<kind>:<subject>`），多实例共享。环境变量：
`HARUKI_SECURITY_ALERT_WEBHOOK_URL`、`HARUKI_SECURITY_ALERT_WEBHOOK_TOKEN`、
`HARUKI_SECURITY_ALERT_THRESHOLD`、`HARUKI_SECURITY_ALERT_WINDOW`。

## 登录失败自动封禁

同一来源地址在窗口内登录失败达到阈值，就禁止它在 AuthV3 登录接口
（`POST /api/v3/bot/:bot_id/auth`）登录一段时间。

### 默认值

```yaml
security:
  auth_ip_ban:
    enabled: false             # 默认关闭，设为 true 才启用
    threshold: 10              # 窗口内计入的失败次数达到这个值就封禁
    window: 10m                # 固定窗口，从第一次失败开始
    ban_duration: 6h           # 第一次封禁时长
    max_ban_duration: 24h      # 重复封禁逐次翻倍，最长到这个值；不大于 ban_duration 时不升级
    escalation_window: 168h    # 最近一次封禁后，封禁次数记多久
    count_build_rejected: true # 构建策略拒绝（enforce）也计入
    exempt_known_bots: true    # 近期从该地址登录成功过的 bot 不受封禁影响
    known_bot_ttl: 168h
    block_bot_routes: false    # 封禁是否同时拦截其他 bot 接口
    never_ban_cidrs: []        # 额外的永不封禁地址 / CIDR
```

环境变量：`HARUKI_SECURITY_AUTH_IP_BAN_` 加大写字段名（`ENABLED`、`THRESHOLD`、`WINDOW`、
`DURATION`、`MAX_DURATION`、`ESCALATION_WINDOW`、`COUNT_BUILD_REJECTED`、
`EXEMPT_KNOWN_BOTS`、`KNOWN_BOT_TTL`、`BLOCK_BOT_ROUTES`、`NEVER_BAN_CIDRS`，后者逗号分隔）。
默认关闭（`enabled` 未设置时为 false），需要显式设为 `true`；开启后没有 Redis 时也不启用。

### 计入哪些失败

| 计入 | 不计入 |
|------|--------|
| 凭据错误、bot 不存在、`bot_id` 不是数字或不一致（400 `auth_failed`） | 每 bot 每分钟 10 次的登录限速（429） |
| 载荷无法解析、请求上下文不符、时间戳过期、nonce 格式不对（400） | bot 所有者被全局封禁（403） |
| 登录 nonce 重放（400 `replay_detected`） | 服务端错误（5xx） |
| 空请求体或无法解开的 Noise 握手（400） | 构建策略 `log-only` 下的报告（登录照常放行） |
| 构建策略拒绝（403，`count_build_rejected` 为 true 时） | 指令接口的请求 nonce 重放 |

登录成功**不会**清零该地址的失败次数：计数属于地址，如果一次成功登录就能清零，
攻击者用自己的有效凭据就能不断洗掉对其他 bot 的失败记录。成功登录只把这个 bot 记为
该地址的“已知 bot”（`exempt_known_bots`）。

### 封禁范围

- 被封地址请求登录接口时，在 Noise 握手、数据库和 bcrypt 之前直接返回 429，带
  `Retry-After`（秒），正文是目录消息 `account.api.auth_banned`（“认证失败次数过多，
  请稍后再试”，明文，不回显地址）。客户端对非 200 只看状态码。封禁期间的请求不再计数。
- 已签发的会话不受影响：默认只拦登录接口，指令、manifest、注销照常工作，因为一个出口
  地址上常有多个 bot。`block_bot_routes: true` 时，`/api/v2/bot/<bot_id>/…` 和
  `/api/v3/bot/<bot_id>/…` 的其他接口也对被封地址返回 429。
- `exempt_known_bots`：最近 `known_bot_ttl` 内从该地址登录成功、之后没有再从该地址失败过
  的 bot，在封禁期间仍可登录（按 URL 里的 `bot_id` 判断，凭据照常校验）。这个 bot 一旦从该
  地址失败，就失去豁免。
- IPv6 按 /64 计数和封禁。
- 永不封禁：回环、私有地址、链路本地、`100.64.0.0/10`、`0.0.0.0/8`，以及
  `never_ban_cidrs` 里的地址。封这些地址等于封掉代理或内部调用方后面的所有人。
  如果 API 也经 CDN 提供、且 CDN 回源时没有可信的转发头，Cloud 看到的是 CDN 节点地址，
  应把 CDN 的回源地址段加进 `never_ban_cidrs`。

### 来源地址

使用 `c.IP()`，与安全事件的 `source_ip` 相同：只有 TCP 对端在
`backend.trusted_proxies` 里时才读 `backend.proxy_header`（默认 `X-Forwarded-For`），
并且从右往左跳过可信代理，取第一个不是可信代理的合法地址（Fiber
`EnableIPValidation`）。客户端自己写在该头最左边的值不会被采用。

### 存储

计数和封禁在 Redis（多实例共享，重启不丢），每次失败由一段 Lua 脚本原子完成计数、
判阈值、计算时长和写入封禁，并发请求不会越过阈值或重复封禁：

| 键 | 内容 |
|----|------|
| `haruki:authban:fail:<ip>` | 当前窗口的失败次数，TTL 为窗口剩余时间 |
| `haruki:authban:bots:<ip>` | 当前窗口出现过的 bot_id（最多 20 个） |
| `haruki:authban:ban:<ip>` | 封禁（hash：since、until、level、count、bots），TTL 为封禁剩余时间 |
| `haruki:authban:level:<ip>` | 封禁次数，用于升级，TTL 为 `escalation_window` |
| `haruki:authban:known:<ip>` | 已知 bot，TTL 为 `known_bot_ttl` |
| `haruki:authban:active` | 有序集合，成员为地址，分值为到期时间（毫秒），供列表和到期日志使用 |

Redis 出错时放行（记 WARN `auth ip ban store unavailable`）。

### 日志与指标

- 开始：WARN `auth ip ban started`（`source_ip`、`failures`、`window`、`ban_duration`、
  `level`、`until`、`last_reason`、`bot_ids`），同时上报 `ip_banned` 安全事件。
- 到期：每分钟检查一次，INFO `auth ip ban expired`（多实例只记一次）。
- 手动解封：WARN `auth ip ban lifted`。
- `/debug/vars` 的 `auth_ip_ban`：`failures_counted`、`bans`、`requests_rejected`、
  `known_bot_exempted`、`bans_expired`、`bans_lifted`、`redis_errors`。

### 管理接口

`VerifyAPIAuthorization` 鉴权（同 `/internal/bot/*`），只在功能启用时注册：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/internal/bot/auth-bans` | 当前封禁列表（最多 500 条，按到期时间排序） |
| GET | `/internal/bot/auth-bans?ip=<地址>` | 该地址的全部状态：封禁、窗口内失败次数和 bot、升级次数、已知 bot、是否永不封禁 |
| DELETE | `/internal/bot/auth-bans?ip=<地址>` | 解封并清空失败次数、窗口 bot 和升级次数（保留已知 bot） |

`ip` 可以是地址或 IPv6 /64；IPv6 地址按所在 /64 处理。列表项：

```json
{"ip":"203.0.113.50","since":"2026-10-10T09:00:00Z","until":"2026-10-10T15:00:00Z",
 "retry_after_seconds":21000,"level":1,"failures":10,"bot_ids":["30042042"]}
```

## 撤销手册

| 目标 | 操作 |
|------|------|
| 某个构建 | 对应条目加 `"revoked": true`，重新签名发布。 |
| 某个版本 | 加进 `revoked_versions`。 |
| 某个 bot 凭据 | 加进 `revoked_bots`；如需同时清掉现有会话，可再删 Redis 的 `hdb:bot:session:<bot_id>`。 |
| 某个来源 | 加进 `blocked_sources`。 |

发布后最多 30 秒生效，先在 `log-only` 下观察 `build_rejected` 的量再切 `enforce`。
