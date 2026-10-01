# Haruki-Cloud — Claude Working Notes

> 这份文档是给 Claude 看的“私房话”：项目通用规范请先读 [AGENTS.md](AGENTS.md)，
> 这里只补充协作习惯、长期记忆要点和容易踩的坑。

## 1. 协作习惯

- **提交格式**：见 AGENTS.md（`[Type] Capitalized description`，
  `Type ∈ {Feat, Fix, Chore, Docs}`）。
- **不要 push、不要擅自 commit**：除用户显式指示外，所有改动完成后先汇报，等用户
  下指令再 `git commit`；`git push` 永远只在用户明确要求时执行。
- **Emoji**：除非用户明确要求，生成代码 / 文档不要带 emoji（CLI 输出同理）。
- **会话工作区**：长任务的计划、临时表格、checkpoint 等放在
  `~/.copilot/session-state/<id>/` 下（plan.md / files/ / SQL DB 都在这里）。
  仓库内不要新建“规划用”的 markdown。

## 2. 项目当前状态（2026-04-23）

项目已经基本完工：

- 主 server、PJSK render 全模块、bot pipeline、CHUNITHM 查询都处于可上线状态。
- 旧服务数据已通过 `cmd/importer` 完整导入新库：
  62 802 条 binding / 1 230 character alias / 12 985 music alias / 6 354 group
  alias / 114 804 条 default binding（覆盖 52 002 个用户）。
- 资产数据 (~159 GB) 已通过 6 路并行 rsync 迁移到新主机。
- `cmd/migrate` 已删除（auto-migrate 在 `internal/server/init_database.go` 启动
  时由 `Schema.Create(ctx)` 完成）。
- `cmd/importer` 是目前**唯一**保留的一次性数据迁移 CLI；`importer-linux-amd64`
  是已交付的 Linux x64 静态二进制（27 MB，CGO_ENABLED=0）。

## 3. 入口文档（按重要性）

| 文档 | 用途 |
|------|------|
| `docs/architecture.cn.md` | 顶层架构 / 分包职责 / parser 布局 |
| `docs/database-schemas.cn.md` | DB schema 参考 |
| `docs/pjsk-command-system.cn.md` | PJSK 指令体系设计 |
| `docs/toolbox-api.cn.md` | 上游 Toolbox API 契约 |
| `docs/deck_refer_help.md` | `deck` 命令族用户帮助 |
| `docs/build-policy.cn.md` | 客户端构建许可 / 撤销与安全事件告警 |

改动涉及模块时，以代码为准——`docs/` 仅描述项目当前形态，不再保留历史进度/对接记录。

## 4. 架构要点

### 组合根
- `internal/pjsk/render/app.App` 是 render runtime 的唯一组合根。
- `internal/server/init_services.go` 把 `config.Cfg.*` 转成 `renderapp.Config`
  并构造 `renderapp.App`。
- handler 层一律通过 `rc.App.<Field>` 访问共享依赖；**不要再引入包级单例**。

### Sekai / Toolbox / Tracker 客户端
- 构造器：`NewSekaiAPIClient(*config.SekaiAPIConfig)` /
  `NewToolboxClient(*config.ToolboxConfig)` /
  `NewTrackerClient(*config.TrackerConfig)`。
- 全部 **nil-receiver 安全**，nil 时返回 `ErrClientNotConfigured`
  （`internal/pjsk/sekai/errors.go`）。
- 测试构造 `&renderapp.App{}` 只设需要的字段即可；未设的 client 走错误路径而
  不是 panic。
- 涉及真实 Sekai HTTP 调用（`httptest.NewServer` + `config.Cfg.SekaiAPI.BaseURL`）
  时，记得把 `SekaiAPI: sekaiapi.NewSekaiAPIClient(&config.Cfg.SekaiAPI)` 塞进
  App 字面量（参考 `TestExecuteMysekaiPhoto`、
  `TestBuildPublicMusicProfilesUsesSelectorFromRequestParams`）。

### 用户身份 / 快照链路
- 生产路径：`Toolbox -> local static (debug fallback, 仅 AllowFallback=true)`。
- prod 不再 fallback 到 local snapshot。
- Cloud 侧不再镜像快照（无 `pjsk_user_snapshots`、无 `snapshot/upload`）；
  Toolbox 为事实来源。
- Handler 不直接 `GetPrivateDataValue(...)` 单 key 查询，统一走
  `ResolveSnapshot` → controller。

### Context 传递
- 主链已基本清掉 `context.Background()`。新代码一律通过 `rc.Ctx` / `ctx`
  参数传递，不要随手 `context.Background()`。
- DB provider（`render/provider/db_*.go`）支持按请求克隆 source，新 provider
  保持这个风格。

### 包命名避坑
- `handler`（bot 命令解析，`internal/pjsk/handler/`）≠ `pjsk/sekai`（上游 HTTP
  客户端）。后者 import **一律** alias 为 `sekaiapi`。
- `accountdata/`（用户绑定 / profile 设置）≠ `render/snapshot/`（游戏快照
  Snapshot，曾名 `render/userdata`）。
- `internal/pjsk/parser/` 包内**无** `CardParser`；card 查询解析在
  `internal/pjsk/render/card/parser.go`。

## 5. 数据库 / ent 注意事项

- 6 个 ent 库：`bot` / `censor` / `chunithm` / `pjsk` / `sekai` / `users`。
- 启动时 `initDBClient` 会对每个 db 调用 `Schema.Create(ctx)` —— **不需要**
  额外迁移工具。
- 改 `ent/<db>/schema/*.go` 后必须 `go generate ./ent/<db>/...`，并把
  `database/<db>/` 下的生成文件一起 commit。
- `field.String(...).MaxLen(N)` 校验的是 `len(s)`（**字节数**），不是 rune 数。
  CJK 字段（如 alias）当前用 `MaxLen(500)`（参考 commit `965abdd`）。

## 6. 测试基线

**每次改动后，提交前必须确保以下三项全部通过：**

```bash
go vet ./...
go test ./...
staticcheck ./...
```

- 三项基线均为全绿，无已知基础失败。
- `integration/` 目录默认跳过；只有 `HARUKI_RUN_INTEGRATION=1` 时执行。
- 新增或扩展 interface 时，所有测试中的 mock 实现都必须同步补齐新方法。

## 7. 服务器启动 / 构建

- 主入口只有 `main.go` 在**项目根**，构建命令 `go build .`，产物
  `haruki-cloud`（或在仓库已有 `haruki-server-linux`）。
- 启动初始化 (`init_*.go`、`fiber.go`、`run.go`) 都在 `internal/server/` 包
  （`package server`）。
- `cmd/` 目前有 `cmd/importer/`、`cmd/extractor/` 与 `cmd/trust-signer/`
  （离线 Ed25519 签名工具：生成根密钥 / 在线 manifest 密钥，签发 keyset；
  绝不在 Cloud 主机上运行 keygen 根密钥）。

## 8. importer 速查

```bash
# 导入全部（顺序：bindings → char-aliases → music-aliases → group-aliases → defaults）
HARUKI_PJSK_DB_URL="host=... dbname=... user=... password=... sslmode=disable" \
HARUKI_USERS_DB_URL="host=... dbname=... user=... password=... sslmode=disable" \
go run ./cmd/importer --target all

# 仅补 default bindings
... go run ./cmd/importer --target defaults

# Dry run
... go run ./cmd/importer --dry-run
```

- 平台识别：`im_user_id` 形如 `<sha256hex>_<digits>` 时为 `qqbot`，否则
  `qq`。
- Default binding 策略：单账号 → global + 区服默认；多账号 → global 按
  jp > cn > tw > en > kr 优先级，外加每个区服各设一个服务器默认。
- 全部步骤幂等，可重复执行。

## 9. 常见陷阱（合并后重检查）

- `resolver_snapshot.go`、`runtime_test.go`、`command_execution_test.go`：
  早期 singleton migration 的高频冲突点，对方分支可能把旧 API
  （`sekaiapi.GetToolboxClient()`）恢复进来。
- `parser/parser.go` 已删除，`CardParser` 系列不存在；不要因为合并把它带回。
- `cmd/server/` 已经上移到根；不要再恢复 `cmd/server/main.go`。
- `cmd/migrate/` 已删除；auto-migrate 已经覆盖。

## 10. 排查服务器配置问题 — .env 优先级

生产环境 compose 文件在 `/data/HarukiServices/configs/`，配置优先级（高到低）：

1. **环境变量**（通过 `.env` + `docker-compose.yml` `environment:` 注入）
2. **`haruki-cloud.yaml`**（挂载到容器）

**排查配置不生效时，第一步先检查 `.env`**，env var 会静默覆盖 YAML，容易漏查。

```bash
# 查看容器实际生效的所有 HARUKI_ 环境变量
docker exec haruki-cloud env | grep HARUKI_

# 修改 .env 后重建容器
docker compose -p haruki-production --env-file .env \
  -f docker-compose.yml -f docker-compose.production.yml \
  up -d haruki-cloud --force-recreate --no-deps
```

注意：compose project name 是 `haruki-production`（`.env` 里 `COMPOSE_PROJECT_NAME`
设定）。执行 compose 命令时必须带 `-p haruki-production` 或 `--env-file .env`。

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
    build finishes. The `Docker tags` job (`docker-retag.yml`, after `CI OK`) then moves
    `:main` to that digest without rebuilding, so `:main` only follows commits whose
    `CI OK` passed. Main images report the version `main-<sha7>`. The registry
    `:buildcache` keeps the module download layer.
  - The aggregate job **`CI OK`** is the only required status check.
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
