# 客户端构建许可与安全事件

本文描述 Cloud 侧的两项机制：

1. **构建许可与撤销**：AuthV3 登录时按发布清单放行客户端构建，并能紧急撤销某个
   build、版本、bot 凭据或来源地址；已签发的会话同样受撤销约束。
2. **安全事件与告警**：登录失败、重放、限速、构建被拒、登录来源突变、客户端身份
   突变等事件统一打点，超阈值时推送 webhook。

代码位置：`internal/core/buildpolicy`（策略文档与判定）、`internal/core/secevent`
（事件与告警）、`api/bot/auth/session_v3.go`（登录接入）、
`api/bot_session_middleware.go`（会话撤销）。

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

### 告警

```yaml
security:
  alert_webhook_url: https://example/hook
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

计数放在 Redis（`haruki:sec:<kind>:<subject>`），多实例共享。环境变量：
`HARUKI_SECURITY_ALERT_WEBHOOK_URL`、`HARUKI_SECURITY_ALERT_THRESHOLD`、
`HARUKI_SECURITY_ALERT_WINDOW`。

## 撤销手册

| 目标 | 操作 |
|------|------|
| 某个构建 | 对应条目加 `"revoked": true`，重新签名发布。 |
| 某个版本 | 加进 `revoked_versions`。 |
| 某个 bot 凭据 | 加进 `revoked_bots`；如需同时清掉现有会话，可再删 Redis 的 `hdb:bot:session:<bot_id>`。 |
| 某个来源 | 加进 `blocked_sources`。 |

发布后最多 30 秒生效，先在 `log-only` 下观察 `build_rejected` 的量再切 `enforce`。
