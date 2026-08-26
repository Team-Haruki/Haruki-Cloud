# Copilot Instructions — Haruki Cloud

This file is the canonical onboarding document for GitHub Copilot working on
the Haruki-Cloud repository. It mirrors [`AGENTS.md`](../AGENTS.md) at the repo
root, which is read by other AI assistants (Codex, Claude, Copilot CLI, etc.).

For deeper architecture background, the authoritative human-facing references
are in [`docs/`](../docs/). The most important entry points are listed in
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
| Auth           | JWT (golang-jwt/v5) + AES-256-GCM + Noise NK |
| JSON           | `encoding/json/v2` via `internal/jsonutil`   |
| Go             | 1.27                                         |

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
│   ├── cache/drawingcache/ # drawing image cache (store, GC, stats, admin API)
│   ├── cluster/            # node role / read-only mode helpers (config.Cfg.Node)
│   ├── core/crypto/        # Noise protocol helpers
│   ├── core/upstream/      # upstream connection pool / transport
│   ├── handler/            # cross-domain command registry / bot routing
│   ├── identity/           # platform user → haruki user resolution
│   ├── jsonutil/           # JSON facade: encoding/json/v2 engine, v1-compatible semantics
│   ├── middleware/secure/  # security middleware
│   ├── observability/commandtrace/ # command execution tracing
│   ├── onebot11/           # OneBot11 message helpers (was internal/pjsk/onebot11/)
│   └── pjsk/               # PJSK subsystem (see §4)
│
├── config/                 # YAML config loader + timeouts
├── database/               # ent-generated DB clients (bot/censor/chunithm/pjsk/sekai/users)
├── ent/                    # ent schema definitions (mirror of database/)
├── docs/                   # canonical human-facing documentation
├── exports/                # legacy JSON snapshots for the importer (local only, not in git)
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
├── assets/      # asset providers
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

---

## 7. Conventions

### Code style

- Comment only what genuinely needs clarification — do not annotate self-evident
  code.
- **Avoid emoji** in generated code unless explicitly requested.
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

## 10. Production deployment — config debugging

The production compose stack is at `/data/HarukiServices/configs/` on the
server. Config is layered; the effective value for any setting is (highest
priority first):

1. **Environment variable** (e.g. `HARUKI_PJSK_RENDER_IMAGE_CACHE_CHARTS_URI`)
   — injected via `.env` + `docker-compose.yml` `environment:` block.
2. **`haruki-cloud.yaml`** — mounted into the container.

**Always check `.env` first when a config value is not taking effect.**
A stale or incorrect env var silently overrides the YAML. Key file:
`/data/HarukiServices/configs/.env`.

To verify what a running container actually sees:

```bash
docker exec haruki-cloud env | grep HARUKI_
```

To apply `.env` changes, recreate the container:

```bash
docker compose -p haruki-production --env-file .env \
  -f docker-compose.yml -f docker-compose.production.yml \
  up -d haruki-cloud --force-recreate --no-deps
```

Note: the compose project name is `haruki-production` (set via
`COMPOSE_PROJECT_NAME` in `.env`). Always pass `-p haruki-production` or
`--env-file .env` so Docker Compose resolves the correct project.

---

## 11. Status snapshot

As of this revision the project is **considered functionally complete**:

- Main server, all PJSK render modules, bot pipeline, and CHUNITHM query
  paths are in production-ready state.
- Legacy data has been imported via `cmd/importer` (62 802 bindings, 1 230
  character aliases, 12 985 music aliases, 6 354 group aliases, 114 804
  default-binding rows across 52 002 users).
- Asset migration completed via parallel rsync to the new host.
- `cmd/migrate` removed (auto-migrate at startup); `cmd/importer` is the only
  remaining one-off data tool.

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

Use the standardized workflow layout in `.github/workflows`:

- `ci.yml` runs on `main` pushes, pull requests targeting `main`, and manual dispatch.
- Go CI order: `gofmt`, `go build ./...`, `go vet ./...`, `staticcheck ./...`, then `go test -race -count=1 ./...`.
- `release.yml` is the standard release build entrypoint. It runs on `v*` tags and manual dispatch, builds release artifacts, uploads them with `actions/upload-artifact`, and publishes GitHub Release assets on tag pushes.
- `docker.yml` is the standard Docker entrypoint. It runs on `main` pushes, `v*` tags, PRs that touch Docker/build inputs, and manual dispatch. PRs build only; non-PR runs push GHCR images with lowercase image names and Docker metadata tags.
- `integration.yml` is a manual integration-test workflow and stays separate from normal CI.

Workflow maintenance rules:

- Keep workflow filenames and top-level names aligned: `CI`, `Release`, `Docker`, and optional package-specific names.
- Use `actions/checkout@v6`, `actions/setup-go@v6`, `actions/upload-artifact@v7`, `actions/download-artifact@v8`, `softprops/action-gh-release@v3`, and current Docker actions (`setup-buildx@v4`, `login@v4`, `metadata@v6`, `build-push@v7`).
- Keep `permissions` minimal: `contents: read` for CI/Docker build-only work, `contents: write` for release publishing, and `packages: write` only when pushing container images.
- Use workflow `concurrency` keyed by workflow name and ref, with release jobs using `release-${{ github.ref_name }}` and `cancel-in-progress: false`.
- Do not reintroduce legacy workflow names such as `rust-ci.yml`, `build.yml`, `release-build.yml`, `docker-build.yml`, or `docker-release.yml` unless a package-specific workflow already exists and is intentionally preserved.
