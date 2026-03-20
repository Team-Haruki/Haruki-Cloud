# Service-Test -> Haruki-Cloud 合并方案

## 1. 结论

这次合并应按“将业务逻辑迁入 `Haruki-Cloud`、按照 `Haruki-Cloud` 的约定重建传输层、默认不保留代码级 `Service-Test` 兼容层”的思路推进。

不要把 `Service-Test/cmd/server/main.go` 原样复制进 `Haruki-Cloud/cmd/server`。这两个项目在服务生命周期、路由风格、配置结构和职责边界上已经明显分化。

建议的最终形态：

- `Haruki-Cloud` 保持为唯一的服务进程。
- `Haruki-Cloud` 成为唯一持有 PJSK render/build API 的位置。
- `Service-Test` 中的 controller / builder / source 逻辑迁入 `Haruki-Cloud` 内部新的 PJSK 渲染子系统。
- `Service-Test` 仅作为迁移来源存在，切换完成后不应继续作为长期并存服务、永久兼容运行时或重复代码镜像保留。
- `Haruki-Cloud` 里已经存在的基础能力直接复用：
  - `database/sekai`
  - `utils/drawing`
  - `utils/query`
  - `api` 响应与中间件约定
- 长期需要保留的是 `Haruki-Cloud/docs` 下的迁移/退役说明文档，而不是旧服务本体。

## 2. 当前状态梳理

### 2.1 Service-Test

观察到的特征：

- 独立的 `net/http` 服务，基于 `http.NewServeMux`。
- `cmd/server/main.go` 中有 `96` 个路由注册和 `78` 个 handler 函数。
- 内部分层已经不算简单：
  - `internal/controller`
  - `internal/builder`
  - `internal/service`
  - `pkg/masterdata`
  - `pkg/asset`
  - `pkg/deck_cgo`
- 通过 `go.mod` 里的本地 `replace` 依赖 `haruki-cloud`。
- 使用 `haruki-cloud/database/sekai` 时，仅把它当作数据源依赖，而不是宿主运行时。
- 自己还维护了一套额外的本地文件运行时：
  - masterdata JSON 目录
  - asset 目录
  - `user.json`
  - `music_metas.json`
  - `mysekai.json`
  - 可选的 deck recommendation backend / CGo engine
- 暴露两类 API 形态：
  - 模块级 `build` / `render` 接口
  - 统一入口 `POST /api/render`

当前主要功能模块：

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

观察到的特征：

- 单一 Fiber 服务，入口在 `cmd/server/main.go`。
- 通过 `haruki-db-configs.yaml` 做集中式配置加载。
- 现有公开 API 风格是 `/<module>/...`，配合 `api/helper.go` 中的统一 JSON envelope。
- 现有内部/私有接口已经具备中间件和鉴权机制。
- `api/pjsk` 目前只暴露 alias 相关接口。
- `utils/query` 已经承担了给调用方使用的进程内查询工具职责。
- `utils/drawing/models.go` 已经定义了大部分 Drawing API 请求结构，而这些结构在 `Service-Test/internal/model` 中又被重复定义了一遍。
- `utils/drawing/client.go` 已经提供了 Drawing API client，而这与 `Service-Test/internal/service/drawing.go` 职责重叠。
- 仓库内已经包含 `database/sekai`，但 `cmd/server` 当前并不会初始化 Sekai DB client。
- 现在的 `config.PJSK` 实际表示的是 alias / binding 数据库，而不是 Sekai masterdata 数据库。

### 2.3 两者之间的结构关系

当前关系可以概括为：

- `Service-Test` 是一个上层应用。
- `Haruki-Cloud` 已经为它提供了部分基础设施。
- `Service-Test` 又在这层基础设施之上额外叠加了一套领域模型和一套传输层。

这意味着，这次合并的重点其实是消除重复层，而不是简单挪文件。

### 2.4 调用方复查结论

这次复查中，我额外检查了当前工作区内可能的调用方，包括：

- `Haruki-ZeroBot`
- `Haruki-OneBot`
- `Haruki-Cloud`
- `HarukiBot-Docs`

没有发现现有代码继续显式依赖 `Service-Test` 旧 `/api/...` 路径的证据，也没有发现现有机器人代码直接绑定 `/api/render` 的实现。

这意味着：

- 方案里不应默认保留一整套应用内 `Service-Test` 兼容层
- 如果未来确认存在工作区之外的历史调用方，应优先使用短期、时限明确的网关/反向代理重写，而不是把旧接口永久留在 `Haruki-Cloud` 代码中

## 3. 需要解决的主要冲突

### 3.1 服务与路由风格冲突

- `Service-Test` 使用 `net/http`，并直接返回原始 PNG / 原始 JSON。
- `Haruki-Cloud` 使用 Fiber，并为常规 API 采用统一 JSON envelope。
- `Service-Test` 路由使用 `/api/...`。
- `Haruki-Cloud` 路由按产品域组织，例如 `/pjsk/...`。

意味着：

- 业务逻辑和 HTTP handler 必须在迁移时拆开。
- 路由兼容必须显式设计，不能靠“顺手保留”。

### 3.2 模型重复

目前存在两套重叠的模型体系：

- `Service-Test/internal/model/*`
- `Haruki-Cloud/utils/drawing/models.go`

还存在一层重复的游戏领域模型：

- `Service-Test/pkg/masterdata/*`
- `Haruki-Cloud/database/sekai/*` 实体及其周边查询包

意味着：

- 如果合并后仍然保留这两套重复模型，代码库会比现在更难理解。
- 合并的目标应当是减少重复，而不是把重复整体搬过去。

### 3.3 数据源分裂

`Service-Test` 当前支持两类数据后端：

- 本地 masterdata JSON
- 通过 `*_cloud_source.go` 访问 `Haruki-Cloud` Sekai DB

`Haruki-Cloud` 当前已具备：

- `database/pjsk`，用于 alias / binding / preference 数据
- `database/sekai`，用于代码生成和 DB 访问
- 但 `cmd/server` 并没有初始化 Sekai DB client

意味着：

- 合并后的 `Haruki-Cloud` 必须新增 Sekai DB 的显式配置和生命周期管理
- 现有 `PJSK` 配置块不能直接复用来承载这件事

### 3.4 用户数据模型不匹配

这是产品层面最大的冲突。

`Service-Test` 当前可以分成两类模块：

主要依赖 masterdata 的模块：

- card
- music detail/list/chart
- gacha
- event
- honor
- stamp
- misc
- score
- sk

强依赖本地用户快照的模块：

- profile
- music progress / rewards
- education
- deck auto recommend
- mysekai
- card/music 中部分与用户态装饰相关的逻辑
- 统一 `/api/render` 中涉及用户态的分支

目前这些模块的行为本质上是进程级单租户，因为 `UserDataService` 启动时只加载一套本地文件。

而 `Haruki-Cloud` 是多用户后端，这种模式不能作为最终状态保留。

意味着：

- 不先引入新的用户快照抽象，这些模块就不能安全迁移
- 迁移顺序必须把“近似无状态模块”和“强用户态模块”分开

### 3.5 Deck 引擎打包风险

`Service-Test/pkg/deck_cgo` 当前会在缺失本地原生库时让默认 CGo 构建直接失败。

意味着：

- 这部分绝不能作为无条件的构建依赖直接并入 `Haruki-Cloud`
- deck 的 CGo 路径必须变成显式 opt-in

## 4. 推荐目标架构

### 4.1 高层形态

保持 `Haruki-Cloud` 作为宿主应用，并新增一个专门的内部渲染子系统：

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

职责划分：

- `api/pjsk/*`
  - HTTP 路由注册
  - Fiber 请求绑定与响应输出
  - 鉴权与兼容路由
- `internal/pjsk/render/*`
  - 迁入后的 `Service-Test` 业务逻辑
  - controllers
  - builders
  - data source adapters
  - user snapshot providers
  - 可选的本地 masterdata fallback
- `utils/drawing/*`
  - 作为 Drawing API 请求模型和 client 的标准实现
- `database/sekai/*`
  - 作为 Sekai DB 访问的标准实现

### 4.2 合并后哪些实现应成为标准

应以以下实现作为标准版本：

- Drawing payload structs: `Haruki-Cloud/utils/drawing/models.go`
- Drawing client: `Haruki-Cloud/utils/drawing/client.go`
- Sekai DB client: `Haruki-Cloud/database/sekai`
- Cloud server/runtime/config/middleware: `Haruki-Cloud`

迁移过程中可以暂时接受的过渡组件：

- 将 `Service-Test/pkg/masterdata` 暂时迁到 `internal/pjsk/render/masterdata`
- 保留一批 cloud source adapter，继续把 ent entity 转成这套临时模型

但这些都应该是过渡方案，而不是长期目标。

### 4.3 推荐路由策略

`Haruki-Cloud` 内部建议的标准路由：

- 公开数据接口继续走 `/pjsk/...`
- render/build 接口作为内部 API，例如：
  - `POST /internal/pjsk/render`
  - `POST /internal/pjsk/card/detail/build`
  - `POST /internal/pjsk/card/detail/render`
  - `POST /internal/pjsk/music/detail/build`
  - 等等

推荐策略是：

- 不在 `Haruki-Cloud` 应用代码中默认保留 `Service-Test` 的 `/api/...` 路由别名
- 统一以新的 `Haruki-Cloud` 路由和鉴权约定为准
- 对内部 render 接口统一挂上 `api.VerifyAPIAuthorization()`

如果后续确认存在工作区之外的历史调用方：

- 只允许使用短期、带明确下线日期的入口层重写
- 不建议在 `Haruki-Cloud` 内部再保留一套长期 `/api/render`、`/api/card/...`、`/api/music/...` 路由

这样做的好处：

- 避免把即将废弃的旧传输契约继续固化进新代码库
- 新工作可以直接与 `Haruki-Cloud` 的现有路由风格保持一致
- 避免将一大批纯渲染接口默认暴露到无鉴权公开面上

### 4.4 应明确采用的 Service-Test 方案

并不是 `Service-Test` 的所有内容都应该丢弃。以下设计和实现值得明确吸收：

- 区域感知的数据源注册与选择模式。
  - 例如 `CardController`、`MusicController`、`EventController` 中的 `RegisterSource`、`resolveRegion`、`sourceForRegion` 这一组模式，适合直接迁入新 render 子系统。
- `pkg/asset/asset_helper.go` 的多根目录资源解析逻辑。
  - 这套 `primary + legacy roots + first existing` 的路径解析方案是成熟的，本地资源回退能力也比 `Haruki-Cloud` 现状更完整。
- 基于 `database/sekai` 的 cloud source adapter 分层。
  - `*_cloud_source.go` 这层把数据库访问和上层 builder/controller 逻辑隔开，迁移后仍然有价值。
- controller -> builder -> source 的业务分层。
  - 相比把所有逻辑堆进 Fiber handler，这一层次在 `Service-Test` 中已经比较清楚，适合作为 `Haruki-Cloud` 的内部实现结构。
- 统一分发入口的“思想”，但不是旧接口契约。
  - `cmd/server/render_dispatch.go` 的统一模块分发思路可保留；
  - 旧的 `/api/render` 路径、宽松参数校验和静默吞错行为则不保留。
- 现有测试样式。
  - 例如 `internal/controller/event_controller_test.go` 这种按 source stub 做区域选择断言的测试方式，值得直接沿用。

应明确舍弃的 `Service-Test` 内容：

- `cmd/server/main.go` 和整套 `net/http` handler 传输层
- 重复的 `internal/model/drawing_request.go` / `internal/service/drawing.go`
- 进程级唯一的 `UserDataService` 运行模式
- 默认开启、缺库即构建失败的 deck CGo 路径
- 没有证据支撑时继续保留的旧 `/api/...` 路由契约

## 5. 包级迁移映射

| Service-Test 来源 | Haruki-Cloud 目标 | 动作 |
| --- | --- | --- |
| `cmd/server/main.go` | `api/pjsk/render_route.go` + `cmd/server/main.go` 初始化接线 | 将业务启动逻辑与 HTTP 传输层拆开 |
| `cmd/server/render_dispatch.go` | `api/pjsk/render_dispatch.go` | 保留功能，但按 Fiber 和新的 app container 重写 |
| `internal/controller/*` | `internal/pjsk/render/controller/*` | 第一阶段基本原样迁入，之后再清理传输层残留 |
| `internal/builder/*` | `internal/pjsk/render/builder/*` | 第一阶段基本原样迁入 |
| `internal/service/*_cloud_source.go` | `internal/pjsk/render/source/*` | 保留 adapter 角色，但由 Haruki-Cloud 托管 |
| `internal/service/*_data_source.go` | `internal/pjsk/render/source/*` | 第一阶段保留接口层 |
| `internal/service/drawing.go` | 使用 `utils/drawing/client.go` | 不再保留第二套 drawing client |
| `internal/model/drawing_request.go` | 使用 `utils/drawing/models.go` | 用标准 drawing models 替代 |
| `internal/model/query.go` | `api/pjsk/render_struct.go` 或 `internal/pjsk/render/request/*` | 仅保留入站 query/request 类型 |
| `internal/model/score_request.go` | 能复用的地方直接用 `utils/drawing/models.go` | 删除重复的输出/请求模型 |
| `internal/apiutils/cloud_clients.go` | `cmd/server/main.go` 中的 Sekai DB 初始化 + app container | 去掉重复的 DB init wrapper |
| `internal/config/config.go` | `Haruki-Cloud/config/config.go` | 合并进新的配置分区 |
| `pkg/asset/*` | `internal/pjsk/render/assets/*` 或 `utils/assets/*` | 迁入并保留本地路径解析逻辑 |
| `pkg/masterdata/*` | `internal/pjsk/render/masterdata/*` | 作为过渡期内部领域模型，后续逐步收敛到 `database/sekai` 周边标准层 |
| `internal/service/masterdata*.go` | `internal/pjsk/render/masterdata/*` | 保留本地 JSON fallback，但作为可选 provider |
| `pkg/deck_cgo/*` | `internal/pjsk/render/deck/*` | 保持可选并加 build tag |
| `data/*` | `internal/pjsk/render/static/*` 或 `assets/pjsk/*` | 把 deck 相关静态数据迁入 Haruki-Cloud |
| `cmd/dbprobe/main.go` | 可选迁为 `cmd/sekai-dbprobe/main.go`，或直接丢弃 | 仅在仍然有运维/调试价值时保留 |

## 6. Haruki-Cloud 需要新增的配置

应新增独立的 Sekai 运行时配置，而不是复用现在的 `PJSKConfig`。

建议结构：

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

关键点：

- `Haruki-Cloud` 当前的 `config.PJSK` 是 alias / binding 数据
- 合并后的渲染系统需要的是新的 Sekai masterdata DB 生命周期，以及独立的 render 配置块

## 7. 推荐迁移顺序

### Phase 0: 建基线并冻结

- 在开始迁文件前，先冻结 `Service-Test` 当前行为。
- 记录现有路由清单和 payload contract。
- 将现有 `Service-Test` 单元测试纳入迁移待办。
- 为以下内容补一份短期 contract 清单：
  - request JSON
  - `build` 接口返回 JSON
  - HTTP 状态码行为
  - 二进制渲染行为

这一阶段的产出：

- 明确支持的 endpoint 列表
- 明确是否真的存在必须保留的历史外部调用方；若没有，则不做应用内兼容层

### Phase 1: 抽基础层

- 在 `Haruki-Cloud/cmd/server/main.go` 中新增 Sekai DB 配置和初始化。
- 引入 `internal/pjsk/render` 包树。
- 把 `pkg/asset` 迁入新的内部渲染子系统。
- 把 `pkg/masterdata` 与本地 masterdata loader 一起迁入新的内部渲染子系统，作为临时兼容层。
- 将 `Service-Test/internal/service/drawing.go` 替换为 `utils/drawing/client.go`。
- 将重复的 drawing request models 替换为 `utils/drawing` 类型或其别名。

退出条件：

- `Haruki-Cloud` 可以启动一个 render app container，但暂时还不暴露路由
- `cmd/server` 中不再直接复制 `Service-Test` 的传输层代码

### Phase 2: 先迁无状态 / 低耦合模块

优先迁这些模块：

- gacha
- event
- honor
- stamp
- misc
- score
- sk
- music detail/list/chart
- card detail/list/box

原因：

- 这些模块本质上主要是 DB/masterdata + asset + drawing 的转换
- 不需要先解决多用户快照语义

工作内容：

- 迁移 controller / builder / source 代码
- 在 `api/pjsk` 中补 Fiber handlers
- 为 build payload 补 contract tests

退出条件：

- 这些模块已经能在 `Haruki-Cloud` 内运行
- 对这些模块而言，旧 `Service-Test` 已经不再是必要依赖

### Phase 3: 统一分发入口

- 将统一分发逻辑迁入 `api/pjsk/render_dispatch.go`，标准入口定为新的 `Haruki-Cloud` 内部路由，而不是继续沿用旧 `/api/render` 契约。
- 把入站命令 payload 重定义为 `Haruki-Cloud` 自己持有的结构。
- 在迁移期间统一参数校验行为，不要继续保留静默吞掉 JSON 反序列化错误的做法。

这一阶段建议顺手修正的行为：

- `Params` JSON 非法时，应快速失败并返回清晰错误
- 不要再继续用零值 struct 往下执行

退出条件：

- command-parser / bot 集成可以直接调用 `Haruki-Cloud` 中已迁移的模块

### Phase 4: 用户快照抽象

引入正式的 provider 接口，例如：

```go
type SnapshotProvider interface {
    LoadByBinding(ctx context.Context, harukiUserID int, server string) (*UserSnapshot, error)
    LoadByRawSource(ctx context.Context, source SnapshotSource) (*UserSnapshot, error)
}
```

建议的第一版实现：

- `LocalFileSnapshotProvider`

建议的目标实现：

- 与 Haruki user / binding 上下文绑定的存储型或服务型 snapshot provider

在以下模块真正完成合并前，这一阶段是必需的：

- profile
- music progress / rewards
- education
- deck auto recommend
- mysekai

退出条件：

- 强用户态模块不再依赖进程级唯一的 `user.json`

### Phase 5: 迁移强用户态模块

在用户快照 provider 就位之后，再迁移：

- profile
- education
- mysekai
- deck
- music progress / rewards

特别说明：

- 当前 `Service-Test` 里的 `profile` 表面上接受 `user_id`，但实际上只读取启动时加载的本地用户数据
- 这个问题应该在迁移时修正，而不是原样保留

退出条件：

- 每用户行为是真正的按用户生效，而不是由单一本地文件伪装出来

### Phase 6: Deck 引擎收口

- 将 deck CGo engine 放到显式 build tag 或显式 opt-in package 后面。
- 默认 `Haruki-Cloud` 构建必须在没有原生库的情况下也能成功。
- HTTP backend recommendation 路径应保持为默认、安全的运行时路径。

建议的技术方向：

- 引入类似 `pjsk_deck_cgo` 的 build tag
- 只有在显式要求时才编译本地原生 deck bindings

退出条件：

- `go test ./...` 和默认构建在没有原生 deck 产物时也能通过

### Phase 7: 切换与清理

- 将调用方从 `Service-Test` 基础地址切换到 `Haruki-Cloud`。
- 默认不在应用内保留兼容路由。
- 如果确有工作区之外的历史调用方，使用入口层短期重写，并附带明确下线日期。
- 经过生产环境观察后：
  - 将 `Service-Test` 从活跃部署和活跃代码职责范围中移除
  - 在 `Haruki-Cloud/docs` 中保留迁移与退役说明文档
  - 删除迁移过程中遗留的重复模型、适配层和任何过渡兼容代码

## 8. 测试策略

### 8.1 优先复用什么

优先迁这些测试：

- `Service-Test` 的 builder tests
- `Service-Test` 的 controller tests
- `Haruki-Cloud` 风格的 `/pjsk` 路由测试

### 8.2 需要新增什么

建议补三层测试：

- builders 和 source adapters 的单元测试
- Fiber endpoint 的路由测试
- 集成测试，配合：
  - Sekai DB test client
  - 可选的 fake drawing server

### 8.3 要断言什么

对于 render/build 迁移，重点断言：

- 请求校验
- region 路由
- data source 选择
- 输出 payload JSON
- HTTP 状态码行为

相比直接断言 PNG 字节，更推荐断言 drawing payload JSON。

## 9. 需要尽早拍板的风险与决策

### 9.1 用户快照来源

待定问题：

- `Haruki-Cloud` 是否要自己托管用户快照存储？
- 还是继续从其他系统接收预构建的 snapshot JSON？

这个决策会直接影响：

- profile
- deck
- education
- mysekai
- music progress/rewards

### 9.2 路由兼容范围

待定问题：

- 是否需要在 `Haruki-Cloud` 应用代码中保留旧的 `/api/...` 路由？

建议答案：

- 默认不保留
- 只有在确认存在历史外部调用方时，才允许通过短期入口层重写过渡

### 9.3 本地 masterdata fallback

待定问题：

- `Haruki-Cloud` 在生产环境中是否还要保留本地 JSON masterdata fallback？

建议答案：

- 保留为可选 fallback 或开发工具
- DB 驱动的 Sekai source 应作为生产环境标准路径

### 9.4 Deck 原生引擎

待定问题：

- 原生 deck recommendation 是否是生产环境必须依赖？

建议答案：

- 默认不是
- 应保持可选、隔离

## 10. 推荐的第一批实施切片

如果目标是在可控风险下尽快拿到有价值进展，建议按下面顺序实现：

1. 为 `Haruki-Cloud` 增加 Sekai DB 运行时配置和 client 初始化。
2. 创建 `internal/pjsk/render` 包树。
3. 先迁 asset helper、source interfaces 以及无状态模块。
4. 为 card/music/gacha/event/honor/stamp/misc/score/sk 暴露 Fiber 路由。
5. 为这批第一阶段模块迁 unified dispatch，但直接采用新的 `Haruki-Cloud` 路由契约。
6. 设计并实现 user snapshot provider。
7. 再迁 profile/education/mysekai/deck，以及剩余依赖用户态的 music 流程。

这样可以避免整次合并被最难的一部分拖住。

## 11. 简短总结

正确的合并方式不是：

- “把 `Service-Test` 整个复制进 `Haruki-Cloud`”

正确的合并方式是：

- 保持 `Haruki-Cloud` 作为宿主
- 把 `Service-Test` 的领域逻辑迁入新的内部 PJSK 渲染子系统
- 复用 `database/sekai` 和 `utils/drawing`
- 明确吸收 `Service-Test` 中更成熟的 source 抽象、区域切换、资源路径解析与测试方式
- 增补真正的 Sekai DB 运行时和真正的用户快照抽象
- 先迁无状态模块
- 再在数据模型修正之后迁强用户态模块
- 默认不保留旧 `Service-Test` 的应用内兼容路由层
- 在切换完成后退役并舍弃旧 `Service-Test` 服务，但保留迁移说明文档

这样得到的结果会与 `Haruki-Cloud` 当前架构保持一致，而不是在里面再嵌一套第二服务器。
