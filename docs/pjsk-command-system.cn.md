# Haruki-Cloud PJSK 指令系统设计

> 最后更新：2026-03-24
>
> 本文档描述的是当前目标模型。若现有实现仍保留部分旧链路，以本文档定义的边界为后续收口目标。

## 1. 核心结论

PJSK 指令系统的主链路应当是“端点中心”模型，而不是“云端全局重新选路”模型。

也就是说：

1. 云端负责下发命令规则。
2. 客户端负责基于规则做前缀树匹配。
3. 客户端命中后，按 `path` 调用对应的 Bot 端点，并上传 `matched_command`。
4. 云端只校验 `matched_command` 是否属于当前端点。
5. 当前端点对应的 handler 再根据原始文本做详细解析。
6. 解析成功后直接进入处理和画图。
7. 如果 `matched_command` 与端点不一致，或原文无法被该 handler 继续处理，则返回错误。

## 2. 主链路

当前目标主链路如下：

```text
Haruki-Cloud 下发 command_manifests
  -> Haruki-ZeroBot 构建本地前缀树
  -> 本地匹配到 command_module + command_path + matched_command
  -> 请求 /api/v2/bot/:botId/<module>/<path>
  -> 上传原始 command + matched_command
  -> 端点校验 matched_command -> handler.path
  -> 命中 handler 后在该 handler 内解析原文参数
  -> 调用 render 处理链路
  -> 返回 PNG
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
2. 恢复原始文本命令
3. 校验 `matched_command` 是否属于当前端点
4. 使用对应 handler 在当前端点语义范围内解析原文
5. 提取当前端点所需参数
6. 若 `matched_command` 或原文不成立，返回 `400`
7. 若解析成立，调用后续处理链路

关键点是：

Bot 端点不应该再把“请求发到哪个端点”这个问题重新交给一个全局路由器决定。

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
3. 返回 PNG 或中间构建结果

这一层不负责前缀树命中，也不负责 Bot 业务端点选路。

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
3. 客户端同时上传 `matched_command`
4. 云端先检查这个 `matched_command` 是否属于 `card/detail`
5. 命中该 path 的 handler 后，再按当前 handler 规则解析原始 `command`
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

## 10. `path` 定义合并方案

### 10.1 合并目标

`path` 的定义应当从 `api/bot/pjsk/route_table.go` 收回到 `internal/pjsk/handler`。

目标结构如下：

1. 每个对外暴露的 Bot handler 自己声明 `Path`
2. handler registry 聚合出完整的 Bot route 列表
3. Bot 路由注册、manifest 同步、端点归属校验都使用同一份 handler-derived 数据
4. `api/bot/pjsk` 不再维护第二份静态 `path` 表

换句话说，未来的唯一事实来源应当是：

`command -> handler -> path`

而不是：

`command -> handler`

再加上另一份独立的：

`path -> route_table`

### 10.2 为什么必须合并

当前双份定义至少有以下风险：

1. `route_table.go` 和 handler 中的 `path` 容易漂移
2. manifest、HTTP 路由注册、端点归属校验可能各自使用不同来源
3. 新增或调整命令时，开发者必须同时修改 API 层和 handler 层，容易漏改
4. 部分 handler 是多义解析器，静态 `mode` 表和真实 handler 行为本身就不完全同构

因此，继续保留两份路径定义只会增加维护成本。

### 10.3 目标分层

合并后的职责划分应当是：

1. `internal/pjsk/handler/*`
   - 定义命令
   - 定义 `Path`
   - 负责详细解析
2. `internal/pjsk/handler` registry
   - 聚合 `path -> commands -> handler metadata`
   - 提供给 Bot API 使用
3. `api/bot/pjsk/*`
   - 只负责 HTTP 暴露
   - 不再定义第二份业务路径表
4. `command_manifests`
   - 从 handler registry 同步生成

### 10.4 详细迁移清单

#### 阶段 1：把 `Path` 写回具体 handler

需要完成：

1. 对所有对外暴露的 Bot handler，直接在 `CommandHandlerBase` 上显式填写 `Path`
2. 不再依赖方法名到 `path` 的映射表
3. 对不对外暴露的内部/兼容 handler，保持 `Path == ""`

例如：

1. `CardDetailHandle` 写 `Path: "card/detail"`
2. `MusicListHandle` 写 `Path: "music/list"`
3. `EventHandle` 写 `Path: "event/list"`

验收标准：

1. 所有 Bot 暴露 handler 都能在自身定义处直接看到 `Path`
2. 不再需要在别处反推它属于哪个路径

#### 阶段 2：在 handler registry 中建立 Bot route 聚合结果

建议新增统一结构，例如：

```go
type BotRoute struct {
    Path string
    Module string
    Commands []string
    Methods []string
    AdditionalParams []string
}
```

需要完成：

1. 在 `RegisterCommandHandler()` 或其上层注册流程中聚合所有带 `Path` 的 handler
2. 将同一路径下的多条命令合并到同一个 route 记录
3. 给 Bot API 暴露只读查询接口，例如：
   - `ListBotRoutes()`
   - `GetBotRoute(path string)`

验收标准：

1. handler registry 可以独立返回完整 Bot route 列表
2. 不依赖 `route_table.go` 也能拿到 `path + commands`

#### 阶段 3：让 Bot HTTP 路由注册改为消费 handler registry

需要完成：

1. `api/bot/pjsk/handler.go` 不再遍历 `botModeTable`
2. 改为遍历 `handler.ListBotRoutes()`
3. 每条 route 注册 `GET|POST /api/v2/bot/:botId/pjsk/<path>`

验收标准：

1. Bot 路由注册来源与 handler registry 完全一致
2. 新增一个 Bot handler 时，不需要再去 API 层补第二份路径定义

#### 阶段 4：让 manifest seed 改为消费同一份 route 数据

需要完成：

1. `api/bot/pjsk/seed.go` 改为直接读取 handler registry 中的 route 列表
2. `command_prefixes` 来自该路径下聚合出的命令集合
3. `command_path` 来自 handler 的 `Path`
4. `command_additional_params` 继续由协议要求统一维护

验收标准：

1. manifest 与 HTTP 路由注册使用同一份 route 元数据
2. manifest 不再依赖 `route_table.go`

#### 阶段 5：移除迁移期映射与静态表

需要完成：

1. 删除 `botPathByMethodName`
2. 删除 `botModeTable`
3. 删除 `route_table.go`
4. 清理所有只为兼容静态路径表而存在的辅助逻辑

验收标准：

1. 代码库中不存在第二份 Bot 路径定义
2. `path` 只能从 handler registry 推导出来

#### 阶段 6：补测试

至少需要补下面几类测试：

1. 注册测试
   - 所有 Bot 暴露 handler 都有非空 `Path`
2. 聚合测试
   - 同一路径下的命令被正确合并
3. 路由注册测试
   - `ListBotRoutes()` 产出的每个路径都被实际注册
4. manifest 测试
   - 返回的 `command_path` 和命令集合与 handler registry 一致
5. 归属校验测试
   - `matched_command` 只能命中其所属 `path`

验收标准：

1. 路由注册、manifest、端点校验三者共享同一份测试事实来源

### 10.5 需要明确的取舍

#### `module` 的处理

`module` 应作为显式注册信息保留，不能再从 `path` 首段推导。

当前约束如下：

1. Bot API 的总路径形状是 `/api/v2/bot/:botId/<module>/<path>`
2. `command_module` 表示顶层业务模块，例如 `pjsk`
3. `command_path` 表示模块内相对路径，例如 `card/detail`
4. 因此 PJSK manifest 应写成 `command_module = "pjsk"`、`command_path = "card/detail"`

这样设计后：

1. `card`、`music`、`event` 这类值只属于 `path` 首段，不再代表 manifest 的 `module`
2. 未来若接入其他模块，也只需要在注册 handler 时显式声明自己的 `module`

#### `mode` 的处理

建议不要把当前 `route_table.go` 中的静态 `mode` 继续当作 Bot 路由元数据主来源。

原因是：

1. Bot 端点命中的是 `path`
2. 最终 render 的 `ResolvedCommand.Mode` 由 handler 详细解析后决定
3. 某些 handler 本身可能根据原文分流到不同 render mode

因此：

静态 `mode` 更适合作为渲染桥接内部概念，而不是 Bot path 注册表的核心字段。

### 10.6 推荐实施顺序

推荐按下面顺序实施：

1. 先把所有 Bot 暴露 handler 的 `Path` 显式写回各自文件
2. 建立 handler registry 的 route 聚合能力
3. 改 Bot 路由注册
4. 改 manifest seed
5. 补测试
6. 最后删除 `route_table.go` 和方法名映射表

这样做的好处是：

1. 每一步都可验证
2. 可以先完成“唯一事实来源”迁移，再做删除
3. 避免一口气替换过多位置导致排查困难

### 10.7 最终验收标准

最终应达到以下状态：

1. `path` 只在 handler 定义侧声明一次
2. handler registry 能完整导出 Bot route 列表
3. Bot HTTP 路由注册使用 handler registry
4. manifest seed 使用 handler registry
5. `matched_command -> handler.path` 校验仍然成立
6. `route_table.go` 不再存在
7. 新增一个 Bot handler 时，只需要改 handler 定义，不需要再改第二份静态路径表

## 11. 相关文档

- [ZeroBot 与 Cloud 联调方案](zerobot-cloud-integration-plan.cn.md)
- [项目进展总结](project-status-summary.cn.md)
- [ZeroBot 渲染接入后续事项](zerobot-render-followup.cn.md)
