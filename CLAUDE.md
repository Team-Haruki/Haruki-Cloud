# Haruki-Cloud — Claude Working Notes

## 约定与规范

- **提交信息格式**：见 [AGENTS.md](AGENTS.md)（`[Type] Capitalized description`，`Type ∈ {Feat, Fix, Chore, Docs}`）。
- **代码生成避免 Emoji**：除非用户明确要求。
- **提交前确认**：除非用户显式指示，改动完成后先汇报，待指令再 commit；不要擅自 `push`。

## 项目入口文档（按重要性排序）

| 文档 | 用途 |
|------|------|
| `docs/project-completion-tracker.cn.md` | 路由清单、模块完成度分档（A/B/C/D）、当前仓库事实总览（~1800 行） |
| `docs/refactoring-progress.cn.md` | 重构进度（P0–P5、R36–R38），里面的 **R38 表格** 列了当前仍在推进的低优先项 |
| `docs/architecture.cn.md` | 顶层架构 / 分包职责 / parser 布局 |
| `docs/project-status-summary.cn.md` | 阶段性总结 |

改动涉及模块时，优先在这些文档里确认**当前**的架构事实，历史说明已很多但未必还生效。

## 架构要点

### 组合根
- `internal/pjsk/render/app.App` 是 render runtime 的组合根，包含所有 controller 和外部 client 字段。
- `cmd/server/init_services.go` 负责把 `config.Cfg` 里的各段转成 `renderapp.Config`，然后构造 `renderapp.App`。
- handler 层通过 `rc.App.<Field>` 访问共享依赖；**不要再引入包级单例**。

### Sekai / Toolbox / Tracker 客户端
- 构造器：`NewSekaiAPIClient(*config.SekaiAPIConfig)` / `NewToolboxClient(*config.ToolboxConfig)` / `NewTrackerClient(*config.TrackerConfig)`。
- 方法全部 **nil-receiver 安全**，nil 时返回 `ErrClientNotConfigured`（定义在 `internal/pjsk/sekai/errors.go`）。
- 测试构造 `&renderapp.App{}` 只设需要的字段即可；未设的 client 会走错误路径而不是 panic。
- 若新增测试涉及真实 Sekai HTTP 调用（`httptest.NewServer` + `config.Cfg.SekaiAPI.BaseURL`），**记得把 `SekaiAPI: sekaiapi.NewSekaiAPIClient(&config.Cfg.SekaiAPI)` 塞进 App 字面量**（参考 `TestExecuteMysekaiPhoto`、`TestBuildPublicMusicProfilesUsesSelectorFromRequestParams`）。

### 包命名避坑
- `handler/sekai`（bot 命令解析）≠ `pjsk/sekai`（上游 HTTP 客户端）。后者 import 一律 alias 为 **`sekaiapi`**。
- `accountdata/`（用户绑定 / profile 设置）≠ `render/userdata/`（游戏快照 Snapshot）。后者仍待重命名为 `render/snapshot`，import 时注意别搞混。

### 用户身份 / 快照链路
- 实际路径：`Toolbox -> local static (debug fallback, 仅 AllowFallback=true)`。
- **生产 prod 环境不再 fallback 到 local snapshot**。
- Cloud 侧不再镜像快照（无 `pjsk_user_snapshots`、无 `snapshot/upload`）；Toolbox 为事实来源。
- Handler 不应直接调 `GetPrivateDataValue(...)` 做单 key 查询，统一走 `ResolveSnapshot` → controller 消费。

### Context 传递
- 重构主链已基本清掉 `context.Background()`。新写代码时一律通过 `rc.Ctx`、`ctx` 参数传递，不要随手 `context.Background()`。
- DB provider（`render/provider/db_*`）已经支持按请求克隆 source，保持这个风格。

## 测试基线

截至 2026-04-16，`go test ./...` 有 **37 packages ok / 7 FAIL**。以下失败**均为基线上已存在**的失败，**不是回归**：

| 包 | 失败测试 |
|----|----------|
| `api/bot/pjsk` | `TestBotEndpointSKQueryUsesTrackerAtBindingPayload`、`TestBotEndpointSKQueryHandlesInlineCQAtInTextSegment` |
| `internal/pjsk/handler` | `TestResolveTrackerTargetUserSupportsSelector`、`TestResolveDeckMusicSelectionMusicCompareSelections`、`TestExecuteCardImageReturnsAllOriginalArts` |
| `internal/pjsk/render/card` | `TestResolveCardImagesSupportsStandardAndRipPaths` |

定位手段：`git stash --include-untracked` 切回基线、重跑同组测试；若失败依旧则可排除是当前改动引入的。

## R38 剩余项（低优先）

1. `render/userdata` → `render/snapshot` 重命名（~20 文件 import 变更，churn 最大）
2. `render/deck/controller_prepare.go` 862 行按职责拆分
3. `accountdata/` 导出收窄（`ProfileBGCleaner`/`BindingResolver` 可降为 unexported）
4. `render/misc`（38 行）是否吸收 —— 已评估性价比低，暂保留

## 常见陷阱

- `cmd/server/` 目录被 `.gitignore` 的 `server` 规则匹配。已跟踪的文件仍能 `git commit`，但首次 `git add` 需要 `-f`。
- Remote merge 后检查 `resolver_snapshot.go`、`runtime_test.go`、`bridge_test.go` 等 — 这些是早期 singleton migration 的高频冲突点，合并方可能把旧 API（`sekaiapi.GetToolboxClient()`）恢复进来。
- 删除 `parser/parser.go` 之后，`internal/pjsk/parser/` 包内再无 `CardParser` 系列。card 查询解析统一走 `internal/pjsk/render/card/parser.go`。
