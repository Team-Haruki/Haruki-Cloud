# Haruki Cloud — Agent Guidelines

This file is the canonical onboarding document for any AI assistant working on
the Haruki-Cloud repository (Codex, Claude, Copilot CLI, etc.). It mirrors
[`.github/copilot-instructions.md`](.github/copilot-instructions.md).

For deeper architecture background, the authoritative human-facing references
are in [`docs/`](docs/). The most important entry points are listed in
[Authoritative documents](#authoritative-documents) below.

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
| Auth           | JWT (golang-jwt/v5) + AES-256-GCM + Noise NK |
| JSON           | bytedance/sonic                              |
| Go             | 1.26.1                                       |

There is **one** runtime entry point: `main.go` at the repo root, which only
sets up signal handling and calls `server.Run(ctx)` from
`internal/server/`. Auxiliary CLIs live under `cmd/` (`importer`, `extractor`).

---

## 2. Repository layout

```
Haruki-Cloud/
├── main.go                 # server entry point; calls internal/server/.Run
├── internal/server/        # bootstrap: init_*.go, fiber.go, run.go
├── cmd/
│   ├── importer/           # legacy export → new DB importer CLI
│   └── extractor/          # schema extractor utility
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
│   └── groupguard/
│
├── internal/
│   ├── core/crypto/        # Noise protocol helpers
│   ├── handler/            # cross-domain command registry / bot routing
│   ├── identity/           # platform user → haruki user resolution
│   ├── middleware/secure/  # security middleware
│   ├── onebot11/           # OneBot11 message helpers (was internal/pjsk/onebot11/)
│   └── pjsk/               # PJSK subsystem (see §4)
│
├── config/                 # YAML config loader + timeouts
├── database/               # ent-generated DB clients (bot/censor/chunithm/pjsk/sekai/users)
├── ent/                    # ent schema definitions (mirror of database/)
├── docs/                   # canonical human-facing documentation
├── exports/                # legacy JSON snapshots for the importer
├── scripts/                # ops helpers (e.g. provision_bot)
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
| `handler/`        | Bot command parsing + execution dispatch (NOT the upstream client)  |
| `meta/`           | Static meta tables                                                  |
| `parser/`         | Free-text command parsers (card parser lives in `render/card`)      |
| `region/`         | Region normalisation                                                |
| `render/`         | Render runtime (controllers, providers, snapshots)                  |
| `requestbuilder/` | Internal request builders for the render layer                      |
| `sekai/`          | **Upstream Sekai HTTP client** — always import as alias `sekaiapi`  |

`internal/pjsk/render/` is the bulk of the runtime:

```
render/
├── app/         # the App composition root (see §3)
├── assets/      # asset providers
├── card/        # card lookup / parser / detail / list
├── common/      # shared render helpers
├── deck/        # deck recommend (challenge / event / WL)
├── education/   # leader / bonds / area
├── event/       # event metadata, ranks
├── gacha/       # gacha details
├── honor/       # honor logic
├── masterdata/  # master data adapters
├── misc/        # miscellaneous (e.g. birthday)
├── music/       # music detail / list / progress / rewards
├── mysekai/     # MySekai data
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
  a `renderapp.Config` and constructs the `App` via `renderapp.New(cfg)`.
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

- Six ent-generated DBs live under `database/`: `bot`, `censor`, `chunithm`,
  `pjsk`, `sekai`, `users`. Schemas are in `ent/<db>/schema/`.
- **Auto-migrate runs at startup.** `internal/server/init_database.go`'s
  `initDBClient` helper calls `Schema.Create(ctx)` for every DB. There is no
  separate `cmd/migrate` tool any more (it was removed).
- After editing any `ent/<db>/schema/*.go`, run `go generate ./ent/<db>/...`
  and commit both the schema change **and** the regenerated files under
  `database/<db>/`.

### Common ent gotcha

`field.String(...).MaxLen(N)` validates **byte length** (`len(s)`), not rune
count. Aliases and other CJK-heavy fields need a comfortably large `MaxLen`
(currently `500` for both `aliases.alias` and `group_aliases.alias`).

---

## 5. CLIs

| CLI                      | Purpose                                                                                                                                                        |
|--------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `go build .`             | Build the main server (`haruki-cloud`).                                                                                                                        |
| `cmd/importer/`          | Migrate legacy `exports/*.json` into the new DB. Targets: `bindings`, `character-aliases`, `music-aliases`, `group-aliases`, `defaults`, or `all`. Idempotent. |
| `cmd/extractor/`         | Schema extraction helper.                                                                                                                                      |
| `scripts/provision_bot/` | Bot provisioning helper.                                                                                                                                       |

`cmd/importer` reads DB connection from env (`HARUKI_PJSK_DB_URL`,
`HARUKI_USERS_DB_URL`) or `haruki-cloud.yaml`. Dry-run mode (`--dry-run`)
parses files and counts records without touching the DB.

---

## 6. Testing

- Baseline: `go test ./...` is **all green** (no known basal failures).
- Integration tests under `integration/` are gated by environment variable —
  they only run when `HARUKI_RUN_INTEGRATION=1` is set.
- Render-layer tests prefer constructing minimal `&renderapp.App{...}` literals
  with only the dependencies under test populated (the rest are nil-safe).

---

## 7. Conventions

### Git commit format

All commits **must** follow:

```
[Type] Short description starting with capital letter
```

| Type      | Usage                                                 |
|-----------|-------------------------------------------------------|
| `[Feat]`  | New feature or capability                             |
| `[Fix]`   | Bug fix                                               |
| `[Chore]` | Maintenance, refactoring, dependency or build changes |
| `[Docs]`  | Documentation-only changes                            |

Rules:

- Description starts with a **capital letter**.
- Imperative mood (`Add ...`, not `Added ...`).
- No trailing period.
- Keep subject ≤ ~70 chars.

Examples:

```
[Feat] Add toolbox live snapshot provider
[Fix] Move user_snapshot config under pjsk_render
[Chore] Rename config file to haruki-cloud.yaml
[Docs] Update known-bugs.md with snapshot fix
```

### Code style

- Comment only what genuinely needs clarification — do not annotate self-evident
  code.
- **Avoid emoji** in generated code unless explicitly requested.
- Prefer `bytedance/sonic` over `encoding/json` in hot paths (the codebase
  already standardises on this).
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
| `docs/deck_refer_help.md`         | User-facing help text for the `deck` command family                |

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

---

## 10. Status snapshot

As of this revision the project is **considered functionally complete**:

- Main server, all PJSK render modules, bot pipeline, and CHUNITHM query
  paths are in production-ready state.
- Legacy data has been imported via `cmd/importer` (62 802 bindings, 1 230
  character aliases, 12 985 music aliases, 6 354 group aliases, 114 804
  default-binding rows across 52 002 users).
- Asset migration completed via parallel rsync to the new host.
- `cmd/migrate` removed (auto-migrate at startup); `cmd/importer` is the only
  remaining one-off data tool.
