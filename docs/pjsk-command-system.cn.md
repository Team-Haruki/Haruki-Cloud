# Haruki-Cloud PJSK 指令系统设计

> 最后更新：2026-03-26
>
> 本文档描述的是当前已落地的主模型。若后续实现发生变化，应以代码和本文档同步更新后的内容为准。

## 1. 核心结论

PJSK 指令系统的主链路应当是“端点中心”模型，而不是“云端全局重新选路”模型。

也就是说：

1. 云端负责下发命令规则。
2. 客户端负责基于规则做前缀树匹配。
3. 客户端命中后，按 `path` 调用对应的 Bot 端点，并上传 `matched_command`。
4. 云端只校验 `matched_command` 是否属于当前端点。
5. 当前端点对应的 handler 再根据原始文本做详细解析。
6. 解析成功后进入统一执行链路。
7. 如果 `matched_command` 与端点不一致，或原文无法被该 handler 继续处理，则返回错误。

## 2. 主链路

当前目标主链路如下：

```text
Haruki-Cloud 下发 command_manifests
  -> Haruki-ZeroBot 构建本地前缀树
  -> 本地匹配到 command_module + command_path + matched_command
  -> 请求 /api/v2/bot/:botId/<module>/<path>
  -> 上传 command_payload 查询参数 + matched_command 请求头
  -> 端点校验 matched_command -> handler.path
  -> 命中 handler 后在该 handler 内解析原文参数
  -> 调用统一执行链路
  -> 返回 PNG 或文本
```

当前 PJSK 这批端点中，`command_module` 固定为 `pjsk`，因此实际请求路径仍然是：

`/api/v2/bot/:botId/pjsk/<path>`

这个模型下，客户端决定“请求哪个候选端点”，云端端点决定“这个候选端点是否真的成立”。

## 3. 各层职责

### 3.1 命令规则层

涉及：

- `internal/pjsk/handler/`
- `api/bot/pjsk/seed.go`
- `api/bot/pjsk/handler.go`
- `command_manifests`

职责：

1. 定义可对外暴露的 Bot 业务端点。
2. 为每个端点维护命令前缀集合。
3. 生成给客户端使用的 manifest。

这里的 `path` 是客户端命中后要请求的端点路径，不是云端内部 render target。

当前实现中，manifest 的命令前缀优先来源于 `internal/pjsk/handler/sekai` 中带 `GetPath()` 的 handler。

当前代码已经开始按这个目标收口：

1. `internal/pjsk/handler` 成为 `path` 的唯一事实来源
2. `api/bot/pjsk` 已改为消费 handler registry
3. manifest 与 Bot HTTP 路由注册都由同一份 route 数据派生

### 3.2 客户端命中层

这一层不在 `Haruki-Cloud` 仓库内实现，但它决定了云端接口边界。

职责：

1. 拉取 `GET /api/v2/bot/:botId/command/manifests`
2. 构建本地前缀树
3. 命中对应 `command_path`
4. 请求对应 Bot 端点

客户端只做“候选端点选择”，不做最终业务解释。

### 3.3 Bot 端点层

涉及：

- `api/bot/pjsk/handler.go`

职责：

1. 校验 Bot session
2. 从 `command_payload` 恢复 OneBot 消息段
3. 构造 `HandlerContext`，提取纯文本参数与 `at` 列表
4. 校验 `matched_command` 是否属于当前端点
5. 使用对应 handler 在当前端点语义范围内解析原文
6. 提取当前端点所需参数
7. 若 `matched_command` 或原文不成立，返回 `400`
8. 若解析成立，调用后续处理链路

关键点是：

Bot 端点不应该再把“请求发到哪个端点”这个问题重新交给一个全局路由器决定。

### 3.3.1 消息段与 `@` 信息

当前 Bot 入口对 `command_payload` 的处理方式是：

1. 优先读取 OneBot `message` 段数组
2. 若没有段数组，再回退 `message` 字符串或 `raw_message`
3. `HandlerContext` 从消息段中提取纯文本参数与 `at` 列表

当前 `at` 信息只识别 OneBot `at` 段中的 `qq` 字段。

### 3.3.2 通用账号指定参数

当前 `sekai` handler 通用支持三类账号指定参数：

1. `u[i]`，例如 `u1`
2. 游戏 UID，例如 `12345678901234`
3. `@qq`，包括文本中的 `@123456789` 和消息段中的 `at.qq`

这些参数由 `Extractor.ExtractUid` 统一提取，并写入 `SekaiHandlerContext.uidArg`。

默认情况下：

1. `SekaiCommandHandler.ParseUIDArg` 视为开启
2. 提取顺序为 `u[i] -> uid -> @qq`
3. 若消息段中存在真实 `at.qq`，会作为最终账号选择器覆盖文本解析结果

绑定、解绑、主账号等需要自行解释 `u1` / UID 的命令，会显式关闭这层通用提取。

### 3.4 参数解析层

涉及：

- `internal/pjsk/parser/extractor.go`
- `internal/pjsk/parser/parser.go`
- `internal/pjsk/parser/music_parser.go`
- `internal/pjsk/parser/event_parser.go`
- 其他类型化解析器

职责：

1. 提供通用提取能力，例如区服、角色、属性、稀有度、年份、难度
2. 提供面向具体业务域的类型化解析器
3. 为具体端点服务，而不是替端点决定目标模块
4. `Extractor.ExtractUid` 用于提取 `u[i]`、游戏 UID、`@qq`

这些解析器是应该保留的。

### 3.5 执行与渲染层

涉及：

- `internal/pjsk/handler/`
- `internal/pjsk/render/`
- `/internal/pjsk/render`
- `/internal/pjsk/<module>/<action>/build|render`

职责：

1. 接收已经提取好的业务参数
2. 调用对应 controller / render runtime
3. 返回统一的执行结果，而不是让上游自己猜测返回类型

这一层不负责前缀树命中，也不负责 Bot 业务端点选路。

需要额外明确：

1. `sekai/*Handle()` 与其他 handler 一样，只负责解析原文并产出 `makeResolvedCmd(...)` 或 `makeResolvedCmdWithParams(...)`
2. handler 不应在自身内部直接完成绑定、解绑、默认绑定切换等业务执行
3. 文本型结果与图片型结果都必须经过 `commandhandler.Execute(...)`
4. `refer/profile.py` 仅作为语义参考，不作为 Cloud 分层实现方式的直接模板

## 4. `GlobalCommandResolver` 的定位

`GlobalCommandResolver` 这类“输入原文后直接给出 module + mode”的全局选路器，不应再作为 Bot 主协议的核心。

更准确的定位应当是：

1. 可作为内部通用入口的兼容能力
2. 可作为迁移期的辅助实现
3. 可用于测试、调试、内部工具

但它不应定义客户端联调的主链路。

## 5. `handler` Trie 的定位

`internal/pjsk/handler` 中的 Trie 不再负责“拿到原文后为 Bot 主链路重新选路”，但它仍然是当前协议中的关键组件。

当前职责应当是：

1. 作为命令注册表，维护 `command -> handler -> path`
2. 作为 manifest 的命令来源之一
3. 在 Bot 端点收到 `matched_command` 后，定位对应 handler
4. 由该 handler 对原文继续做详细解析

也就是说：

Trie 仍然保留，但不再承担“替客户端决定访问哪个端点”的职责。

## 6. 端点应如何解析原文

以 `card/detail` 为例，目标行为应是：

1. 客户端命中 `card/detail`
2. 请求 `/api/v2/bot/:botId/pjsk/card/detail`
3. 客户端同时上传 `command_payload` 和 `X-Haruki-Bot-Matched-Command`
4. 云端先检查这个 `matched_command` 是否属于 `card/detail`
5. 命中该 path 的 handler 后，再按当前 handler 规则解析 `command_payload` 中的原始命令
6. 若 `matched_command` 不属于该 path，则返回 `400`
7. 若 handler 成功产出最终 render 参数，则继续处理

也就是说，端点内部解析仍然由当前 path 绑定的 handler 负责，而不是再把原文交给一个全局路由器重新决定端点。

## 7. `command_manifests` 的正确含义

`command_manifests` 当前的正确含义是：

1. 客户端前缀树规则表
2. 客户端候选端点选择表
3. Bot 业务端点元数据表

它不是：

1. 云端最终 render target 表
2. 云端内部 parser 的替代物
3. 客户端最终语义裁决结果

## 8. 当前应保留的内容

下面这些内容符合目标模型，应保留：

1. `/api/v2/bot/:botId/pjsk/*` 直达型端点
2. `command_manifests` 及其 seed 机制
3. `internal/pjsk/handler` 中带 `GetPath()` 的命令注册体系
4. `internal/pjsk/parser` 中的通用提取器和类型化解析器
5. render runtime 与模块化渲染路由

需要特别说明：

`route_table.go` 已从主链路中移除。

当前 Bot 路径定义应只从 handler registry 推导，而不是再维护第二份静态路径表。

## 9. 当前应降级为非主链路的内容

下面这些内容不应再被文档写成客户端主链路：

1. `GlobalCommandResolver` 先决定 module + mode，再进入 Bot 端点
2. 云端根据原文重新全局决定目标端点
3. 客户端直接调用 `/internal/pjsk/render`
4. 客户端直接调用 `/internal/pjsk/command`

## 10. `path` 定义合并状态

这项合并已经完成，当前代码状态如下。

### 10.1 当前唯一事实来源

当前 Bot 路由元数据的唯一事实来源是：

`command -> handler -> path`

也就是说：

1. 每个对外暴露的 Bot handler 直接在自身定义处声明 `Path`
2. `internal/pjsk/handler` registry 聚合同一路径下的命令集合
3. Bot HTTP 路由注册使用 `ListBotRoutes()`
4. manifest seed 也使用同一份 route 数据

`api/bot/pjsk/route_table.go` 已不存在。

### 10.2 当前分层

当前职责划分已经落地为：

1. `internal/pjsk/handler/*`
   - 定义命令
   - 定义 `Path`
   - 负责详细解析
2. `internal/pjsk/handler` registry
   - 聚合 `path -> commands -> module -> method`
   - 提供给 Bot API 与 manifest seed 使用
3. `api/bot/pjsk/*`
   - 只负责 HTTP 暴露
   - 不再维护第二份静态路径表
4. `command_manifests`
   - 从 handler registry 同步生成

### 10.3 当前效果

当前实现已经满足：

1. `path` 只在 handler 定义侧声明一次
2. handler registry 能完整导出 Bot route 列表
3. Bot HTTP 路由注册使用 handler registry
4. manifest seed 使用 handler registry
5. `matched_command -> handler.path` 校验仍然成立

### 10.4 需要继续注意的点

虽然 `route_table.go` 已删除，但仍需继续注意：

1. 新增命令时必须显式写好 `Path`
2. 多义 handler 的 path 归属要继续保持稳定
3. 文档不能再用“静态 route_table”描述当前 Bot 路由来源

## 11. 账号绑定命令修正规则

本节用于修正 `profile` 中账号绑定相关命令的实现边界。

### 11.1 基本原则

账号绑定相关命令虽然最终只返回文本，但它们仍然属于标准 PJSK 命令执行链路的一部分。

因此必须满足：

1. `ProfileBindHandle`
2. `ProfileUnbindHandle`
3. `ProfileSetMainHandle`
4. `ProfileClearDefaultBindingHandle`

这几类 handler 的返回值都应与其他 handler 保持一致，统一返回 `*parser.ResolvedCommand`。

禁止再采用下面这种做法：

1. handler 内部直接调用绑定 service
2. handler 直接拼接文本结果
3. Bot API 根据 handler 返回 `string` 特判出站

### 11.2 正确链路

账号绑定命令的目标链路应为：

```text
Bot 端点
  -> handler 校验 matched_command 是否属于当前 path
  -> handler 解析原文
  -> handler 返回 ResolvedCommand
  -> commandhandler.Execute(...)
  -> profile 执行分发
  -> 返回 onebot11.Message
  -> Bot API 直接输出 OneBot11 JSON 响应
```

也就是说：

1. handler 负责“命令解释”
2. `Execute` 负责“命令执行”
3. Bot API 负责“HTTP 出站”

三者不能混在一起。

### 11.3 `ResolvedCommand` 侧的要求

绑定相关 handler 应新增并使用明确的 `Mode`，例如：

1. `profile-bind`
2. `profile-bind-list`
3. `profile-unbind`
4. `profile-default-set`
5. `profile-default-clear`

其中：

1. 原始输入中的 UID、`uN`、是否显式指定区服等信息，应由 handler 解析后写入 `Params`
2. `Query` 仍可保留原始剩余参数，但不能把真正的业务执行放在 handler 内完成

### 11.4 `Execute` 返回值收口

这条链路已经走过两步收口：

1. 第一阶段先把图片/文本统一到 `Execute(...)`
2. 第二阶段再把 `Execute(...)` 的外部契约进一步收口为 `onebot11.Message`

当前代码状态是：

```go
func Execute(ctx context.Context, resolved *parser.ResolvedCommand, app *renderapp.App) (onebot11.Message, error)
```

当前约定如下：

1. 图片类 `execute*` 在 bridge 内部完成 `Render... -> ImageCache.StoreAndGetURL(...) -> onebot11.Image(url)`
2. 文本类 `execute*` 直接在 bridge 内部返回 `onebot11.Text(text)`
3. `CommandResultDataType` 仍保留在 bridge 内部，主要给 `executeProfile(...)` 这类辅助路径区分图片/文本，不再作为 API 对外契约
4. Bot API 与 legacy API 不再根据 `data_type` 分支出站

### 11.5 Bot API 的正确职责

`api/bot/pjsk/handler.go` 应当调整为：

1. 调用 handler，拿到 `*parser.ResolvedCommand`
2. 调用 `commandhandler.Execute(...)`
3. 直接输出 `Execute(...)` 返回的 `onebot11.Message`

当前实现还补充了两个关键约束：

1. `BuildContext(ctx, event)` 只负责从消息段提取 `ArgText` 与 `at` 列表
2. 真正的命令匹配通过 `MatchCommandHandler(ctx.GetArgs())` 完成，并校验结果与请求头 `matched_command` 及当前 endpoint path 一致
3. API 层不再保留 `case *parser.ResolvedCommand`、`case string` 或 `case data_type` 这类业务分流逻辑

### 11.6 `profile` 执行分发要求

`internal/pjsk/handler/bridge.go` 中的 `executeProfile(...)` 应扩展为同时支持：

1. 传统资料卡类图片模式
2. 账号绑定相关文本模式

也就是说，绑定 service 的实际调用位置应下沉到 `executeProfile(...)` 对应分发中，或其继续调用的 profile 执行层中，而不是停留在 `sekai/profile.go` 的 handler 内。

### 11.7 本轮修正的实施顺序

当前这轮收口实际按下面顺序落地：

1. 先回退 handler 内部直接执行业务、直接返回字符串的实现
2. 为 profile 绑定命令定义稳定的 `ResolvedCommand.Mode`
3. 先用 `payload + data_type` 打通图片/文本统一执行
4. 再把 `Execute(...)`、bot API、legacy API 的外部契约统一收口到 `onebot11.Message`
5. 最后再把 `BuildContext` 简化为只负责消息提取，把命令匹配收回 registry

### 11.8 验收标准

账号绑定链路的最终正确状态应为：

1. binding 相关 handler 与其他 handler 一样，只返回 `ResolvedCommand`
2. Bot API 不再根据 handler 返回 `string` 做特殊分支
3. `commandhandler.Execute(...)` 成为图片命令和文本命令的统一执行入口，并直接返回 `onebot11.Message`
4. bot API 与 legacy API 直接输出 `Execute(...)` 结果，不再以 `data_type` 为外部协议
5. `refer/profile.py` 只保留语义参考价值，不污染 Cloud 当前的分层结构

## 12. 歌曲别名命令补充（2026-03-26）

当前歌曲别名已经不再是 `music` 模块内的占位逻辑，而是作为独立的 `ModuleAlias` 文本执行链路落地。

### 12.1 当前命令路径

| 功能 | Path | 说明 |
|------|------|------|
| 歌曲别名查询 | `music/alias` | 仅查询已审核通过的别名 |
| 添加歌曲别名 | `music/alias/add` | 提交审核申请，不直接写入正式别名表 |
| 删除歌曲别名 | `music/alias/del` | 仅管理员可用，只删除已审核别名 |
| 待审核列表 | `music/alias/pending` | 仅管理员可用 |
| 通过别名审核 | `music/alias/approve` | 仅管理员可用，支持批量 |
| 拒绝别名审核 | `music/alias/reject` | 仅管理员可用，单条 + 原因 |

### 12.2 解析与执行边界

当前别名链路按下面顺序工作：

```text
sekai/music.go
  -> makeResolvedCmdWithParams(..., ModuleAlias, mode, params)
  -> bridge.executeAlias(...)
  -> musicalias.ExecuteCommand(...)
  -> musicalias.Service
```

也就是说：

1. `sekai/music.go` 只负责命令格式检查和参数提取
2. `bridge.go` 只负责把 `ModuleAlias` 路由到文本执行器
3. `musicalias.Service` 负责歌曲解析、冲突检查、审核权限和数据库写入

### 12.3 歌曲定位规则

无论是查询还是提交别名，目标歌曲都按以下顺序解析：

1. 歌曲 ID
2. 曲名（精确匹配）
3. 已审核别名（精确匹配）

如果命中多首歌曲，则要求调用方改用歌曲 ID。

### 12.4 审核规则

当前歌曲别名审核采用“两阶段”模型：

1. 用户提交 `/music alias add`
2. 每个别名单独写入 `pending_alias`
3. 管理员使用 `/待审核别名` 查看列表
4. 管理员使用 `/同意别名 ...` 或 `/拒绝别名 ...` 完成审核

当前规则还包括：

1. 查询只能读取正式 `alias` 表中的已审核别名
2. 提交时会拒绝与已有曲名重复的别名
3. 提交时会拒绝与已审核别名重复的别名
4. 提交时会拒绝与待审核别名重复的别名
5. 删除命令只允许删除正式 `alias` 表中的已审核别名
6. 审核管理员身份通过 `(platform, user_id) -> haruki_user_id -> alias_admins` 校验

## 13. 相关文档

- [ZeroBot 与 Cloud 联调方案](zerobot-cloud-integration-plan.cn.md)
- [项目进展总结](project-status-summary.cn.md)
- [ZeroBot 渲染接入后续事项](zerobot-render-followup.cn.md)
