# Service-Test -> Haruki-Cloud Merge Plan

## 1. Conclusion

This merge should be done as "move business logic into Haruki-Cloud, rebuild the transport layer around Haruki-Cloud conventions, keep a compatibility layer during cutover".

Do not copy `Service-Test/cmd/server/main.go` into `Haruki-Cloud/cmd/server` as-is. The two projects already diverged in server lifecycle, route style, config layout, and responsibility boundaries.

Recommended end state:

- `Haruki-Cloud` remains the only server process.
- `Haruki-Cloud` becomes the only place that owns PJSK render/build APIs.
- `Service-Test` controller/builder/source logic is moved into a new internal PJSK render subsystem inside `Haruki-Cloud`.
- Existing `Haruki-Cloud` assets are reused where they already exist:
  - `database/sekai`
  - `utils/drawing`
  - `utils/query`
  - `api` response/middleware conventions

## 2. Current State Review

### 2.1 Service-Test

Observed characteristics:

- Standalone `net/http` server with `http.NewServeMux`.
- `96` route registrations and `78` handler functions in `cmd/server/main.go`.
- Core layering is already non-trivial:
  - `internal/controller`
  - `internal/builder`
  - `internal/service`
  - `pkg/masterdata`
  - `pkg/asset`
  - `pkg/deck_cgo`
- Depends on `haruki-cloud` via local `replace` in `go.mod`.
- Uses `haruki-cloud/database/sekai` only as a data source dependency, not as the hosting runtime.
- Owns an additional local-file runtime:
  - masterdata JSON directory
  - asset directory
  - `user.json`
  - `music_metas.json`
  - `mysekai.json`
  - optional deck recommendation backend / CGo engine
- Exposes two API styles:
  - module-specific `build` / `render` endpoints
  - unified `POST /api/render` dispatch

Main functional modules currently present:

- card
- music
- gacha
- event
- education
- honor
- profile
- stamp
- misc
- score
- deck
- sk
- mysekai

### 2.2 Haruki-Cloud

Observed characteristics:

- Single Fiber server in `cmd/server/main.go`.
- Centralized config loading from `haruki-db-configs.yaml`.
- Existing public API style is `/<module>/...`, with JSON envelope helpers in `api/helper.go`.
- Existing internal/private style already uses auth middleware where needed.
- `api/pjsk` currently exposes only alias endpoints.
- `utils/query` already acts as an in-process query toolkit for consumers.
- `utils/drawing/models.go` already contains most of the drawing payload structs that Service-Test recreates in `internal/model`.
- `utils/drawing/client.go` already contains the Drawing API client wrapper that overlaps with `Service-Test/internal/service/drawing.go`.
- Repository includes `database/sekai`, but `cmd/server` does not currently initialize a Sekai DB client.
- Current `config.PJSK` means the alias/binding DB, not Sekai masterdata DB.

### 2.3 Structural Relationship Between Them

The current relationship is:

- `Service-Test` is an upper-layer application.
- `Haruki-Cloud` already provides part of its infrastructure.
- `Service-Test` adds a second domain model and a second transport layer on top of that infrastructure.

That means the merge is mostly about removing duplicate layers, not merely moving files.

## 3. Main Conflicts To Resolve

### 3.1 Server and Route Style Conflict

- `Service-Test` uses `net/http` and returns raw PNG / raw JSON bodies directly.
- `Haruki-Cloud` uses Fiber and a standard JSON envelope for normal APIs.
- `Service-Test` routes are `/api/...`.
- `Haruki-Cloud` routes are grouped by product domain, for example `/pjsk/...`.

Implication:

- Business logic and HTTP handlers must be separated during migration.
- Route compatibility should be handled explicitly, not accidentally.

### 3.2 Model Duplication

There are two overlapping model systems:

- `Service-Test/internal/model/*`
- `Haruki-Cloud/utils/drawing/models.go`

There is also a duplicated game-domain model layer:

- `Service-Test/pkg/masterdata/*`
- `Haruki-Cloud/database/sekai/*` entities plus related query packages

Implication:

- If these duplicates are kept after merge, the codebase will become harder to reason about than it is now.
- The merge should reduce duplication, not preserve it.

### 3.3 Data Source Split

`Service-Test` currently supports two data backends:

- local masterdata JSON
- Haruki-Cloud Sekai DB through `*_cloud_source.go`

`Haruki-Cloud` currently hosts:

- `database/pjsk` for alias/binding/preference data
- `database/sekai` code generation and DB access packages
- but no runtime initialization of a Sekai DB client in `cmd/server`

Implication:

- merged `Haruki-Cloud` must add explicit Sekai DB configuration and lifecycle management
- the existing `PJSK` config block cannot simply be reused for this

### 3.4 User Data Model Mismatch

This is the biggest product-level mismatch.

`Service-Test` has two kinds of modules:

Modules that are mostly masterdata-driven:

- card
- music detail/list/chart
- gacha
- event
- honor
- stamp
- misc
- score
- sk

Modules that depend strongly on a local user snapshot:

- profile
- music progress / rewards
- education
- deck auto recommend
- mysekai
- parts of card/music payload decoration
- unified `/api/render` when it implies per-user behavior

Current `Service-Test` behavior for these modules is effectively single-tenant process state, because `UserDataService` loads one local file set at startup.

`Haruki-Cloud` is a multi-user backend, so the same approach is not acceptable as the final state.

Implication:

- these modules cannot be migrated safely without a new user snapshot abstraction
- migration order must distinguish stateless-ish modules from user-bound modules

### 3.5 Deck Engine Packaging Risk

`Service-Test/pkg/deck_cgo` currently makes default CGo builds fail when the native library is absent.

Implication:

- this must not be merged as an unconditional build dependency into `Haruki-Cloud`
- the deck CGo path needs to become opt-in

## 4. Recommended Target Architecture

### 4.1 High-Level Shape

Keep `Haruki-Cloud` as the host application. Add a dedicated internal render subsystem:

```text
Haruki-Cloud/
  api/pjsk/
    route.go
    render_route.go
    render_handler.go
    render_struct.go
    render_dispatch.go
  internal/pjsk/render/
    app/
    controller/
    builder/
    source/
    userdata/
    assets/
    masterdata/
    deck/
```

Responsibility split:

- `api/pjsk/*`
  - HTTP route registration
  - Fiber request binding / response writing
  - auth / compatibility routing
- `internal/pjsk/render/*`
  - migrated Service-Test business logic
  - controllers
  - builders
  - data source adapters
  - user snapshot providers
  - optional local masterdata fallback
- `utils/drawing/*`
  - canonical Drawing API request models and client
- `database/sekai/*`
  - canonical Sekai DB access

### 4.2 What Should Be Canonical After Merge

Use these as the canonical implementations:

- Drawing payload structs: `Haruki-Cloud/utils/drawing/models.go`
- Drawing client: `Haruki-Cloud/utils/drawing/client.go`
- Sekai DB client: `Haruki-Cloud/database/sekai`
- Cloud server/runtime/config/middleware: `Haruki-Cloud`

Temporary transitional components that are acceptable during migration:

- a moved copy of `Service-Test/pkg/masterdata` under `internal/pjsk/render/masterdata`
- moved cloud source adapters that still convert ent entities into that temporary model

These should be transitional, not permanent design goals.

### 4.3 Recommended Route Strategy

Recommended canonical routes inside `Haruki-Cloud`:

- public data APIs stay under `/pjsk/...`
- render/build APIs become internal APIs, for example:
  - `POST /internal/pjsk/render`
  - `POST /internal/pjsk/card/detail/build`
  - `POST /internal/pjsk/card/detail/render`
  - `POST /internal/pjsk/music/detail/build`
  - and so on

Recommended migration compatibility layer:

- keep temporary aliases for legacy Service-Test routes:
  - `POST /api/render`
  - `POST /api/card/...`
  - `POST /api/music/...`
  - etc.
- protect internal render endpoints with `api.VerifyAPIAuthorization()` once consumers are ready

This lets us:

- preserve old callers during cutover
- align new work with Haruki-Cloud route conventions
- avoid mixing large render-only APIs into public unauthenticated surfaces by default

## 5. Package-Level Migration Map

| Service-Test source | Haruki-Cloud target | Action |
| --- | --- | --- |
| `cmd/server/main.go` | `api/pjsk/render_route.go` + `cmd/server/main.go` init wiring | Split business bootstrap from HTTP transport |
| `cmd/server/render_dispatch.go` | `api/pjsk/render_dispatch.go` | Keep functionality, rewrite around Fiber and new app container |
| `internal/controller/*` | `internal/pjsk/render/controller/*` | Move mostly as-is first, then trim transport leftovers |
| `internal/builder/*` | `internal/pjsk/render/builder/*` | Move mostly as-is first |
| `internal/service/*_cloud_source.go` | `internal/pjsk/render/source/*` | Keep adapter role, but hosted inside Haruki-Cloud |
| `internal/service/*_data_source.go` | `internal/pjsk/render/source/*` | Preserve interfaces during first migration phase |
| `internal/service/drawing.go` | use `utils/drawing/client.go` | Do not keep a second drawing client |
| `internal/model/drawing_request.go` | use `utils/drawing/models.go` | Replace with canonical drawing models |
| `internal/model/query.go` | `api/pjsk/render_struct.go` or `internal/pjsk/render/request/*` | Keep only incoming query/request types |
| `internal/model/score_request.go` | use `utils/drawing/models.go` where applicable | Remove duplicated output/request models |
| `internal/apiutils/cloud_clients.go` | `cmd/server/main.go` Sekai DB init + app container | Remove duplicate DB init wrapper |
| `internal/config/config.go` | `Haruki-Cloud/config/config.go` | Merge into new config sections |
| `pkg/asset/*` | `internal/pjsk/render/assets/*` or `utils/assets/*` | Move and keep local path logic |
| `pkg/masterdata/*` | `internal/pjsk/render/masterdata/*` | Temporary compatibility domain model |
| `internal/service/masterdata*.go` | `internal/pjsk/render/masterdata/*` | Keep local JSON fallback as optional provider |
| `pkg/deck_cgo/*` | `internal/pjsk/render/deck/*` | Keep optional and build-tagged |
| `data/*` | `internal/pjsk/render/static/*` or `assets/pjsk/*` | Move static deck-related data into Haruki-Cloud |
| `cmd/dbprobe/main.go` | optional `cmd/sekai-dbprobe/main.go` or drop | Keep only if still useful for ops/debug |

## 6. New Config That Haruki-Cloud Will Need

Add a dedicated Sekai runtime config instead of overloading current `PJSKConfig`.

Recommended shape:

```yaml
sekai:
  enabled: true
  db_type: postgres
  db_url: ...

pjsk_render:
  enabled: true
  drawing_base_url: http://...
  drawing_timeout: 30s
  drawing_retry_count: 3
  asset_dirs:
    primary: /path/to/assets
    legacy: []
  local_masterdata:
    enabled: false
    dir: /path/to/masterdata
  user_snapshot:
    provider: local_file
    user_json: /path/to/user.json
    music_meta_json: /path/to/music_metas.json
    mysekai_json: /path/to/mysekai.json
  deck_recommend:
    enabled: true
    use_local_engine: false
    timeout: 60s
    default_algs: [dfs, ga]
```

Key point:

- current `config.PJSK` in Haruki-Cloud is for alias/binding data
- the merged render system needs a new Sekai masterdata DB lifecycle plus its own render config block

## 7. Recommended Migration Order

### Phase 0: Baseline And Freeze

- Freeze `Service-Test` behavior before moving files.
- Record current route list and payload contracts.
- Port existing Service-Test unit tests into a migration backlog.
- Add a short-lived contract list for:
  - request JSON
  - response JSON for `build`
  - HTTP status behavior
  - binary render behavior

Output of this phase:

- explicit list of supported endpoints
- explicit list of routes that must remain compatible during cutover

### Phase 1: Foundation Extraction

- Add Sekai DB config and initialization to `Haruki-Cloud/cmd/server/main.go`.
- Introduce `internal/pjsk/render` package tree.
- Move `pkg/asset` into the new internal render subsystem.
- Move `pkg/masterdata` and local masterdata loaders into the new internal render subsystem as a temporary compatibility layer.
- Replace `Service-Test/internal/service/drawing.go` usage with `utils/drawing/client.go`.
- Replace duplicated drawing request models with `utils/drawing` types or aliases to them.

Exit criteria:

- Haruki-Cloud can boot a render app container without exposing routes yet
- no transport code from Service-Test is copied directly into `cmd/server`

### Phase 2: Stateless / Low-Coupling Modules First

Migrate these first:

- gacha
- event
- honor
- stamp
- misc
- score
- sk
- music detail/list/chart
- card detail/list/box

Reason:

- they are mostly DB/masterdata + asset + drawing transformations
- they do not require solving multi-user snapshot semantics first

Work items:

- move controller/builder/source code
- add Fiber handlers in `api/pjsk`
- add compatibility routes for old `/api/...` paths if needed
- add contract tests around build payloads

Exit criteria:

- these modules run inside Haruki-Cloud
- old Service-Test is no longer required for them

### Phase 3: Unified Dispatch

- Migrate `/api/render` logic into `api/pjsk/render_dispatch.go`.
- Redefine the incoming command payload as a Haruki-Cloud-owned struct.
- Normalize parameter validation behavior during migration. Do not keep silent JSON unmarshal failures.

Recommended behavior change during this phase:

- invalid `Params` JSON should fail fast with a clear error
- do not continue with zero-value structs silently

Exit criteria:

- command-parser / bot integrations can call Haruki-Cloud directly for migrated modules

### Phase 4: User Snapshot Abstraction

Introduce a formal provider interface, for example:

```go
type SnapshotProvider interface {
    LoadByBinding(ctx context.Context, harukiUserID int, server string) (*UserSnapshot, error)
    LoadByRawSource(ctx context.Context, source SnapshotSource) (*UserSnapshot, error)
}
```

Recommended first implementation:

- `LocalFileSnapshotProvider`

Recommended target implementation:

- storage-backed or service-backed snapshot provider tied to Haruki user/binding context

This phase is required before the following modules are considered truly merged:

- profile
- music progress / rewards
- education
- deck auto recommend
- mysekai

Exit criteria:

- user-bound modules do not depend on one process-wide `user.json`

### Phase 5: User-Bound Modules

After the snapshot provider is in place, migrate:

- profile
- education
- mysekai
- deck
- music progress / rewards

Special note:

- `profile` in current Service-Test pretends to accept `user_id`, but effectively reads only startup-local user data
- this should be corrected during migration instead of preserved

Exit criteria:

- per-user behavior is real, not emulated by one local file

### Phase 6: Deck Engine Hardening

- Keep deck CGo engine behind an explicit build tag or an explicit opt-in package.
- Default Haruki-Cloud builds must succeed without native deck libraries.
- Keep HTTP backend recommendation path as the default runtime-safe path.

Recommended technical direction:

- introduce a build tag such as `pjsk_deck_cgo`
- only compile native deck bindings when explicitly requested

Exit criteria:

- `go test ./...` and default builds work without native deck artifacts

### Phase 7: Cutover And Removal

- Switch callers from Service-Test base URL to Haruki-Cloud.
- Keep compatibility routes temporarily.
- After production soak:
  - remove compatibility aliases
  - archive or delete Service-Test repo/module
  - delete duplicated models still left over from temporary migration

## 8. Test Strategy

### 8.1 What To Reuse

Port these first:

- builder tests from Service-Test
- controller tests from Service-Test
- Haruki-Cloud style route tests for new `/pjsk` endpoints

### 8.2 What To Add

Add three layers of tests:

- unit tests for builders and source adapters
- route tests for Fiber endpoints
- integration tests with:
  - Sekai DB test client
  - optional fake drawing server

### 8.3 What To Assert

For render/build migration, assert:

- request validation
- region routing
- data source selection
- output payload JSON
- status code behavior

Prefer asserting drawing payload JSON over asserting PNG bytes directly.

## 9. Risks And Decisions That Must Be Made Early

### 9.1 User Snapshot Source

Open decision:

- Is Haruki-Cloud going to own user snapshot storage?
- Or will it continue receiving prebuilt snapshot JSON from another system?

This decision directly affects:

- profile
- deck
- education
- mysekai
- music progress/rewards

### 9.2 Route Compatibility Scope

Open decision:

- Should legacy `/api/...` routes remain long-term?
- Or are they only temporary compatibility routes during migration?

Recommended answer:

- temporary only

### 9.3 Local Masterdata Fallback

Open decision:

- Should Haruki-Cloud keep local JSON masterdata fallback in production?

Recommended answer:

- keep it as an optional fallback or dev tool
- DB-backed Sekai source should be the canonical production path

### 9.4 Deck Native Engine

Open decision:

- Is native deck recommendation a required production dependency?

Recommended answer:

- no, not by default
- keep it optional and isolated

## 10. Recommended First Implementation Slice

If the goal is fastest useful progress with controlled risk, implement in this order:

1. Add Sekai DB runtime config and client init to Haruki-Cloud.
2. Create `internal/pjsk/render` package tree.
3. Migrate asset helper, source interfaces, and stateless modules first.
4. Expose Fiber routes for card/music/gacha/event/honor/stamp/misc/score/sk.
5. Migrate unified dispatch for only those first-wave modules.
6. Design and implement user snapshot provider.
7. Migrate profile/education/mysekai/deck and remaining user-bound music flows.

This avoids blocking the whole merge on the hardest part.

## 11. Short Summary

The correct merge is not:

- "copy Service-Test into Haruki-Cloud"

The correct merge is:

- keep Haruki-Cloud as the host
- move Service-Test domain logic into a new internal PJSK render subsystem
- reuse `database/sekai` and `utils/drawing`
- add a real Sekai DB runtime and a real user snapshot abstraction
- migrate stateless modules first
- migrate user-bound modules after the data model is corrected

That gives a result that is consistent with Haruki-Cloud's current architecture instead of embedding a second server inside it.
