# Haruki-Cloud PJSK 账号绑定实现说明

> 最后更新：2026-03-25
>
> 本文档记录 2026-03-24 这轮账号绑定相关改造的最终收口结果，重点说明命令链路、分层边界、代码落点、测试覆盖和后续注意事项。

## 1. 文档目的

这轮改动同时涉及了三个层面：

1. PJSK 账号绑定能力本身
2. Bot 指令执行链路的统一化
3. `handler` / `userdata` / API 三层之间的职责重划

如果只看代码，很容易只看到“加了绑定功能”，却看不出这次真正修正的是：

1. 文本型命令不能绕过 `ResolvedCommand -> Execute` 链路
2. 账号绑定业务不应留在 `handler` 层
3. Bot API 不应直接根据 handler 的 Go 返回类型做业务分流

因此需要单独保留一份实现说明，供后续维护和联调参考。

## 2. 这轮改造后的核心结论

### 2.1 命令链路结论

账号绑定相关命令现在已经与其他 PJSK 指令统一到同一条链路：

```text
客户端命中前缀树
  -> 请求 /api/v2/bot/:botId/pjsk/profile/*
  -> Bot 端点校验 matched_command 与 path
  -> sekai/profile handler 解析原文
  -> handler 返回 ResolvedCommand
  -> commandhandler.Execute(...)
  -> profile 分发到 userdata 绑定执行
  -> Execute 返回 payload + data_type
  -> API 层按 data_type 输出最终响应
```

这意味着：

1. handler 只负责“解释命令”
2. Execute 只负责“执行命令”
3. API 只负责“出站响应”

### 2.2 分层结论

这轮改造后，账号绑定业务已经从 `handler` 层下沉到 `userdata` 层。

正确边界如下：

1. `internal/pjsk/handler/`
   - 命令注册
   - `ResolvedCommand` 桥接
   - `Execute` 薄分发
2. `internal/pjsk/userdata/`
   - 账号绑定领域逻辑
   - 文本结果格式化
   - profile binding 专用执行入口
3. `api/bot/pjsk/`
   - 协议校验
   - HTTP 出站响应

### 2.3 结果类型结论

`commandhandler.Execute(...)` 不再只返回 `[]byte`，而是统一返回：

```go
([]byte, CommandResultDataType, error)
```

当前定义的结果类型常量为：

1. `CommandResultDataTypeImagePNG = "image/png"`
2. `CommandResultDataTypeText = "text/plain"`

因此：

1. 图片命令和文本命令都走同一个执行入口
2. API 层不再依赖 handler 的返回值 Go 类型做业务判断

## 3. 当前已实现的账号绑定能力

本轮已经完成的用户侧能力如下。

### 3.1 绑定

命令语义：

- `绑定 xxxxxx`

行为：

1. 使用 Sekai API 对 5 个区服依次探测用户是否存在
2. 若存在，则在当前用户身份下写入绑定
3. 如果这是该用户的第一个绑定：
   - 自动设为全局默认绑定
   - 自动设为该区服默认绑定
4. 若同一个 UID 在多个区服都存在：
   - 当前实现会选择第一个命中的区服
   - 返回文本提示“该 ID 在多个服务器都存在，当前默认绑定第一个命中的服务器”

### 3.2 解绑

命令语义：

- `取消绑定 xxxxxx`
- `取消绑定 u1`

行为：

1. 支持按 UID 精确解绑
2. 支持按排序后的 `uN` 序号解绑
3. 若被解绑账号恰好是全局默认绑定或区服默认绑定：
   - 自动在剩余绑定中重建对应默认绑定

### 3.3 绑定列表

命令语义：

- `绑定列表`

行为：

1. 列出当前用户所有已绑定账号
2. 按 UID 数值升序排序
3. 列表中显示：
   - `uN` 序号
   - 区服
   - UID
   - 是否是全局默认
   - 是否是区服默认

### 3.4 设置默认绑定

命令语义：

- `设置默认绑定 123456789`
- `设置默认绑定 u1`
- `[region]设置默认绑定 123456789`
- `[region]设置默认绑定 u1`

行为：

1. 未显式带区服时，修改全局默认绑定
2. 显式带区服时，修改该区服默认绑定
3. 支持按 UID 或 `uN` 选择目标账号

### 3.5 取消默认绑定

命令语义：

- `取消默认绑定 123456789`
- `取消默认绑定 u1`
- `[region]取消默认绑定 123456789`
- `[region]取消默认绑定 u1`

行为：

1. 未显式带区服时，清除全局默认绑定
2. 显式带区服时，清除对应区服默认绑定
3. 只能取消当前该 scope 下已经生效的默认绑定

## 4. 当前命令与路径映射

### 4.1 `profile/bind`

路径：

- `/api/v2/bot/:botId/pjsk/profile/bind`

当前承载命令：

1. `/pjsk bind`
2. `/pjsk id`
3. `/绑定`
4. `/pjsk 绑定`
5. `/绑定列表`

其中：

1. `绑定列表` 仍走同一路径
2. handler 根据命中的原始命令，决定最终产出：
   - `profile-bind`
   - `profile-bind-list`

### 4.2 `profile/unbind`

路径：

- `/api/v2/bot/:botId/pjsk/profile/unbind`

当前承载命令：

1. `/pjsk unbind`
2. `/pjsk解绑`
3. `/解绑`
4. `/取消绑定`

对应 mode：

- `profile-unbind`

### 4.3 `profile/default`

路径：

- `/api/v2/bot/:botId/pjsk/profile/default`

当前承载命令：

1. `/pjsk set main`
2. `/pjsk主账号`
3. `/设置主账号`
4. `/主账号`
5. `/设置默认绑定`
6. `/默认绑定`

对应 mode：

- `profile-default-set`

### 4.4 `profile/default/clear`

路径：

- `/api/v2/bot/:botId/pjsk/profile/default/clear`

当前承载命令：

1. `/取消默认绑定`
2. `/清除默认绑定`
3. `/取消主账号`
4. `/清除主账号`

对应 mode：

- `profile-default-clear`

## 5. 代码落点

### 5.1 handler 层

#### `internal/pjsk/handler/result.go`

职责：

1. 定义统一执行结果类型常量

当前常量：

1. `CommandResultDataTypeImagePNG`
2. `CommandResultDataTypeText`

#### `internal/pjsk/handler/profile_mode.go`

职责：

1. 保留 profile 渲染模式常量

当前仅保留：

1. `ProfileModeRender = "profile"`

#### `internal/pjsk/handler/bridge.go`

职责：

1. 统一承接 `ResolvedCommand`
2. 对各 module 做薄分发
3. 对 profile 模块继续转发到：
   - 传统 profile 图片渲染
   - userdata 绑定执行

注意：

这里现在只负责“转发”，不再保存绑定领域逻辑。

### 5.2 sekai handler 层

#### `internal/pjsk/handler/sekai/profile.go`

职责：

1. 定义 profile 系列命令
2. 解析命令参数
3. 产出 `ResolvedCommand`

注意：

1. 这里不再直接调用 `BindingService`
2. 这里不再直接拼接绑定结果文本
3. 绑定、解绑、默认绑定、清除默认绑定、交换绑定等命令显式关闭通用 `uidArg` 解析
4. 也就是说，这些命令中的 `u1` / UID 仍由各自命令本身解释，而不会先被 `sekai` 公共层吃掉

#### `internal/pjsk/handler/sekai/handler.go`

职责：

1. 统一处理区服前缀、前缀参数、帮助/预览/详细模式等通用参数
2. 统一处理账号指定参数：
   - `u[i]`
   - 游戏 UID
   - `@qq`
3. 将通用账号选择器写入 `SekaiHandlerContext.uidArg`

当前实现约束：

1. `SekaiCommandHandler.ParseUIDArg` 默认开启
2. profile 绑定相关命令显式关闭该能力
3. 消息段中的 `at` 只识别 OneBot 标准 `qq` 字段

### 5.3 userdata 层

#### `internal/pjsk/userdata/binding_service.go`

职责：

1. 用户身份解析后的绑定领域操作
2. 与 DB 表交互
3. 与 Sekai API 校验交互

核心能力：

1. `Bind`
2. `List`
3. `Unbind`
4. `SetDefault`
5. `ClearDefault`

#### `internal/pjsk/userdata/profile_binding.go`

职责：

1. 定义 profile binding 相关 mode 常量
2. 定义 profile binding 的参数结构
3. 提供统一执行入口：
   - `ExecuteProfileBindingCommand`
4. 生成绑定命令的文本结果

也就是说，账号绑定作为一个 profile 领域子能力，其“执行”已经收口在 `userdata` 下，而不是 `handler` 下。

### 5.4 render runtime

#### `internal/pjsk/render/app/app.go`

职责：

1. `renderapp.App` 新增 `Bindings *userdata.BindingService`
2. 作为统一 runtime 容器，被 `Execute` 侧消费

### 5.5 服务启动注入

#### `cmd/server/main.go`

职责：

1. 初始化 `users` DB
2. 初始化 `pjsk` DB
3. 初始化 `BindingService`
4. 挂载到 `renderRuntime.Bindings`

这意味着：

账号绑定依赖已经从“sekai handler 私有运行时”转为“render runtime 公共依赖”。

## 6. 数据来源与依赖

### 6.1 用户身份来源

账号绑定不直接使用平台 ID 作为 PJSK 绑定主键，而是先通过：

- `internal/identity.Resolver`

将：

1. `platform`
2. `platform_user_id`

解析为：

- `haruki_user_id`

### 6.2 绑定数据表

当前使用：

1. `pjsk.user_bindings`
2. `pjsk.user_default_bindings`

分别保存：

1. 用户绑定的游戏账号
2. 用户的全局默认绑定和区服默认绑定

### 6.3 UID 存在性校验

绑定时会调用 Sekai API：

- `GetUserProfile(server, uid)`

按以下区服顺序进行探测：

1. JP
2. CN
3. TW
4. KR
5. EN

### 6.4 当前未使用本地 JSON

需要明确：

本轮账号绑定功能不依赖本地 `.json` 用户快照。

本地 JSON 目前仍只用于其它与用户快照相关的渲染测试或临时数据场景，不参与绑定校验和绑定持久化。

## 7. API 层的变化

### 7.1 Bot API

涉及：

- `api/bot/pjsk/handler.go`

当前行为：

1. 读取 `command_payload`
2. 读取 `X-Haruki-Bot-Matched-Command`
3. 将 `command_payload` 恢复为 OneBot 消息段
4. 通过 `BuildContext` 提取纯文本参数和 `at` 列表
5. 校验当前 `matched_command` 是否属于该 path
6. 调用 handler 获取 `ResolvedCommand`
7. 调用 `commandhandler.Execute(...)`
8. 按 `CommandResultDataType` 输出响应：
   - `image/png`
   - JSON 包装的文本消息

### 7.2 legacy API

涉及：

- `api/legacy/pjsk/command.go`

当前行为也已统一改为：

1. `resolver.Resolve(...)`
2. `commandhandler.Execute(...)`
3. 按 `CommandResultDataType` 出站响应

不过需要再次明确：

`/internal/pjsk/command` 不是客户端主协议，而且账号绑定命令当前并不通过 `GlobalCommandResolver` 暴露给它。

## 8. 与之前错误实现相比，已经修正的点

这轮改造特别修正了以下问题。

### 8.1 修正点一：handler 不再直接执行业务

之前错误做法：

1. `sekai/profile.go` 中直接调用 `BindingService`
2. handler 内直接拼接文本并返回

当前修正后：

1. handler 只构造 `ResolvedCommand`
2. 真正执行放到 `Execute -> userdata`

### 8.2 修正点二：文本命令不再绕过 Execute

之前错误做法：

1. 账号绑定类命令直接返回字符串
2. Bot API 根据 `string` 分支直接响应

当前修正后：

1. 文本命令也必须走 `Execute`
2. `Execute` 返回显式结果类型

### 8.3 修正点三：绑定业务不再放在 handler 目录

之前错误做法：

1. 在 `internal/pjsk/handler/` 下保存 `profile_binding` 业务执行代码

当前修正后：

1. 绑定业务执行和文本格式化已移动到 `internal/pjsk/userdata/`
2. `handler` 只保留 mode 分发和统一桥接逻辑

### 8.4 修正点四：依赖注入不再走 handler 私有运行时

之前错误做法：

1. 在 `sekai` 侧维护 `RuntimeServices`
2. 由 handler 私有读取 `bindingService()`

当前修正后：

1. 绑定服务直接注入 `renderapp.App`
2. `Execute` 统一通过 runtime 读取依赖

## 9. 测试覆盖

### 9.1 `internal/pjsk/userdata/binding_service_test.go`

覆盖：

1. 首次绑定自动设置默认绑定
2. 绑定列表按 UID 升序
3. 设置默认绑定
4. 解绑后自动重建默认绑定

### 9.2 `internal/pjsk/userdata/profile_binding_test.go`

覆盖：

1. `DecodeProfileBindingParams` 的字段解码和去空格
2. `ExecuteProfileBindingCommand` 的绑定结果文本
3. `ExecuteProfileBindingCommand` 的绑定列表文本

### 9.3 `api/bot/pjsk/handler_test.go`

覆盖：

1. 图片型命令仍能正常返回 `image/png`
2. 文本型命令现在通过真实的 `profile/bind + /绑定列表` 链路返回 JSON 文本
3. `decodeCommand` 会保留 OneBot 消息段，并覆盖 `@qq` 场景

### 9.4 `api/legacy/pjsk/command_test.go`

覆盖：

1. legacy 指令入口在 `Execute` 改签名后仍能正常返回图片

### 9.5 当前验证命令

本轮实际执行通过：

```bash
go test ./internal/pjsk/userdata ./internal/pjsk/handler/... ./api/bot/pjsk ./api/legacy/pjsk ./cmd/server
```

补充说明：

当前 `uidArg` 通用解析相关测试还覆盖了：

1. `internal/pjsk/parser/parser_test.go`
2. `internal/pjsk/handler/context_test.go`
3. `internal/pjsk/handler/sekai/handler_test.go`

## 10. 当前限制与未完成项

### 10.1 仍未迁移的 profile 子功能

以下 profile 相关能力仍是 `TODO` 或禁用状态：

1. 交换绑定
2. 隐藏/显示抓包
3. 隐藏/显示 ID
4. 注册时间查询
5. profile 服务状态检查
6. 抓包模式
7. 抓包状态
8. 黑名单增删
9. verify / verify list

### 10.2 多服同 UID 的选择规则仍较保守

当前规则是：

1. 若一个 UID 在多个服都存在
2. 选择探测顺序中的第一个命中服
3. 返回文本提醒

这能工作，但不是最终最强语义。

### 10.3 隐藏显示规则仍未完整接上

当前新建绑定默认：

- `visible = true`

因此：

1. 列表展示逻辑已经支持隐藏显示
2. 但隐藏/显示 ID 的命令本身还未迁移完成

## 11. 后续建议

如果继续沿这条线推进，建议优先顺序如下：

1. 补 `userdata/profile_binding.go` 更细的文本断言测试
2. 继续迁移 profile 中剩余仍为 `TODO` 的子能力
3. 若要进一步收口，可考虑为文本结果补统一的结构化载荷定义，而不仅仅是文本常量
4. 等客户端联调时，再根据实际需求决定是否需要新增更明确的错误码或文本模板

## 12. 相关文档

- [PJSK 指令系统设计](pjsk-command-system.cn.md)
- [项目进展总结](project-status-summary.cn.md)
- [ZeroBot 与 Cloud 联调方案](zerobot-cloud-integration-plan.cn.md)
