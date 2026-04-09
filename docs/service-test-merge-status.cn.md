# Service-Test 合并进度与收尾说明

> 2026-04-09 补充说明：本文描述的是 2026-03 合并阶段的落地结果与边界。文中提到的 `/internal/pjsk/*` internal render/build 路由属于当时的兼容运行时形态，现已从仓库与运行时中移除；当前主链请以 Bot 协议与 [项目完成度跟踪](project-completion-tracker.cn.md) 为准。

截至 2026-03-21，这轮 `Service-Test -> Haruki-Cloud` 合并任务在当前约定范围内可以视为完成。

这份文档用于说明：

- 当前已经完成了什么；
- 明确放弃了什么；
- 还有哪些内容是后续事项，但不属于本轮任务；
- 后续继续演进时应注意哪些边界。

## 1. 当前结论

- `Service-Test` 不再作为长期运行时服务保留。
- `Haruki-Cloud` 已成为当前 PJSK render/build 能力的承载位置。
- 不保留旧 `Service-Test` 运行时兼容层。
- 必须保留迁移与退役说明文档。
- `Haruki-ZeroBot`、`Haruki-OneBot` 不在本轮代码修改范围内。

## 2. 本轮已经完成的内容

### 2.1 渲染子系统已迁入 Haruki-Cloud

`Haruki-Cloud` 内已经建立新的 `internal/pjsk/render` 包树，并接入 `api/pjsk`。

已具备路由和实现的模块包括：

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

### 2.2 模块路由已经落地

当时标准内部路由位于 `/internal/pjsk/...` 下，典型形式为：

- `POST /internal/pjsk/<module>/<action>/build`
- `POST /internal/pjsk/<module>/<action>/render`

这意味着 `Service-Test` 原有模块化 render/build 能力已经可以在 `Haruki-Cloud` 内部直接使用。

### 2.3 统一分发入口已经落地

统一入口在当时也已经迁入 `Haruki-Cloud`：

- `POST /internal/pjsk/render`

当前统一分发入口采用 `Haruki-Cloud` 自己持有的请求契约：

```json
{
  "target": "card/detail",
  "operation": "build",
  "payload": {
    "query": "1001",
    "region": "jp"
  }
}
```

当前约束是：

- `target` 必须命中内部白名单
- `operation` 只允许 `build` 或 `render`
- 非法 JSON 和非法目标会直接失败
- 不保留旧 `/api/render` 的宽松吞错行为

### 2.4 区域值已经开始收口

渲染子系统内部已经引入专门的区域类型与常量，避免继续把 `"jp"` 之类的硬编码字符串散落在新代码里。

这部分不是对全仓库的一次性清扫，但对当前合并进来的渲染子系统已经形成了统一约定。

### 2.5 测试已经覆盖到当前合并结果

本轮新增或补齐了与渲染路由相关的测试，包括：

- 模块 build/render contract tests
- 统一分发入口测试
- 已迁模块的关键行为测试

当前验证结果已经通过：

- `go test ./api/pjsk ./internal/pjsk/render/...`

## 3. 本轮明确放弃的内容

这些内容是有意识地不做，而不是遗漏：

- 不保留 `Service-Test` 作为并行运行服务
- 不在 `Haruki-Cloud` 中保留旧 `/api/render`、`/api/card/...`、`/api/music/...` 路由别名
- 不修改 `Haruki-ZeroBot` 代码
- 不修改 `Haruki-OneBot` 代码
- 不保留 `Service-Test` 风格运行时兼容层

## 4. 当前仍然存在的临时实现

下面这些能力已经“接通”，但仍然是临时桥接方案，不应误认为已经完成最终收口。

### 4.1 本地用户快照桥接仍在使用

强用户态模块当前仍然依赖本地 JSON 快照桥接，而不是 Cloud 内正式 provider。

受影响的模块包括：

- profile
- music progress / rewards
- education
- deck auto recommend
- mysekai

当前实现特点：

- 仍依赖本地 `user.json`
- 可选读取 `music_metas.json`
- `mysekai` 会合并本地 `mysekai.json`

这套实现适合当前迁移阶段、测试和联调，但不是最终生产形态。

补充说明（2026-03-26）：

- `music_metas.json` 不再只是“本地调试时可选读取”的静态附件。
- 在当前 `Haruki-Cloud` runtime 中，`internal/pjsk/meta.Loader` 已成为区服级 `music_metas` 的主读取入口，并已被 `music chart` 技能预览链实际消费。
- 具体行为是：
  - `skill=false`：普通 chart，不读取 `music_meta`
  - `skill=true`：按 `region + music_id + difficulty` 从 `MetaLoader` 中选取对应条目，注入 chart payload
  - 若 `MetaLoader` 当前无缓存，再回退到 snapshot 的 `MusicMetaBytes()`
- 因此，`music_metas` 当前已经处于“公开区服静态数据缓存 + 本地 snapshot fallback”并行阶段，而不是纯粹的本地 JSON 临时文件。

再补充一层（Sekai API 公开资料，2026-03-26）：

- 当前 `Haruki-Cloud` 已不再只能依赖本地 `user.json` 为 `profile` 之外的模块提供头部资料。
- `internal/pjsk/handler/bridge.go` 已新增统一的公开资料构造逻辑：先通过绑定解析调用者 UID，再调用 `GetSekaiAPIClient().GetUserProfile()` 构造公开资料卡。
- 这套公开资料当前已复用到：
  - `music-list`
  - `music-progress`
  - `music-rewards`
  - `deck auto recommend` 的头部 profile
- 这意味着当前状态已经从“这些模块完全依赖本地 snapshot”推进到“资料卡部分可优先复用 Sekai API，主体数据仍依赖 snapshot / 私有数据”的混合阶段。

### 4.2 mysekai 仍使用本地 masterdata fallback

`mysekai` 当前已经迁入 `Haruki-Cloud`，但主要依赖本地 masterdata 目录。

这意味着：

- 现阶段已经可以运行
- 但还不是完全 DB/source 驱动的最终形态

### 4.3 deck auto recommend 已收口到 HTTP 外部服务

`deck` 当前主链已经具备以下能力：

- typed `recommend` build/render
- `recommend/auto` 通过外部 `deck-service` 做推荐

当前收口方式是：

- runtime 仅在配置了 `deck_recommend.service_base_url` 时启用 auto recommend
- 本地启发式 fallback 已退出主链
- `pjsk_deck_cgo` 目录已从仓库移除，不再参与正式运行时选路

当前主要风险转为：

- 外部 `deck-service` 可用性
- 本地 masterdata 目录完整性

## 5. 后续事项，但不属于本轮任务

### 5.1 正式用户快照 provider

后续如果要把强用户态模块真正收口，仍然需要实现 Cloud 内正式 snapshot provider。

这一步应当遵循已有设计拆分：

- `IdentityResolver`
- `BindingResolver`
- `SnapshotStore`
- `SnapshotFactory`

这一项已经单独写入设计文档，但不进入当前任务实现范围。

### 5.2 强用户态模块的“真实按用户生效”

当前 profile、education、mysekai、deck auto、music progress/rewards 已经迁入 `Haruki-Cloud`，但还不是“按调用用户动态解析数据”的最终形态。

后续真正收口时，需要把当前本地文件桥接替换为正式 provider，而不是继续扩展单进程本地快照方案。

### 5.3 deck 历史 native 残留清理

当前运行时已经固定走 HTTP 外部服务。`deck_cgo` 目录已经删除；后续如继续清理 deck 历史残留，建议重点放在：

- 旧阶段文档里对 `use_local_engine` / native 路径的描述是否统一标注为历史信息
- 避免新的配置样例或联调文档继续把本地 native 当作正式路径

### 5.4 调用方切换

后续如果要切换机器人或其他调用方，应直接切到 `Haruki-Cloud` 的内部路由契约。

这一步不在本轮代码修改范围内，只保留说明文档。

### 5.5 Sekai API 复用扩展的暂缓边界备忘录

截至 2026-03-26，`Sekai API` 公开资料复用已经安全推进到以下范围：

- `profile` 主链
- `card-list` / `card-box`
- `music-list` / `music-progress` / `music-rewards`
- `education-challenge`
- `mysekai` 的信息卡头部（`resource / fixture-list / door-upgrade / music-record / talk-list`）
- `deck auto recommend` 头部资料

这些模块的共同点是：**只需要替换头部资料卡**，不会改变主体数据来源，也不会把公开资料误当成私有抓包明细。

当前明确暂缓的是 `event` 方向，原因如下：

- `event` 现有 query / controller 结构没有像 `music`、`deck`、`education`、`mysekai` 那样预留 profile override 注入点。
- 一旦继续改，很容易从“只替换头部资料卡”滑到“改 event-record 主体数据构造”，边界会明显变糊。
- `lunabot` 在 `event`、`education`、`mysekai` 等模块里经常混用 `basic profile`、`detailed profile` 与私有字段；如果不先拆清“哪些字段只是头部资料，哪些字段属于主体业务数据”，直接照搬容易引入奇怪错误。

后续若要继续推进 `Sekai API` 线，建议顺序：

1. 先保持“只做头部资料卡复用”原则，不碰主体私有字段。
2. 若要进入 `event`，先单独给 `event` 设计 query-level profile override，而不是直接在 `event-record` 里硬塞公开 profile。
3. 只有在能够明确字段边界后，才考虑把 `education` / `mysekai` / `event` 里更深层的详细用户态字段迁出 snapshot。

结论：

- 当前这条线已经推进到一个相对安全的阶段。
- `event` 及更深层私有字段迁移属于**下一阶段单独任务**，不建议为了赶进度在本轮继续硬推。

### 5.6 lunabot 隐藏抓包逻辑对照备忘录

截至 2026-03-26，对 `lunabot` 当前“隐藏抓包 / suite / mysekai”链路的检查结论如下：

- `hide_suite` 的真正拦截点是 `get_detailed_profile(...)`
- 默认情况下，只要某模块依赖 `get_detailed_profile(...)` 且未显式 `ignore_hide=True`，用户隐藏抓包后该模块就会失败
- `mysekai` 使用独立的 `get_mysekai_info(...)`，并不直接受 `hide_suite` 控制
- `lunabot` 内部存在一批特例会显式 `ignore_hide=True`，包括部分：
  - `profile` 资料卡增强字段
  - `education`
  - `deck`
  - `mysekai` 中借 suite 读取的字段
- `music-rewards` 是一个较清晰的样板：
  - 有 suite 时读取 `userMusicAchievements` 做精确计算
  - 没有 suite 时退化为 `basic profile` + `userMusicDifficultyClearCount` 的公开估算

这说明 `lunabot` 并不是统一的“隐藏抓包后自动进入估算模式”，而是多种策略并存：

- `suite exact`
- `suite exact but ignore_hide`
- `public basic`
- `public inferred`
- `mysekai private`

对 `Haruki-Cloud` 的直接启发是：

1. `music-rewards` 这类模块适合先落地“精确 / 估算”双分支
2. `event`、`education` 深层字段、`deck` 算法主体、`mysekai` 主体数据不应仓促照搬
3. `Haruki-Cloud` 当前已开始把 `suite` 与 `mysekai` 的可用性控制拆成两套语义；后续只需继续沿这条方向补齐剩余文档和边界约束，不应再回退到共用 `suite_visible`

## 6. 后续开发时的注意事项

- 不要重新把旧 `/api/render` 契约写回 `Haruki-Cloud`。
- 不要因为调用方改造成本高，就恢复长期兼容层。
- 不要继续扩展基于单一本地 `user.json` 的长期运行方案。
- 新增 render 模块时，优先接入 `/internal/pjsk/<module>/...` 和 `/internal/pjsk/render`，不要再创造第二套内部协议。
- 如果某些 `Service-Test` 旧实现确实更成熟，应明确写清“采用旧方案的哪一部分”，而不是整体回搬。
- `Service-Test` 退役后，保留的是文档，不是运行时依赖。

## 7. 相关文档

当前建议一起保留以下文档：

- `service-test-merge-plan.cn.md`
- `pjsk-user-snapshot-provider-design.cn.md`
- `zerobot-render-followup.cn.md`

其中分工是：

- `service-test-merge-plan.cn.md` 保留历史方案与阶段划分
- `service-test-merge-status.cn.md` 记录当前实际落地结果与收尾结论
- `pjsk-user-snapshot-provider-design.cn.md` 保留正式 provider 设计
- `zerobot-render-followup.cn.md` 保留机器人后续接入说明

## 8. 收尾结论

如果按当前明确范围来衡量，这轮任务已经完成：

- `Service-Test` 的主要渲染能力已迁入 `Haruki-Cloud`
- 统一分发入口已经落地
- 旧兼容层明确放弃
- 后续事项已转化为文档

后续继续推进时，应将重点放在：

- 正式用户快照 provider
- 强用户态模块去本地 JSON 化
- 调用方切换

这些属于下一阶段工作，不属于本轮合并收尾范围。
