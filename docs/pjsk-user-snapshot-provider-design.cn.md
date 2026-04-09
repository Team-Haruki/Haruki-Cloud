# PJSK 用户快照 Provider 设计

## 1. 设计结论

- 生产环境的用户快照来源应当是 `Haruki-Cloud` 内部 provider，而不是本地 `user.json` / `music_metas.json` / `mysekai.json`。
- `local_file` 只保留为测试、联调和开发 fallback，不作为正式运行方案。
- render 请求路径上的 provider 应当是只读组件，不在请求链路里直接做实时抓取或临时拼文件。
- provider 必须建立在 Cloud 现有身份与绑定体系之上：
  - `users`：平台用户身份
  - `pjsk.user_bindings` / `pjsk.user_default_bindings`：PJSK 绑定关系
  - `Toolbox`：生产环境中的用户态事实来源

阶段性说明：

- 当前 `Service-Test` 合并阶段不实现这套正式 provider。
- 当前阶段先继续使用本地 JSON 作为临时过渡方案，保证剩余模块可以先并入 `Haruki-Cloud`。
- 本文档保留为后续替换本地 JSON 路径时的正式设计依据。
- 2026-04-09 补充：第一阶段抽象已开始落地，`internal/pjsk/render/userdata` 已新增 `Snapshot` / `SnapshotProvider` / `SnapshotFactory` 接口，并接入 `ToolboxSnapshotProvider` 作为请求级 live snapshot provider、`DefaultSnapshotFactory` 作为统一快照构造入口。
- 2026-04-09 再补充：架构口径已重新收敛为“Cloud 只读、Toolbox 为事实来源”。此前尝试过的 `pjsk_user_snapshots`、`snapshot/upload`、验证后回填、读链写回等 Cloud 侧快照镜像方案已撤回，不再作为正式设计。
- 2026-04-09 再补充一条：`Snapshot.RawValue(key)` 已落地，`userPlayerFrames` / `userMusicAchievements` 这类顶层原始字段已经可以直接从 Toolbox 快照读取，`profile`、`deck`、`music/rewards` 的一部分路径已不再需要在 handler 层单独直连 Toolbox key 查询。

## 2. 已确认的事实

### 2.1 数据来源（已确认）

用户态快照数据统一由 **Haruki Toolbox API** 提供（`utils/sekai.HarukiToolboxClient`）：

| 本地文件 | Toolbox 调用 | 数据类型 | 存储字段 |
|---------|-------------|---------|---------|
| `user.json` | `GetSuiteData(server, pjskUserID, platform, platformUserID)` | `ToolboxDataTypeSuite` | 运行时直接读取，不在 Cloud 持久化 |
| `mysekai.json` | `GetMySekaiData(server, pjskUserID, platform, platformUserID)` | `ToolboxDataTypeMySekai` | 运行时直接读取，不在 Cloud 持久化 |
| `music_metas.json` | `https://sekai-data.3-3.dev/music_metas-{region}.json`（公开远端） | 全局区服数据 | `music_meta_cache`（区服级） |

`music_metas` 是全局按区服的静态数据（完全不含用户状态），来源与用户快照无关，存储层独立设计。

当前 Toolbox API 路由（详见 [toolbox-api.cn.md](toolbox-api.cn.md)）：

- 完整快照：`GET /api/private/game-data/:server/:data_type/:user_id`
- 单 key 查询（如 `upload_time`）：同路由加 `?key=...`
- 快速验证绑定查询：`GET /api/private/game-binding?platform=...&platform_user_id=...`

Cloud 当前不再维护独立的“快照写入链路”或 Cloud 侧快照镜像表。

**不**接受外部 raw_json 推送；生产数据由 Cloud 在请求期只读拉取 Toolbox。

2026-04-09 进度补充：

- 当前统一 snapshot/provider 读路径已开始下沉到 handler/controller：
  - `userPlayerFrames` 已可经由 snapshot 供 `profile`、部分公共资料卡、`deck/mysekai` 读取
  - `userMusicAchievements` 已可经由 snapshot 供 `music/rewards` 读取
  - `userBonds`、`userCharacters`、`userCharacterMissionV2s`、`userCharacterLiveUsageCounts`、`userCharacterMissionV2Statuses` 已可经由 snapshot 供 `education/bonds`、`education/leader` 读取
  - `education` 相关 bonds / leader 构建逻辑已进一步下沉到 `render/education` controller；为此 `EducationProvider` / `DataSource` 已补齐 bonds、bond levels、game-character style、leader mission requirement 等 masterdata 访问能力
  - `mysekai` raw payload 已新增独立 `MySekaiPayloadProvider`（直接只读 Toolbox），用于覆盖 `SuiteVisible=false` 但 `MySekaiVisible=true` 的账号场景；handler 层已不再直接调用 `GetMySekaiData()`

### 2.2 Service-Test 当前做法

`Service-Test` 的用户态模块依赖启动时加载的一组本地文件：

- `user.json`
- `music_metas.json`
- `mysekai.json`

其核心问题不是“文件格式不对”，而是“这是进程级单租户状态”，与 `Haruki-Cloud` 的多用户后端形态不兼容。

### 2.3 Haruki-Cloud 当前现状

当前 `Haruki-Cloud` 已有：

- `database/pjsk` 中的 `user_bindings` 与 `user_default_bindings`
- `database/users` 中的平台用户表
- `database/sekai` 中的主数据
- `/internal/pjsk/...` 的内部 render 路由

当前 `Haruki-Cloud` 还没有：

- 完整清完所有请求期 fallback 细节并把所有用户态模块都收口到统一 provider 语义

当前 `Haruki-Cloud` 已经有：

- `Snapshot` 统一快照接口
- `SnapshotProvider` 请求级解析接口
- `SnapshotFactory` 统一快照构造接口
- `ToolboxSnapshotProvider` 过渡实现
- `DefaultSnapshotFactory`，并已用于统一 `local_file` 与 Toolbox live 两条构造路径
- `FallbackSnapshotProvider`
- `MySekaiPayloadProvider`
- render controller 对快照的依赖开始从具体 `*userdata.Service` 收口到 `Snapshot` 接口

### 2.4 调用方现状

当前 `Haruki-ZeroBot` 在调用 Cloud 时会传：

- `im_user_id`
- `group_id`
- `command`
- `server`（可选，缺省时走默认 region）

但它现在没有显式传 `im_platform`。而 `pjsk.user_bindings.haruki_user_id` 指向的是 `users` 表内部主键，不是 QQ/IM 用户号本身。

这意味着正式方案里必须加入：

- `IdentityResolver`：先把 `im_platform + im_user_id` 解析成 `haruki_user_id`
- 再基于 `haruki_user_id + region` 解析默认绑定

## 3. 目标与非目标

## 3.1 目标

- 支持多用户并发，不允许进程级唯一用户态。
- 让 `profile`、`education`、`music progress/rewards`、`mysekai`、`deck` 等模块复用同一套 provider 抽象。
- 保留 `Service-Test` 中已经成熟的“从原始快照构造视图”的逻辑，但把“快照来源”换成 Cloud 内部实现。
- 在 provider 层隔离存储、身份解析、绑定解析和视图构建，避免 controller 直接感知底层存储细节。

## 3.2 非目标

- 不保留以本地 JSON 为核心的生产运行模式。
- 不在 render 请求里直接访问外部游戏服务或做重型同步。
- 不重新引入 `Service-Test` 的单用户全局 `UserDataService` 语义。
- 不因为 provider 设计而恢复旧 `/api/render` 兼容层。

## 4. 核心架构

建议拆成三层，而不是一个“大而全”的 provider：

1. `IdentityResolver`
2. `BindingResolver`
3. `SnapshotFactory`

调用链：

```text
render handler
  -> selector extractor
  -> IdentityResolver
  -> BindingResolver
  -> Toolbox live fetch
  -> SnapshotFactory
  -> controller / builder
```

职责划分：

- `IdentityResolver`
  - 输入 `im_platform + im_user_id`
  - 输出 `haruki_user_id`
- `BindingResolver`
  - 输入 `haruki_user_id + region + optional pjsk_user_id`
  - 输出最终目标 PJSK 账号
- `SnapshotFactory`
  - 复用 `Service-Test` 的解析/合并逻辑，构造 render 模块需要的视图

这样做的原因：

- 身份解析与快照读取不是一个问题
- 绑定关系与快照视图构造也不是一个问题
- 生产环境由 Toolbox 提供事实数据，Cloud 侧不需要额外维护用户快照镜像表

## 5. Provider 的输入模型

建议在 `internal/pjsk/render/userdata` 下定义统一 selector：

```go
type Selector struct {
    IMPlatform string
    IMUserID   string
    Region     renderregion.Value

    // 可选。仅允许定位到当前用户已绑定的目标账号，不允许任意越权查询。
    PJSKUserID string
}
```

规则：

- 常规路径：使用 `IMPlatform + IMUserID + Region` 查默认绑定
- 显式目标账号路径：使用 `PJSKUserID` 时，也必须验证该账号属于当前用户的绑定集合
- 不允许把任意 `user_id` 直接塞给 provider 并绕过绑定检查

## 6. 身份解析设计

建议新增 `IdentityResolver`：

```go
type IdentityResolver interface {
    ResolveHarukiUserID(ctx context.Context, platform string, userID string) (int, error)
}
```

默认实现：

- 查询 `database/users`
- 以 `(platform, user_id)` 唯一定位内部 `haruki_user_id`

关键点：

- `im_user_id` 不是 `haruki_user_id`
- `Haruki-ZeroBot` 需要补传 `im_platform`
- 在 ZeroBot/OneBot 场景里，当前可先约定 `im_platform=qq`
- 但 provider 设计本身不应写死 `qq`

## 7. 绑定解析设计

建议定义：

```go
type BindingRef struct {
    HarukiUserID int
    Region       renderregion.Value
    PJSKUserID   string
}

type BindingResolver interface {
    Resolve(ctx context.Context, selector Selector) (BindingRef, error)
}
```

解析逻辑：

- 如果未指定 `PJSKUserID`
  - 查 `pjsk.user_default_bindings`
  - 再取关联的 `pjsk.user_bindings`
- 如果指定了 `PJSKUserID`
  - 校验该 `(haruki_user_id, region, pjsk_user_id)` 确实存在于 `user_bindings`
- 默认 region 统一走 `renderregion.Value`

错误类型建议细分：

- 未注册 Haruki 用户
- 未设置默认绑定
- 指定账号未绑定
- region 非法

## 8. 快照存储设计（已废弃历史方案）

以下内容是曾经考虑过、但现已明确放弃的 Cloud 侧快照入库方案。
保留这里只为说明为什么仓库里一度出现过 `pjsk_user_snapshots` 相关实现；它不再是当前正式设计。

## 8.1 存储位置

建议把快照存储落在 `database/pjsk`，而不是 `database/users`。

原因：

- 快照是 PJSK 领域数据，不是通用用户账户数据
- 它天然与 `user_bindings` 同域
- `users` 更适合身份、风控、全局封禁等横切数据

## 8.2 存储键设计

快照应按 PJSK 游戏账号维度唯一，而不是按 Haruki 用户维度唯一。

推荐唯一键：

- `server`
- `pjsk_user_id`

不推荐按 `binding_id` 或 `haruki_user_id` 唯一，原因：

- 同一个游戏账号理论上可能被重复绑定
- 绑定关系变化不应复制一份原始快照

## 8.3 推荐表模型

建议新增类似 `pjsk_user_snapshots` 的表，字段可先保持简单：

- `id`
- `server`
- `pjsk_user_id`
- `main_snapshot_json`（对应原 `user.json`，由 `GetSuiteData()` 写入）
- `mysekai_snapshot_json`（对应原 `mysekai.json`，由 `GetMySekaiData()` 写入）
- `main_updated_at`
- `mysekai_updated_at`
- `source`
- `version`
- `created_at`
- `updated_at`

> `music_metas` 为全局区服数据，不属于用户快照，存储在独立的 `music_meta_cache` 表（唯一键: `region`）。

设计要点：

- 两份快照独立更新时间，避免强行要求同批次刷新
- `version` 用于缓存失效和后续增量升级

## 8.4 读写职责边界

render provider 只负责读。

快照写入建议由独立链路负责，例如：

- 内部同步任务
- 管理端上传
- 其他 Cloud 内部服务回填

不要把“抓取快照”塞进 render 请求链路，否则：

- 延迟不可控
- 错误面会扩散到所有渲染模块
- 很难做限流与重试

## 9. 运行时对象设计

建议 provider 暴露的不是裸 blob，而是一个统一快照对象：

```go
type Snapshot interface {
    RawBytes() ([]byte, error)
    MusicMetaBytes() ([]byte, error)
    DetailedProfile(region renderregion.Value) *drawing.DetailedProfileCardRequest
    ProfileCard(region renderregion.Value) *drawing.ProfileCardRequest
    MusicResults(diff string) map[int]string
    GetMusicResult(musicID int, diff string) string
    ChallengeLive() *ChallengeLiveData
}
```

这里的方向是：

- 对外保留 `Service-Test` 已经证明有价值的视图能力
- 对内不暴露全局单例，不允许 provider 自己偷偷缓存一份“当前用户”

`MySekai` 模块如果后续需要更完整的原始 map 访问，可以再加：

- `MergedMap() map[string]any`
- `RawMySekaiBytes() ([]byte, error)`

但这些应当以真正的模块需求为准，不要一次性把全部 `Service-Test` 细节原样搬过来。

## 10. 配置设计

`Haruki-Cloud` 中的 `user_snapshot` 配置建议改成下面的语义：

```yaml
pjsk_render:
  user_snapshot:
    provider: internal_cloud
    local_file:
      user_json: /path/to/user.json
      music_meta_json: /path/to/music_metas.json
      mysekai_json: /path/to/mysekai.json
```

约束：

- `internal_cloud` 为生产默认值
- `local_file` 只在测试或开发环境启用
- 不再把三条本地文件路径直接挂在生产配置根上作为默认方案

## 11. 路由与调用契约调整

为支持正式 provider，调用方需要补两个约束：

### 11.1 调用方参数

`Haruki-ZeroBot` 传给 Cloud 的参数应至少包括：

- `im_platform`
- `im_user_id`
- `server`

其中：

- `im_platform` 目前可先固定为 `qq`
- `server` 继续走现有 region 逻辑

### 11.2 用户态查询与绘图 payload 解耦

不建议把 `im_user_id`、`im_platform`、`pjsk_user_id` 直接塞进 Drawing payload body。

更合适的方式是：

- render handler 从请求参数或内部上下文提取 selector
- body 只保留模块业务查询字段

这样可以避免：

- 绘图请求模型被调用方身份语义污染
- `build` 返回给 drawing 的 payload 混入无关字段

## 12. 缓存建议

建议做两级缓存：

- 一级：原始快照记录缓存
  - key: `server + pjsk_user_id + version`
- 二级：解析后运行时对象缓存
  - key: `server + pjsk_user_id + version`

注意：

- 缓存 key 必须绑定 `version` 或更新时间
- 不能按 `haruki_user_id` 缓存最终快照，否则同账号多绑定时会出现重复副本

## 13. 错误与降级策略

建议 provider 统一返回结构化错误：

- `ErrIdentityNotFound`
- `ErrDefaultBindingNotFound`
- `ErrBindingNotFound`
- `ErrSnapshotNotFound`
- `ErrSnapshotCorrupted`

行为建议：

- 未绑定或无快照：快速失败，向调用方返回清晰可读错误
- 不要静默回退到空快照
- 不要在生产里偷偷改用本地 JSON

## 14. 测试策略

测试应分三层：

1. `IdentityResolver` / `BindingResolver` 单元测试
2. `SnapshotFactory` 对 `Service-Test` 视图逻辑迁移后的单元测试
3. render handler 集成测试

`local_file` provider 的定位：

- 单元测试 fixture
- 本地开发回归
- 不进入生产默认路径

## 15. 推荐实施顺序

1. 在 `Haruki-ZeroBot` 补传 `im_platform`。
2. 在 `Haruki-Cloud/internal/pjsk/render/userdata` 定义 selector、snapshot、snapshot provider、snapshot factory 接口。
   - 2026-04-09 进度：已落地
3. 以 Toolbox 作为生产事实来源，保留 `local_file` 仅作测试/联调 fallback。
   - 2026-04-09 进度：已落地
4. 继续迁移剩余 `music progress` 等路径到更统一的 snapshot/provider 语义。
5. 继续清理 `mysekai` 及其他仍保留的请求期 fallback 细节，评估是否还需要保留 raw mysekai payload 专用读链。
6. 最后处理 `deck auto recommend` 这类更重的用户态逻辑。

## 16. 最终建议

这件事的关键不是“把 `user.json` 从磁盘搬到数据库”，而是把 `Service-Test` 的单用户快照模型重建为 `Haruki-Cloud` 内部的正式多用户能力。

正式方案应当是：

- 身份解析走 `users`
- 默认绑定解析走 `pjsk.user_default_bindings`
- 原始快照事实来源保持在 `Toolbox`
- render runtime 通过只读 provider 获取统一快照对象
- 本地 JSON 只作为测试工具保留
