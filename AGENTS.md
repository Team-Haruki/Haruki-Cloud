# Haruki Cloud — Agent Guidelines

This file is the canonical onboarding document for any AI assistant working on
the Haruki-Cloud repository (Codex, Claude, Copilot, etc.) and the single source
of truth for agent guidance. [`CLAUDE.md`](CLAUDE.md) and
[`.github/copilot-instructions.md`](.github/copilot-instructions.md) only point
here; edit this file instead of copying text into them.

For deeper architecture background, the authoritative human-facing references
are in [`docs/`](docs/). The most important entry points are listed in
[Authoritative documents](#8-authoritative-documents) below.

---

## 1. Project at a glance

Haruki-Cloud is the core backend of the **HarukiBot** ecosystem. It serves:

- the bot command pipeline (parse → execute → OneBot11 message)
- Project SEKAI (PJSK) and CHUNITHM query/render data
- bot registration / auth / session management

| Component      | Tech                                         |
|----------------|----------------------------------------------|
| HTTP framework | Fiber v3                                     |
| ORM            | Ent (entgo.io)                               |
| Databases      | PostgreSQL / MySQL / SQLite                  |
| Cache          | Redis                                        |
| Auth           | JWT (golang-jwt/v5) + Noise NK (AuthV3) |
| JSON           | `encoding/json/v2` via `internal/jsonutil`   |
| Go             | 1.27                                         |

There is **one** runtime entry point: `main.go` at the repo root, which only
sets up signal handling and calls `server.Run(ctx)` from
`internal/server/`. Auxiliary CLIs live under `cmd/` (`importer`, `extractor`,
`trust-signer`, `asset-index`, `image-cache-reconcile`; see §5).

---

## 2. Repository layout

```
Haruki-Cloud/
├── main.go                 # server entry point; calls internal/server/.Run
├── internal/server/        # bootstrap: init_*.go, fiber.go, run.go
├── cmd/
│   ├── importer/           # legacy export → new DB importer CLI
│   ├── extractor/          # schema extractor utility
│   ├── trust-signer/       # offline Ed25519 keyset signer (never run on the Cloud host)
│   ├── asset-index/        # manual resource-catalog bootstrap / publication
│   └── image-cache-reconcile/ # render-cache index vs. object store check (--repair)
│
├── api/
│   ├── helper.go           # shared response helpers, VerifyAPIAuthorization
│   ├── struct.go           # shared response/error types
│   ├── bot_session_middleware.go
│   ├── public/             # unauthenticated endpoints (/api/v2/public/...)
│   │   ├── pjsk/           # alias query
│   │   └── chunithm/       # alias / song lookup
│   ├── bot/
│   │   ├── auth/           # bot registration / login / session / stats
│   │   └── pjsk/           # bot command endpoints (handler-registry driven)
│   ├── groupguard/
│   └── trust/              # GET /api/v3/trust/keyset (serves the offline-signed keyset)
│
├── internal/
│   ├── cachepersist/       # bounded background writer for rebuildable cache snapshots
│   ├── cluster/            # node role / read-only mode helpers (config.Cfg.Node)
│   ├── core/buildpolicy/   # AuthV3 client build allowlist / revocations (docs/build-policy.cn.md)
│   ├── core/crypto/        # Noise protocol helpers
│   ├── core/dbpool/        # database/sql pool sizing / recycling
│   ├── core/secevent/      # security event funnel + threshold alerts
│   ├── core/trustsign/     # detached-payload Ed25519 signing contract (shared with Haruki-Client)
│   ├── core/upstream/      # upstream connection pool / transport
│   ├── core/urlhost/       # picks one public base URL out of per-node hosts
│   ├── handler/            # cross-domain command registry / bot routing
│   ├── httpcoding/         # zstd content coding with internal services (deck-service, Drawing)
│   ├── i18n/               # ALL user-facing copy: TOML catalogs + help docs under locales/<locale>/ (see §12)
│   ├── identity/           # platform user → haruki user resolution
│   ├── jsonutil/           # JSON facade: encoding/json/v2 engine, v1-compatible semantics
│   ├── middleware/secure/  # security middleware
│   ├── observability/commandtrace/ # command execution tracing
│   ├── observability/upstreamcall/ # per-upstream-call timing (Drawing, deck)
│   ├── onebot11/           # OneBot11 message helpers (was internal/pjsk/onebot11/)
│   ├── pjsk/               # PJSK subsystem (see below)
│   ├── server/             # bootstrap (see above)
│   ├── storage/            # Store abstraction: local filesystem and S3-compatible (storage/s3) backends
│   └── testutil/           # shared test assertions / clock helpers
│
├── config/                 # YAML config loader + timeouts
├── database/               # ent-generated DB clients (bot/censor/chunithm/{maindb,music}/pjsk/sekai/users)
├── ent/                    # ent schema definitions (mirror of database/)
├── docs/                   # canonical human-facing documentation
├── exports/                # legacy JSON snapshots for the importer (local only, not in git)
├── scripts/                # ops helpers (provision_bot, prepare-release.sh, ...)
├── deploy/                 # failover / secondary-node scripts and compose files
├── integration/            # integration tests (gated behind HARUKI_RUN_INTEGRATION)
└── Dockerfile / docker-compose.yml
```

`internal/pjsk/` itself contains:

| Sub-package       | Role                                                                |
|-------------------|---------------------------------------------------------------------|
| `accountdata/`    | User binding / profile settings (NOT to be confused with snapshots) |
| `alias/`          | Alias service (review queue, validation, records)                   |
| `chartstyle/`     | Chart rendering style helpers                                       |
| `displaytime/`    | Time/region display helpers                                         |
| `drawing/`        | Image rendering helpers (`ProfileBgSettings`, etc.)                 |
| `eventutil/`      | Event window / window-aligned helpers                               |
| `filteralias/`    | Attribute / filter keyword alias tables                             |
| `handler/`        | Bot command parsing + execution dispatch (NOT the upstream client)  |
| `meta/`           | Static meta tables                                                  |
| `parser/`         | Free-text command parsers (card parser lives in `render/card`)      |
| `region/`         | Region normalisation                                                |
| `render/`         | Render runtime (controllers, providers, snapshots)                  |
| `requestbuilder/` | Internal request builders for the render layer                      |
| `sekai/`          | **Upstream Sekai HTTP client** — always import as alias `sekaiapi`  |
| `subscription/`   | Subscription pushes (e.g. MySekai birthday)                         |

`internal/pjsk/render/` is the bulk of the runtime:

```
render/
├── app/         # the App composition root (see §3)
├── assetindex/  # immutable per-region asset inventories (publish / consume)
├── assets/      # asset providers
├── cachefill/   # per-key singleflight + failure backoff for DB-backed master data caches
├── card/        # card lookup / parser / detail / list
├── common/      # shared render helpers
├── costume/     # 3D costume / preview
├── deck/        # deck recommend (challenge / event / WL)
├── education/   # leader / bonds / area
├── event/       # event metadata, ranks
├── gacha/       # gacha details
├── honor/       # honor logic
├── inventory/   # inventory categories / lookup
├── masterdata/  # master data adapters
├── misc/        # miscellaneous (e.g. birthday)
├── music/       # music detail / list / progress / rewards
├── mysekai/     # MySekai data
├── playerframe/ # equipped player frames + operator overrides (docs/player-frame-overrides.md)
├── profile/     # user profile rendering
├── provider/    # request-scoped DB providers (db_*.go)
├── releasecheck/# release window checks
├── score/       # score board
├── sk/          # SK ranking / forecast / trace
├── snapshot/    # **Snapshot** abstraction (was render/userdata)
├── source/      # request-scoped sources
├── stamp/       # stamp lookup
└── vlive/       # virtual live
```

---

## 3. Architecture: composition root

`internal/pjsk/render/app.App` is the **single composition root** of the render
runtime. It holds every controller, every external client, the DB client, the
Redis client, and runtime config.

- `internal/server/init_services.go` translates each `config.Cfg.*` section into
  a `renderapp.Config` and constructs the `App` via
  `renderapp.New(sekaiClient, pjskClient, renderapp.Config{...})`.
- Handlers receive an `*App` (typically as `rc.App`) and must access shared
  dependencies via fields on it. **Do not introduce package-level singletons.**
- Tests can construct `&renderapp.App{...}` literals and only set the fields
  they need; the unset clients are nil-safe.

### Sekai / Toolbox / Tracker clients

- Constructors:
    - `sekaiapi.NewSekaiAPIClient(*config.SekaiAPIConfig)`
    - `sekaiapi.NewToolboxClient(*config.ToolboxConfig)`
    - `sekaiapi.NewTrackerClient(*config.TrackerConfig)`
- All methods are **nil-receiver safe**; nil clients return
  `ErrClientNotConfigured` (defined in `internal/pjsk/sekai/errors.go`).
- When writing tests that exercise a real Sekai HTTP server
  (`httptest.NewServer` + `config.Cfg.SekaiAPI.BaseURL`), remember to populate
  `SekaiAPI: sekaiapi.NewSekaiAPIClient(&config.Cfg.SekaiAPI)` in the App
  literal — see `TestExecuteMysekaiPhoto` and
  `TestBuildPublicMusicProfilesUsesSelectorFromRequestParams` for reference.

### Snapshot pipeline (PJSK)

- The production resolution path is **Toolbox → local static (debug fallback,
  only when `AllowFallback=true`)**. Production never falls back to local.
- Cloud no longer mirrors snapshots — there is no `pjsk_user_snapshots` table
  and no `snapshot/upload` route. **Toolbox is the source of truth.**
- Handlers must consume snapshots via `ResolveSnapshot` → controller; do not
  call `GetPrivateDataValue(...)` for single-key lookups in handlers.

### Context plumbing

- The main render chain has been freed of `context.Background()`. Always pass
  `ctx` (typically `rc.Ctx`) through. New code must follow the same rule.
- DB providers in `render/provider/db_*.go` already support per-request source
  cloning. Keep the pattern when adding new providers.

### Naming pitfalls

| Looks similar but…                                                                                                                                    |
|-------------------------------------------------------------------------------------------------------------------------------------------------------|
| `internal/pjsk/handler/` (bot command parsing) ≠ `internal/pjsk/sekai/` (upstream HTTP client). The latter is **always** imported as `sekaiapi`.      |
| `internal/pjsk/accountdata/` (user binding / profile settings) ≠ `internal/pjsk/render/snapshot/` (game snapshot data; previously `render/userdata`). |
| `internal/pjsk/parser/` no longer contains card parsers — card query parsing lives in `internal/pjsk/render/card/parser.go`.                          |

---

## 4. Database & migrations

- Seven ent clients live under `database/`: `bot`, `censor`, `chunithm/maindb`,
  `chunithm/music`, `pjsk`, `sekai`, `users`. Schemas are in `ent/<db>/schema/`
  (for CHUNITHM: `ent/chunithm/maindb/schema/` and `ent/chunithm/music/schema/`).
- **Auto-migrate runs at startup.** `internal/server/init_database.go`'s
  `initDBClient` helper calls `Schema.Create(ctx)` for every DB. There is no
  separate `cmd/migrate` tool any more (it was removed).
- After editing any `ent/<db>/schema/*.go`, run `go generate ./ent/<db>/...`
  and commit both the schema change **and** the regenerated files under
  `database/<db>/`.

### Common ent gotcha

`field.String(...).MaxLen(N)` validates **byte length** (`len(s)`), not rune
count. Aliases and other CJK-heavy fields need a comfortably large `MaxLen`
(currently `500` for both `alias.alias` and `group_alias.alias`).

---

## 5. CLIs

| CLI                             | Purpose                                                                                                                                                         |
|---------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `go build .`                    | Build the main server (`haruki-cloud`).                                                                                                                         |
| `cmd/importer/`                 | Migrate legacy `exports/*.json` into the new DB. Targets: `bindings`, `character-aliases`, `music-aliases`, `group-aliases`, `defaults`, or `all`. Idempotent. |
| `cmd/extractor/`                | Dump the `database/sekai` table/column/unique-key layout to `schema_info.json`.                                                                                 |
| `cmd/trust-signer/`             | Offline Ed25519 tool: `keygen` / `sign` / `verify` for the trust keyset and the online manifest signing key. Never run root `keygen` on the Cloud host.        |
| `cmd/asset-index/`              | Bootstrap a region's resource catalog (`--region`; dry inventory by default, `--publish` writes shards, BPM index and pointer). Pause the region updater first. |
| `cmd/image-cache-reconcile/`    | Page through render-cache index entries and check their objects; `--repair` drops index references to confirmed-missing objects. Never deletes objects.        |
| `scripts/provision_bot/`        | Manually provision a bot user (`--qq`, optional `--bot-id`, `--force`, `--rebind`) and print its credential.                                                    |

`cmd/importer` reads DB connection from env (`HARUKI_PJSK_DB_URL`,
`HARUKI_USERS_DB_URL`) or `haruki-cloud.yaml` (`--config`); exports are read
from `--exports-dir` (default `./exports`). Dry-run mode (`--dry-run`) parses
files and counts records without touching the DB.

- `all` runs the targets in order: bindings → character-aliases →
  music-aliases → group-aliases → defaults.
- Platform detection: an `im_user_id` shaped `<sha256hex>_<digits>` is
  imported as `qqbot`, anything else as `qq`.
- Default bindings: a user with one binding gets the global default plus that
  server's default; with several bindings the global default follows
  jp > cn > tw > en > kr and every server gets its own default. Existing
  defaults are skipped.

---

## 6. Testing

**After every code change, all three checks must pass before committing:**

```bash
go vet ./...
go test ./...
staticcheck ./...
```

- Baseline: all three are **green** (no known basal failures).
- Integration tests under `integration/` are gated by environment variable —
  they only run when `HARUKI_RUN_INTEGRATION=1` is set.
- Render-layer tests prefer constructing minimal `&renderapp.App{...}` literals
  with only the dependencies under test populated (the rest are nil-safe).
- When adding or extending an interface, all test mock implementations of that
  interface must be updated to implement the new methods.
- User-copy guards (see §12) run as ordinary tests and all require zero
  findings: `internal/i18n` (catalog style, catalog integrity, every message
  renders, copylint, help-doc style, helper golden file) and
  `internal/pjsk/handler` (help-doc triggers, route help coverage, help
  layout, help examples), plus the parameter-echo guards (echo-free forms in
  `internal/i18n`, error replies and endpoints in `api/bot/pjsk`). After changing helper output or a golden reply,
  regenerate with `HARUKI_UPDATE_GOLDEN=1 go test ./...` and review the diff.

---

## 7. Conventions

### Code style

- Comment only what genuinely needs clarification — do not annotate self-evident
  code.
- **Avoid emoji** in generated code unless explicitly requested.
- **No user copy in Go code.** Every user-facing string lives in the
  `internal/i18n` catalogs and every user-facing error is a typed
  `usererror.Error`; see §12 and `docs/i18n.md`.
- JSON goes through `internal/jsonutil` (an `encoding/json/v2` engine with
  v1-compatible semantics). Do not reintroduce `bytedance/sonic` — it was
  removed in the Go 1.27 / json/v2 migration.
- New code must thread `ctx` explicitly; no `context.Background()` in request
  paths.

### Workflow

- Don't push or commit without confirmation unless the user explicitly asks.
- Don't run linters / formatters / generators that aren't already in use here.
- `docs/` only contains "what the project is" reference material; planning,
  status, and integration notes live in the session workspace, not the repo.

---

## 8. Authoritative documents

When in doubt about current architecture, consult these in order:

| Document                          | Purpose                                                            |
|-----------------------------------|--------------------------------------------------------------------|
| `docs/architecture.cn.md`         | Top-level architecture / package responsibilities / parser layout  |
| `docs/database-schemas.cn.md`     | DB schema reference                                                |
| `docs/pjsk-command-system.cn.md`  | PJSK command system design                                         |
| `docs/toolbox-api.cn.md`          | Upstream Toolbox API contract                                      |
| `docs/build-policy.cn.md`         | Client build allowlist / revocation and security event alerts      |
| `docs/storage-migration.cn.md`    | Storage slot / object storage rollout and rollback table           |
| `docs/sk-tracker-cloud-contract.cn.md` | SK semantics Cloud expects from the Event Tracker cloud API   |
| `docs/public-bot-v2-api.cn.md`    | Public (unauthenticated) bot query API                             |
| `docs/player-frame-overrides.md`  | `pjsk_render.player_frame_overrides` operator config               |
| `docs/i18n.md`                    | How to add, review and translate user copy (catalogs, help docs)   |

`docs/` only describes the current shape of the project. Historical progress
and integration plans are no longer kept — when a doc and the code disagree,
the code wins; please update the doc in the same change.

---

## 9. Pitfalls worth re-checking after a merge

- `resolver_snapshot.go`, `runtime_test.go`, `command_execution_test.go` —
  former hot spots of the singleton-removal refactor; merges from older
  branches sometimes resurrect `sekaiapi.GetToolboxClient()`-style APIs.
- `parser/parser.go` no longer exists; do not re-import the deleted
  `CardParser` type.
- Server entry: only `main.go` at the repo root, build with `go build .`.
  Init logic (`init_*.go`, `fiber.go`, `run.go`) lives in `internal/server/`.
  Do not bring back `cmd/server/main.go`.

## 10. Production deployment — config debugging

Config is layered; the effective value for any setting is (highest priority
first):

1. **Environment variable** (e.g. `HARUKI_PJSK_RENDER_IMAGE_CACHE_URI`)
   — injected via the stack's `.env` + `docker-compose.yml` `environment:`
   block. `config.ReadConfig` applies these overrides after loading the YAML.
2. **`haruki-cloud.yaml`** — mounted into the container.

**Always check `.env` first when a config value is not taking effect.**
A stale or incorrect env var silently overrides the YAML.

To verify what a running container actually sees:

```bash
docker exec haruki-cloud env | grep HARUKI_
```

To apply `.env` changes, recreate the container with the same compose project
name, `--env-file` and `-f` files the stack was started with:

```bash
docker compose -p <project> --env-file .env -f docker-compose.yml [-f <override>.yml] \
  up -d haruki-cloud --force-recreate --no-deps
```

Pass the same `-p` / `--env-file` every time so Docker Compose resolves the
existing project.

---

## 11. Status snapshot

As of this revision the project is **considered functionally complete**:

- Main server, all PJSK render modules, bot pipeline, and CHUNITHM query
  paths are in production-ready state.
- Legacy data has been imported via `cmd/importer` (62 802 bindings, 1 230
  character aliases, 12 985 music aliases, 6 354 group aliases, 114 804
  default-binding rows across 52 002 users).
- Asset migration completed via parallel rsync to the new host.
- `cmd/migrate` removed (auto-migrate at startup). The remaining data tools are
  `cmd/importer` (legacy import), `cmd/asset-index` (catalog bootstrap) and
  `cmd/image-cache-reconcile` (render-cache index repair).

## 12. 用户文案规范（User copy）

本节是用户可见文案的唯一规范。用户可见文案指聊天回复、错误提示、`-help`
帮助文档，以及 Cloud 发给 Drawing、会被画进图片的标签和标题。

### 12.1 文案放在哪里

- **所有用户文案都在 i18n 目录里，Go 代码里不写文案。** 目录在
  `internal/i18n/locales/<locale>/`：每个领域一个 TOML 文件
  （`common.toml`、`format.toml`、`moderation.toml`……），帮助文档在
  `help/*.md`。目前只写 `zh-CN`；新增语言只需新建同级目录，缺失的消息自动回退到
  `zh-CN`。具体做法见 [`docs/i18n.md`](docs/i18n.md)。
- 代码通过 `internal/i18n` 取文案：`i18n.M(id, i18n.Data{...})` 得到
  `Message`（延迟渲染，`.String()` 或 `.In(locale)`），`i18n.T(...)` 直接得到
  `zh-CN` 字符串。ID 必须写成字符串字面量，占位符写成 `i18n.Data{...}` 字面量，
  这样完整性测试才能检查。不要手写 `i18n.Message{...}`。
- 共享格式一律用 `internal/i18n` 的函数，不要自己拼：`RegionLabel`、
  `RegionName`（只给自己加括号的图片用）、`AccountLabel`、`MaskUID`、
  `FormatUserTime`、`TimeAgo`、`UploadedLine`、`FormatDuration`、`Thousands`、
  `Wan`、`Decimal`、`Percent`/`PercentN`、`PageLabel`、`DifficultyLabel`、
  `LiveTypeLabel`、`EchoQuery`、`LinesText`，
  以及错误模板 `Unavailable`、`Timeout`、`NotFound`、`Ambiguous`、
  `OutOfRange`、`BadParam`、`Usage`、`WithUsage`、`RequestFailed`、
  `Misconfigured`、`ReadOnly`。
- **错误一律用 `utils/usererror` 的带类型错误**：`usererror.Error{Code,
  Message, Cause}`。`Error()` 只返回用户文案；`Cause` 只进日志
  （`usererror.LogText`）。构造函数：`Invalid`、`Forbidden`、`Setup`、
  `BadParam`、`Misuse`、`Unrecognized`、`Usage`、`NotFound`、`Ambiguous`、
  `OutOfRange`、`Unavailable`、`Timeout`、`Misconfigured`、`Internal`、
  `ReadOnly`，或 `New`/`Wrap`。
  - 用户输入错误必须用输入类 Code（`input`、`bad_param`、`usage`、
    `not_found`、`ambiguous`、`out_of_range`），不能显示成服务故障。需要用户
    自己先做一步（绑定、上传数据、验证）的用 `Setup`（Code `setup`）。这些和
    `forbidden`、`read_only` 都算正常结果（`usererror.IsExpected`），不记错误日志。
  - 指令写法不对时只写一行原因：`Misuse(reason)`；完全无法识别参数时用
    `Unrecognized()`。回复层（`api/bot/pjsk/error_reply.go`）按路由把
    `Unrecognized` 换成该路由的引导（`param_guidance.go`，目录 `guidance.*`），
    并给 `usage`/`bad_param` 加上用户实际输入的指令的“发送 /<指令> -help 查看
    用法”。生产代码不需要知道触发词。
  - 只有 Bot 管理员能处理的问题（未配置、鉴权失败、协议不兼容）一律
    `Misconfigured(cause)`，回复“服务配置异常，请联系 Bot 管理员”。
  - 任何路径都不能把原始错误文本给用户：英文、内部原因、上游响应体、状态码、
    字段名、内部 ID 都不行。禁止 `fmt.Errorf("<中文>: %w", err)` 这类写法；
    改成 `usererror.Wrap(code, i18n.M(...), err)`。
  - 只有带类型错误的文案会发给用户。回复层（`commandErrorText`）对其他错误先
    用上游分类器分类，分不出来就回复通用的“请求处理失败”。处理器里判断"已是
    用户文案"用 `isUserFacingError`（只认带类型错误）。错误分流一律按类型
    （`errors.Is`/`errors.As`、`usererror.As`、消息 ID），不要匹配错误文本。
  - 最后一道防线：`i18n.SanitizeMessage` 丢掉不是由任何目录消息渲染出来的行
    （丢掉的行写日志），结果为空或含敏感 URL 时回复通用错误。字面文字太少的
    模板行（如 `u{{.Index}} {{.Account}}`、`{{.Value}}万`、
    `{{.Region}}活动 {{.ID}}`）是弱模式，只在回复本身用到该消息时才认。
  - **参数回显默认关闭。** 客户端没有在请求里设 `enableParamEcho: true` 时，错误回复
    （参数错误、找不到、匹配到多个、超出范围、用法、参数引导）不能出现任何用户输入：
    查询词、参数值、无法识别的写法、用户写的名称和别名、用户写的未注册指令，以及**从用户
    指令里解析出的数字**（ID、名次、数量、WL 期数、页码、BPM、物量、游戏 UID、QQ 号……）。
    我们自己算出的数字（上限、范围、结果数量、按“当前活动”得到的活动 ID）不算用户输入。
    显示用户输入的占位符以 `User` 开头（`{{.UserQuery}}`、`{{.UserRank}}`），文字用
    `i18n.UserText`/`i18n.EchoQuery`，数字用 `i18n.UserNumber`；这样的消息必须有
    `<ID>_no_echo` 形式（同样的占位符去掉 `User…`，不留空引号或悬空的“：”），回复层按
    `i18n.WithParamEcho` 选择。已注册的指令名不算用户输入。分不清来源的数字按用户输入处理
    （例如错误回复里的活动 `common.event_label`）。
  - **还没有审核的别名按收件人区分。** 回复提交者或任何非管理员的消息（提交别名的确认，
    以及提交时的“已在待审核列表/已审核/与名称重复/提交里有重复”）按回显规则：不开启回显时
    只显示待审核 ID（“待审核别名 #12：歌曲「…」（ID 74）”）。只有通过别名审核管理员检查
    （`Service.requireAdmin`）后才会产生的回复——待审核列表、查提交者、同意、拒绝、批量拒绝，
    以及同意时的“已审核/与名称重复/批量里重复”错误——不论是否开启回显都显示别名原文
    （`alias.record.review`、`alias.review.*`，占位符不用 `User…`），因为管理员要审核原文；
    其中管理员写的待审核 ID 仍按回显规则。删除已审核别名照常显示原文。判断依据是命令本身的
    权限检查，不看群聊/私聊。提交确认用 `onebot11.LocalizedText` 返回目录消息，投递层按客户端
    的设置渲染（共享执行的结果同时带两种回复）。其他成功回复不在此列。详见 `docs/i18n.md`。
  - 测试断言 `Code` 和消息 ID，不断言中文原文（`testutil.RequireUserError`、
    `testutil.MessageID`）；中文由目录本身和
    `internal/i18n/testdata/helpers.zh-CN.golden` 锁定。
- 上游返回的英文错误（工具箱、游戏数据服务、查榜服务、组卡服务、渲染服务）属于
  跨仓库约定，全部集中在 `internal/core/upstreamerr`：`contract.go` 列出各服务
  的消息片段和状态码规则，`Classify` 给出 `Service` + `Kind`，`UserError` 给出
  通用回复；客户端返回实现 `upstreamerr.Described`/`Kinded` 的错误（网络错误用
  `upstreamerr.Transport`）。新增上游错误时改 `contract.go` 并补
  `contract_test.go` 和各客户端的契约测试；上游有状态码或错误码时优先用它们。

### 12.2 目录约定

- **ID**：小写字母、数字、下划线，用点分段，第一段等于文件名：
  `<领域>.<子域>.<名称>`，例如 `moderation.kill.done_until`。ID 一旦被代码使用
  就保持稳定；措辞变了也不改 ID。不要用 go-i18n 的保留字（`id`、`other`、
  `one`、`description` 等）作 ID 段。
- **description**：每条都必须有，写清出现在哪里（哪条指令、什么情况），再写
  `占位符：Name=含义；Other=含义`，每个占位符都要说明。审查文案的人只看目录，
  不看代码。
- **占位符**：只用命名占位符 `{{.Name}}`（大驼峰），不用 `%s`/`%d`，不用模板
  逻辑。需要分支时拆成几条消息。值是 `Message` 时会按同一语言先渲染。显示用户输入的
  占位符以 `User` 开头，消息要有 `<ID>_no_echo` 形式（12.1）。
- 一个领域一个文件；共享模板放 `common.toml`，格式化相关放 `format.toml`。
- 新消息必须被代码使用，否则完整性测试失败（确需保留的写进
  `internal/i18n/testdata/unused_ids.allowlist` 并注明原因）。

### 12.3 已定规则

| 编号 | 规则 |
|---|---|
| B | 区服显示：`日服(JP)`、`国服(CN)`、`台服(TW)`、`韩服(KR)`、`国际服(EN)`（半角括号、大写代码、不加空格，由 `RegionLabel` 生成）。账号行：`[日服(JP)] <UID>`（`AccountLabel`）。通用名词用"区服"，不用"服务器"指区服。小写区服代码只作为输入语法（如 `/jp查曲`）。 |
| C | 首次提到或引导用户去工具箱上传时写"抓包数据（Suite）"、"烤森（MySekai）"；之后写"抓包数据"、"烤森"、"烤森数据"。 |
| D | 中文与拉丁字母、数字之间加一个半角空格：`卡牌 ID`、`最多 5 个`、`游戏 UID`。例外：用户原样输入的记号（`u1`、`event123`、`t100`、`wl1`、`10火`、`/jp查曲`）和 B 的区服显示名。 |
| E | 活动点数：名词写 PT（目标 PT、活动 PT），数字后的单位写 pt（还需 1234 pt）。 |
| F | 不向用户提内部组件（SekaiAPI、Tracker、Cloud、Cloud 节点、masterdata、OneBot self_id、Client、数据库、上游、Drawing/绘图服务）。用功能名：获取游戏数据失败、查榜服务、渲染服务。只有 Bot 管理员能处理的问题统一回复"服务配置异常，请联系 Bot 管理员"，细节写日志或 commandtrace。 |
| H | Cloud 负责发给 Drawing 的所有标签的本地化，从目录取预先本地化的字符串。Drawing 目前自己本地化的原始 key 先不动，记为 Drawing 后续事项。改动图片文字时提升 `renderCacheKeyVersion`。 |
| I | 帮助文档里写了但未注册的指令：没有歧义和冲突时把文档写法注册为别名，否则改文档。 |
| J | 错误脱敏必须完整，见 12.1。 |
| K | 烤森"数据已过期"、`/sud`（抓包状态）和 `/msd` 用同一套术语（C）、同一个时间格式函数和同一种结构。 |
| L | 每个已注册路由都有帮助文档；帮助示例必须能被解析；区服前缀说明只生成一次；帮助版式统一；用法错误回复一行原因加"发送 /<指令> -help 查看用法"（`WithUsage`）。 |

### 12.4 默认规则

- **语气**：用"你"或省略主语，不用"您"。中性陈述，不用语气词，文字回复不用
  emoji（✅/❌ 改成"已验证/未验证"）。图片里同一组标签的 emoji 要么都有要么都没有。
- **标点**：中文后用全角冒号"："；中文里用全角括号"（）"（B 和 `欢乐嘉年华(5v5)` 除外）；逗号"，"、
  并列"、"、分句"；"。单句聊天回复结尾不加"。"，同一条里两句话之间用"。"。
  关键词和用户输入值用“”，游戏专有名词用「」，不用 `"`。描述性范围用 `~`
  （0~100），输入语法保留 `-`（1-4）。区服代码列表按 jp、cn、tw、kr、en 排序。
  URL 单独成行或两侧留空白。省略号写"……"。
- **格式**：时间统一 `2026-10-09 14:05 (UTC+8)`（`FormatUserTime`，按用户时区，
  默认 Asia/Shanghai，禁止服务器本地时区和 `MST`）；时长 `2分03秒`
  （`FormatDuration`）；大数用"万"（`Wan`），千分位只用 `Thousands`；百分比用
  `Percent`；分页 `第 1/3 页`（`PageLabel`）。
- **结构**：成功回复用"已<动词>……"；错误回复写"<原因>，<下一步>"。用法表头写
  "用法："。参数错误第一行写"参数格式不正确：“<参数>”"，下一行写具体原因
  （`BadParam`）。
- **保持不变**：公开 API 文档约定的 `message: "success"`
  （`docs/public-bot-v2-api.cn.md`）和 `api/public` 的英文协议消息。

### 12.5 术语表

| 概念 | 规范写法 | 禁用写法 |
|---|---|---|
| 区服（通用） | 区服 | 服务器（指区服时；"游戏服务器维护中"可保留） |
| 区服显示名 | 日服(JP)、国服(CN)、台服(TW)、韩服(KR)、国际服(EN) | JP服、[JP]、在 JP 服、CN 服、日服（jp） |
| 区服代码（输入） | jp、cn、tw、kr、en | jp/cn/en/tw/kr、大小写混写 |
| suite 数据 | 抓包数据（Suite）→ 抓包数据 | suite、Suite数据、User Data、用户数据、套装 |
| MySekai | 烤森（MySekai）→ 烤森、烤森数据 | mysekai、Mysekai、MySekai数据 |
| command | 指令 | 命令 |
| 榜线 | 榜线 | 档线、分数线、SK 线 |
| 档位 / 名次 | T100（档位）、第 N 名（名次） | 显示用的 t100、Txxx名 |
| 账号 | 游戏账号 | PJSK账号、%s服PJSK账号 |
| 账号号码 | 游戏 UID | 游戏ID、游戏UID、账号ID、单独的 UID |
| 默认账号 | 默认绑定 | 主账号（只作触发词）、默认账号 |
| 平台身份 | QQ 号 | QQ号、QQ账号 |
| 工具箱 | 工具箱（帮助里首次写 Haruki 工具箱） | Toolbox、toolbox、Haruki工具箱 |
| 卡组 / 组卡 | 卡组 = 5 张卡；组卡 = 动作或功能 | 主队、队伍、推荐队伍 |
| 歌曲 | 歌曲、歌曲名、歌曲 ID | 歌、曲目 |
| stamp | 贴纸、贴纸 ID | 表情（指贴纸时） |
| 别名系统 | 歌曲别名、角色别名；状态写已审核、待审核 | 角色昵称（指别名时）、已通过的别名 |
| 提及某人 | @群友 | @用户、@某人 |
| 自制谱面 | 自制谱面 | 自定义谱面、自制谱 |
| Live | 虚拟 Live、多人 Live、单人 Live | 虚拟Live、虚拟演唱会、多人LIVE、协力 |
| World Link | WL（WL 活动、WL 章节） | WorldLink、wl活动 |
| 活动点数 | PT（名词）、pt（数字后单位） | 目标PT、1234PT |
| 演出能量（火） | 演出能量（游戏里的正式名称，图片、目录和帮助正文都用它）；帮助里首次提到或需要用户输入时写“演出能量（火）”。火是用户输入的简写，输入写法 `5火`、`10火`、`5体力` 原样保留在反引号里 | 显示文本里的体力、火数 |
| 欢乐嘉年华活动 | 欢乐嘉年华(5v5)（半角括号，图片、目录和帮助正文都用它，例如“欢乐嘉年华(5v5) 胜率预测”）；用户输入仍写 `5v5` | 单独的 5v5 作显示名、5v5（欢乐嘉年华） |
| 排名追踪 | 排名追踪，帮助的用法和示例只写 `/排名追踪`（`/档线轨迹` 只作为已注册别名） | 档线轨迹 |
| 个人信息 | 个人信息、个人信息背景、自定义个人信息、模块化个人信息 | 个人资料、资料卡、模块化资料、profile、自定义档案 |
| 难度 | EASY、NORMAL、HARD、EXPERT、MASTER、APPEND | 显示文本里的小写 expert |
| 封禁到期 | 解封时间 | 封禁至 |
| 内部组件 | 游戏数据服务 / 获取游戏数据失败、查榜服务、渲染服务、组卡服务 | SekaiAPI、Tracker、Cloud、masterdata、Client、数据库、上游、绘图服务 |
| 管理员才能处理 | 服务配置异常，请联系 Bot 管理员 | 请更新 Client、请检查配置 |
| 称呼 | 你 / 省略主语 | 您 |
| 重试 | 请稍后再试 | 请稍后重试 |
| 不可用 | 暂时不可用 | 未就绪 |
| 用法 | 用法：、发送 /<指令> -help 查看用法 | 使用方式:、查看完整用法请发送： |
| 参数错误 | 参数格式不正确：“<参数>” | 参数解析失败 |

### 12.6 不属于文案（留在代码里）

- 用户输入的指令触发词和别名表（`Commands: []string{...}`、`PrefixArgs`）、
  解析关键字（`internal/pjsk/parser/`、`internal/pjsk/filteralias/`、
  `render/common/nicknames.go`、`render/card/extractor.go` 等）。
- 游戏主数据名称（角色、歌曲、卡牌、家具等）和外部 API 的取值
  （`utils/censor` 的"合规/不合规"）。
- 日志、内部错误原因、测试数据、运维工具（`cmd/`、`scripts/`）。
- 存进数据库、之后原样显示的值（例如系统写入的封禁原因），在该行末尾标注
  `//copylint:ignore <原因>`。
- 上游服务写的状态和错误文本（`internal/core/upstreamerr`）。
- 代码里的触发词、关键字和游戏数据只用这两种标注，原因必须写：单行在行尾写
  `//copylint:ignore <原因>`；整张表（`var`/`const` 声明）在声明的注释里写一行
  `//copylint:ignore-block <原因>`。常用原因：`解析关键字`、`指令触发词`、
  `角色名（游戏数据）`。只放解析表的目录和文件列在 `copylintAllowedPaths`。

### 12.7 防回退测试

| 测试 | 位置 | 规则 |
|---|---|---|
| `TestCatalogStyle` | `internal/i18n` | 目录零容忍：汉字后半角冒号、中文半角括号、中英之间缺空格、单句结尾"。"、禁用词（您、请稍后重试、未就绪、命令、Toolbox、SekaiAPI、Tracker、masterdata、Cloud、suite、Mysekai、套装、档线、分数线、体力、（状态、`"` 等）、`%s` 占位符、缺 description、占位符未说明、ID 格式 |
| `TestCatalogIntegrity` | `internal/i18n` | 代码引用的 ID 都存在、占位符一一对应、没有未使用的 ID、其他语言不多出 ID |
| `TestCatalogDescriptionReferencesExist` | `internal/i18n` | description 里提到的消息 ID 都存在（可用 `*` 表示一组） |
| `TestEveryMessageRendersWithSampleData` | `internal/i18n` | 每条消息都能用示例数据渲染，占位符都出现在结果里，没有残留模板语法 |
| `TestCopylint` | `internal/i18n` | 非测试 Go 代码里没有中文字面量和"中文 + %w/%v"的 `fmt.Errorf`（标注见 12.6） |
| `TestHelpDocStyle` | `internal/i18n` | 帮助文档的标点、空格和禁用词零容忍（反引号和代码块不检查） |
| `TestHelperGolden` | `internal/i18n` | 共享格式函数的中文输出（`testdata/helpers.zh-CN.golden`） |
| `TestHelpDocTerms` | `internal/i18n` | 帮助文档正文（反引号和代码块以外）不用术语表的禁用写法 |
| `TestHelpDocTriggersAreRegistered` | `internal/pjsk/handler` | 帮助文档里反引号中的 `/指令` 都能解析到已注册指令 |
| `TestCatalogCommandsAreRegistered` | `internal/pjsk/handler` | 目录消息正文里叫用户发送的 `/指令` 都能解析到已注册指令 |
| `TestEveryRouteHasHelpDoc` | `internal/pjsk/handler` | 每个已注册路由都有自己的帮助文档 |
| `TestEveryHelpDocIsReachable` | `internal/pjsk/handler` | 没有用户看不到的帮助文档（只允许路由文档、`generic`、`mysekai_blueprint`） |
| `TestHelpDocsFollowLayout` | `internal/pjsk/handler` | 帮助版式：`# 标题`、用法/参数/示例/说明按顺序，不手写区服前缀说明（版式见 `docs/i18n.md`） |
| `TestHelpDocExamplesParse` | `internal/pjsk/handler` | `## 示例` 里的每个示例都能被该文档的路由解析 |
| `TestUserInputMessagesHaveEchoFreeForm` | `internal/i18n` | 带 `User…` 占位符的消息都有 `<ID>_no_echo` 形式，占位符一致 |
| `TestEchoFreeFormsHideUserInput` | `internal/i18n` | 用文字哨兵和数字哨兵（`UserNumber` 和普通 int 各一次）渲染这些消息：不回显时哨兵不出现、不留空引号或悬空的“：”，回显时出现 |
| `TestUserWrittenPlaceholdersAreUserInput` | `internal/i18n` | description 写着“用户写的/用户输入/管理员输入/用户提交”的占位符必须以 `User` 开头（成功回复等例外逐条列出） |
| `TestCommandErrorTextHidesUserInputWithoutParamEcho`、`TestCommandErrorTextHidesNumbersWithoutParamEcho`、`TestBotEndpointHidesUserInputWithoutParamEcho` | `api/bot/pjsk` | 错误回复和 Bot 端点在没有开启参数回显时不出现用户输入（文字和数字），开启后照常显示 |
| `TestSucceededSharedBotCommandKeepsBothVariants`、`TestAdminAliasReviewReplyShowsTextWithoutEcho`、`TestAliasTextFollowsTheAudience`、`TestApprovalErrorsShowAliasTextToAdmins`、`TestAliasRepliesGolden` | `api/bot/pjsk`、`internal/pjsk/alias` | 提交者不开启回显时看不到未审核的别名原文，共享结果同时带两种回复；管理员的审核回复不开启回显也显示原文；golden 文件记录不同的形式 |

这些测试都要求零发现，没有基线。唯一的允许清单是
`internal/i18n/testdata/unused_ids.allowlist`（暂时没有代码引用的消息 ID，每行写原因），
目前为空。

## Git commits

All commit subjects must follow:

```text
[Type] Short description starting with capital letter
```

Allowed types:

| Type      | Usage                                                 |
|-----------|-------------------------------------------------------|
| `[Feat]`  | New feature or capability                             |
| `[Fix]`   | Bug fix                                               |
| `[Chore]` | Maintenance, refactoring, dependency or build changes |
| `[Docs]`  | Documentation-only changes                            |

Rules:

- Description starts with a capital letter.
- Use imperative mood: `Add ...`, not `Added ...`.
- No trailing period.
- Keep the subject at or below roughly 70 characters.
- **Agent attribution uses the standard Git `Co-authored-by:` trailer in the commit body, not a free-form `Agent:` line.** This makes GitHub render the co-author avatar on the commit page. The trailer must be on its own line, separated from the subject by a blank line, in the form `Co-authored-by: <Display Name> <email>`. Suggested values per agent:
  - Claude (any 4.x): `Co-authored-by: Claude Opus 4.7 <noreply@anthropic.com>` (substitute the actual model, e.g. `Claude Sonnet 4.6`, `Claude Haiku 4.5`)
  - Codex: `Co-authored-by: Codex <noreply@openai.com>`
  - Copilot: `Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>`

Examples from this repo's history:

```text
[Feat] Add tracker batch trace line lookups
[Fix] Resolve custom profile resources
[Chore] Bump dependencies
[Docs] Clean up obsolete docs, keep only project-shape reference files
```

## GitHub Actions workflows

CI reuses the shared templates in
[`seiunx-dev/ci-templates`](https://github.com/seiunx-dev/ci-templates) at `@v1`.
The files in `.github/workflows` are thin callers:

- `ci.yml` (`CI`) runs on `main` pushes, pull requests targeting `main`, and manual
  dispatch:
  - `go-ci`: `gofmt`, `go mod tidy -diff`, `go build` / `go vet` (`-mod=readonly`),
    staticcheck (pinned in `.github/tools/go.mod`), then the tests **once**:
    `go test -race -count=1 ./...` with coverage (`-coverpkg` leaves out `database/` and
    `ent/`, minimum 90%) and a Postgres service exposed as `HARUKI_IMAGECACHE_TEST_DSN`.
  - `sonar` scans that coverage (skipped green on Dependabot/fork PRs); the suite is not
    run a second time for Sonar.
  - `docker` does not wait for the tests. PRs build only, and only when Go sources,
    `go.mod`/`go.sum`, the Dockerfile or the workflows change. On `main` it runs in
    parallel with the tests and pushes the immutable
    `ghcr.io/team-haruki/haruki-cloud:sha-<full sha>` and `:sha-<7 chars>` as soon as the
    build finishes. The `Docker tags` job (template `docker-retag.yml`, after `CI OK`) then moves
    `:main` to that digest without rebuilding, so `:main` only follows commits whose
    `CI OK` passed. Main images report the version `main-<sha7>`. The registry
    `:buildcache` keeps the module download layer.
  - `workflows` runs actionlint on the workflow files.
  - The aggregate job **`CI OK`** (needs `go`, `sonar`, `docker`, `workflows`) is the
    single gate: `Docker tags` and the release gate wait for it. It is meant to be the
    required status check, but `main` currently has no branch protection or ruleset,
    so GitHub does not enforce it on merges.
- `integration.yml` (`Integration`) is manual only and stays out of `CI`.
  `./integration/...` drives a **running** Haruki-Cloud server (`HARUKI_TEST_BASE_URL`,
  default `http://127.0.0.1:6666`) with a provisioned bot and its own users/pjsk
  databases. The workflow starts Postgres and Redis but not that server, so it cannot pass
  on a hosted runner as it stands (it has never been run).
- `release.yml` (`Release`): push a tag `v<version>` on a `main` commit whose `CI OK` is
  green. The version comes from the tag only (`version.Version` is `dev` in source and is
  set with `-ldflags -X`), so there is no version file to bump. `release-gate` waits for
  `CI OK` on the tagged commit; then `go-release` cross-compiles `haruki-server` (CGO off,
  `-trimpath`, `version.Version=v<version>`) into `haruki-server-linux-amd64.tar.gz` and
  `haruki-server-linux-arm64.tar.gz` (binary at the archive root); the image is **built**
  (not promoted from `main`, because the version is compiled in) with `VERSION=<version>`
  and tagged `:<version>`, `:<major>.<minor>` and, for the highest stable tag, `:latest`
  (production pulls `:<version>`, e.g. `3.7.5`); and the GitHub Release is published with
  `SHA256SUMS-<tag>.txt`. Manual dispatch is a dry run: it builds the binaries as
  `v0.0.0-dev.<sha7>` and publishes nothing, also when started on a tag.
- Dockerfile: modules are downloaded in their own layer, `ARG VERSION` is declared right
  before `go build` so its per-commit value does not invalidate that layer, and the binary
  is built with `CGO_ENABLED=0` like the release binaries (the server's SQLite driver is
  `modernc.org/sqlite`; `mattn/go-sqlite3` is only used by tests).

Workflow maintenance rules:

- Use the shared templates first. Add custom jobs or steps only when a template
  genuinely cannot meet the project's needs, keep them in the thin caller files, and
  add a comment explaining why.
- Template bugs and missing features are fixed upstream in `seiunx-dev/ci-templates`
  (new `v1.x.y` tag), not worked around here.
- Keep top-level `permissions: contents: read`; grant `packages: write` / `contents: write`
  only on the job that needs it.
- Do not suppress `githubactions:S7637` (full-SHA pins) in `sonar-project.properties`: the
  template's `sonar.yml` already ignores it for the `@v1` references.
- Third-party actions in caller-side custom steps are pinned to a full commit SHA with a
  `# vX.Y.Z` comment; Dependabot (`github-actions`) updates them and the template refs.
- CI uses the Go version in `go.mod` exactly (`GOTOOLCHAIN=local`); keep
  `.github/tools/go.mod` and the Dockerfile's `golang` image on the same version.

## Release notes

Release notes follow the org standard,
[`RELEASE_NOTES.md` in `seiunx-dev/ci-templates`](https://github.com/seiunx-dev/ci-templates/blob/main/RELEASE_NOTES.md),
and are written in English.

- Title every release with the tag only, for example `v3.8.3`; every tag gets a release.
- Publish a tag as a pre-release only when it has an `-alpha`, `-beta` or `-rc` suffix.
- Omit empty sections, and end every item with its PR number `(#123)`, or the short
  commit SHA when there is no PR.
- `release.yml` publishes the release with auto-generated notes; rewrite them to the
  standard afterwards with `gh release edit <tag> --notes-file <file>` (never pass `--latest`).
