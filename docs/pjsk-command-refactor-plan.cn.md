# Haruki-Cloud PJSK 命令系统重构方案

> 最后更新：2026-04-19
>
> 本文档描述 PJSK 命令系统的结构重构目标、约束、执行步骤与测试计划。
> 本次重构明确以“去除现有奇怪链路、建立单一清晰命令模型”为目标，不保留旧结构兼容层。

## 0. 当前进度

截至 2026-04-19，以下步骤已经完成：

1. `internal/pjsk/onebot11` 已迁移到 `internal/onebot11`。
2. `internal/handler` 已收敛为唯一通用 command registry / bot route 元信息来源。
3. Bot API 入口已改为优先使用 `matched_command` 精确定位命令，并在必要时回退到同 path 的文本匹配结果。
4. `internal/pjsk/handler/sekai` 已整体提平到 `internal/pjsk/handler`，命令仍保留反射注册方式，但不再跨包分裂。
5. 执行前置已从 `bridge.Execute` 中抽离为独立的 `PrepareExecutionRuntime(...)`，开始为后续去除旧 bridge 主链做准备。
6. Handler 主链、Bot API 入口、`RequestContext` 与 bridge 执行层之间已经改用 `handler.CommandRequest`，`parser.ResolvedCommand` 不再是它们之间的核心运行时对象。
7. 命令 handler 在注册阶段已绑定 executor，`Handle(...)` 产出 `CommandRequest` 时即带执行能力；Bot API 已直接走 `ExecuteCommandRequest(...)`，不再依赖运行时的 `module -> executeX(...)` 默认分发。
8. `misc` / `vlive` 两个单命令模块的执行器已经回收到各自命令文件，`bridge_misc.go` 与 `bridge_vlive.go` 已删除。
9. `alias` / `gacha` / `arrest` / `regtime` / `stamp` / `checkdata` 的执行器也已回收到命令文件，相关 `bridge_*.go` 已删除。
10. `card` 域的执行逻辑已回收到 [`internal/pjsk/handler/card.go`](/home/xmlq/codes/haruki/Haruki-Cloud/internal/pjsk/handler/card.go)，`bridge_card.go` 已删除。
11. `event` 域的执行逻辑已回收到 [`internal/pjsk/handler/event.go`](/home/xmlq/codes/haruki/Haruki-Cloud/internal/pjsk/handler/event.go)，`bridge_event.go` 已删除。
12. `profile` 域的执行逻辑已回收到 [`internal/pjsk/handler/profile.go`](/home/xmlq/codes/haruki/Haruki-Cloud/internal/pjsk/handler/profile.go)，`bridge_profile.go` 已删除。
13. `education` 域的执行逻辑已回收到 [`internal/pjsk/handler/education.go`](/home/xmlq/codes/haruki/Haruki-Cloud/internal/pjsk/handler/education.go)，`bridge_education.go` 已删除。
14. `score` 域的执行逻辑已回收到 [`internal/pjsk/handler/score.go`](/home/xmlq/codes/haruki/Haruki-Cloud/internal/pjsk/handler/score.go)，`bridge_score.go` 已删除。
15. `sk` 域的执行逻辑已回收到 [`internal/pjsk/handler/sk.go`](/home/xmlq/codes/haruki/Haruki-Cloud/internal/pjsk/handler/sk.go)，`bridge_sk.go` 与 `bridge_sk_modes.go` 已删除。
16. `mysekai` 域的执行逻辑已回收到 [`internal/pjsk/handler/mysekai.go`](/home/xmlq/codes/haruki/Haruki-Cloud/internal/pjsk/handler/mysekai.go)，`bridge_mysekai.go` 已删除。
17. `deck` 域的执行逻辑已回收到 [`internal/pjsk/handler/deck.go`](/home/xmlq/codes/haruki/Haruki-Cloud/internal/pjsk/handler/deck.go)，`bridge_deck.go` 已删除。
18. `music` 域的执行逻辑已回收到 [`internal/pjsk/handler/music.go`](/home/xmlq/codes/haruki/Haruki-Cloud/internal/pjsk/handler/music.go)，`bridge_music.go` 已删除。
19. 旧 bridge 辅助文件已按职责改名：`bridge.go` -> `execution_helpers.go`，`bridge_event_record.go` -> `event_record_builder.go`，`bridge_deck_resolve*.go` -> `deck_*` 辅助文件。
20. `requestbuilder` 中仍依赖旧命令壳的入口已改为 typed `CommandInput`，旧适配层 `command_request_legacy.go` 已删除。
21. `parser` 包中仅为旧统一 resolver 服务的 `ResolvedCommand`、`GlobalCommandResolver` 与 `global_resolver.go` 已删除，对应 resolver 测试已清理。
22. 运行时不再暴露 `Dispatch()` 这类“重新全局匹配再 Handle”的旧入口；相同能力仅保留为测试内局部 helper。
23. 注册阶段不再通过 `pathExecutor(path)` 进行按路径字符串的执行器二次分发；各命令已在定义处显式绑定本域 executor。
24. 测试层已完成一轮旧命名清理：`BuildsResolvedCommand` 类函数名已收口为 `BuildsCommandRequest`，`bridge_*_test.go` 文件也已按当前职责改名。

当前尚未完成的核心工作：

1. 将统一执行前置从当前的公共 executor 入口进一步下沉为“命令直接执行”的运行时模型。
2. 清理文档与少量辅助层中的历史性旧结构描述与命名残留。

## 1. 背景

当前 PJSK 命令系统的主链路大致如下：

```text
命令注册
  -> handler trie / bot route 注册
  -> Bot 端点收到请求
  -> 重新 MatchCommandHandler
  -> handler 解析原文并产出 ResolvedCommand
  -> bridge.Execute(...)
  -> bridge_x.go 再按 module / mode 二次分发
  -> render / controller 执行
```

当前实现存在以下结构性问题：

1. 同一条命令被拆成“解析层”和“执行层”两部分，分别分散在 `internal/pjsk/handler/sekai` 与 `internal/pjsk/handler/bridge_*.go` 中，导致一个功能横跨两套文件与两套分发逻辑。
2. Bot 端点已经收到客户端上传的 `matched_command`，但仍会重新基于消息文本做一次全局命中，导致运行时存在冗余选路。
3. `ResolvedCommand -> bridge -> executeX -> switch mode` 形成了重复的字符串分发链，`Params` 在命令解析与执行之间以 `json.RawMessage` 来回序列化/反序列化，增加复杂度。
4. 仓库中已经出现两套重复的 handler registry：
   - `internal/pjsk/handler/*`
   - `internal/handler/*`
   当前处于半重构状态，不应继续并存。
5. `internal/pjsk/onebot11` 并非 PJSK 独占能力，当前包路径不合理。
6. `parser` 包中混杂了“仍在使用的底层提取/解析能力”和“已不适合作为主链路核心的全局命令壳”，结构混乱。

本次重构的目的不是局部修补，而是直接替换当前不合理的命令系统结构。

## 2. 重构目标

### 2.1 目标链路

重构后的 PJSK Bot 主链路应收口为：

```text
反射注册命令
  -> 通用 registry 维护 command -> handler / path / route metadata
  -> Bot 端点按 matched_command 直接定位命令
  -> 校验当前命令属于当前 path
  -> 从消息中按 matched_command 提取 args
  -> PJSK 统一执行前置
  -> 命令直接执行
  -> 返回 onebot11.Message
```

运行时不再存在：

1. 重新全局匹配命令前缀。
2. `ResolvedCommand` 作为主运行时对象。
3. `bridge.Execute(...)` 作为统一二次分发入口。
4. `bridge_x.go` 按 `mode` 做再次 switch 的结构。

### 2.2 目标结构

#### `internal/handler`

作为唯一通用命令注册表，负责：

1. 命令注册。
2. 命令前缀匹配。
3. `matched_command` 精确查找。
4. `path -> command_prefixes` 的 route 元信息维护。
5. `ListBotRoutes()` 供 manifest / route 注册使用。

该层不负责：

1. PJSK 专用上下文。
2. OneBot11 消息业务解析。
3. 命令执行。
4. `ResolvedCommand`。

#### `internal/onebot11`

承接当前 `internal/pjsk/onebot11` 的全部实现，作为整个仓库的通用 OneBot11 基础包。

#### `internal/pjsk/handler`

作为 PJSK 唯一命令层，负责：

1. 命令定义。
2. 命令反射注册。
3. PJSK 命令上下文构造。
4. 参数提取。
5. 统一执行前置。
6. 直接执行并返回 `onebot11.Message`。

`internal/pjsk/handler/sekai` 子包将被提平并移除，所有命令文件直接放在 `internal/pjsk/handler` 包内。

## 3. 非目标

本次重构不以以下事项为目标：

1. 不保留旧接口兼容层。
2. 不维持 `ResolvedCommand` 在 Bot 主协议中的运行时角色。
3. 不保留旧的 `bridge` 文件作为过渡转发层。
4. 不扩展新的 Bot 协议字段。
5. 不在本次重构内重新设计 `command_manifests` 的对外契约。

## 4. 设计约束

### 4.1 保留反射注册

反射注册方式保留，但实现位置调整。

新的实现方式：

1. 在 `internal/pjsk/handler` 包内定义一个零值命令集合结构体，例如 `commandSet`。
2. 各命令文件将方法挂载在 `commandSet` 上。
3. 启动时通过反射扫描 `commandSet` 的方法，完成命令注册。

这样既保留现有“按方法注册”的编写习惯，也能移除 `sekai` 子包带来的分层割裂。

### 4.2 Manifest 契约尽量保持不变

`command_manifests` 的生成入口仍保持为 `ListBotRoutes()` 驱动。

允许调整：

1. route 元信息的内部来源。
2. route registry 的内部实现。

暂不调整：

1. `command_module`
2. `command_path`
3. `command_prefixes`
4. `command_mode`
5. `command_additional_params`

### 4.3 统一执行前置必须单独抽象

当前 bridge 中已有的横切逻辑不能散落回各命令中复制，必须收口成 PJSK 命令统一前置。

至少包含：

1. requester ban 检查。
2. 默认区服解析。
3. 用户时区注入。
4. `RequestContext` / runtime 构造。
5. binding / snapshot / profile 等请求级懒加载入口。

## 5. 包级调整方案

### 5.1 `internal/pjsk/onebot11` -> `internal/onebot11`

调整方向：

1. 整包迁移。
2. 全仓 import 同步替换。
3. 旧路径彻底删除。

原因：

1. 当前 bot api、alias、handler runtime 等多处都在使用，不属于 PJSK 专用能力。
2. 保留在 `internal/pjsk` 下会继续误导后续结构设计。

### 5.2 `internal/pjsk/handler/sekai` -> `internal/pjsk/handler`

调整方向：

1. `sekai` 子目录下的命令文件全部提平。
2. 命令方法继续通过反射注册，但不再跨包。
3. 原 `bridge_*.go` 中的执行逻辑按命令域合并回同包内。

说明：

1. “提平”不等于把所有逻辑写成大文件。
2. 允许保留私有 helper、typed params、domain-specific 子函数。
3. 允许同一命令域拆分多个文件，但它们都属于同一个 package。

### 5.3 `internal/pjsk/handler/handler.go` 的处理

当前仓库同时存在：

1. `internal/pjsk/handler/handler.go`
2. `internal/handler/handler.go`

这是重复结构，必须收敛。

处理原则：

1. `internal/handler` 作为唯一通用 registry 保留。
2. `internal/pjsk/handler/handler.go` 删除。
3. `internal/pjsk/handler/bot_route.go` 删除。
4. `internal/handler/context.go` 如果继续依赖 PJSK / OneBot11 业务类型，则重做或删除，不保留“伪通用上下文”。

### 5.4 `parser` 包的清理

保留：

1. `Extractor`
2. `EventParser`
3. `CommandParser`
4. 其他仍被命令直接使用的底层解析能力

删除或退役：

1. `ResolvedCommand`
2. `GlobalCommandResolver`
3. 其他仅为旧主链路服务的类型或入口

要求：

1. 不允许继续保留“运行时主链路已废弃，但类型还留在 parser/types.go 里”的混乱状态。
2. `parser/types.go` 中若同时承载“保留类型”和“待删除类型”，则需拆分。

### 5.5 `requestbuilder` 的处理

`internal/pjsk/requestbuilder` 曾直接接收 `*parser.ResolvedCommand`，属于旧命令壳的一部分；该依赖现已移除。

本次重构采用“前者”方案，即继续保留 `requestbuilder` 作为独立层，但其接口重构为接收 typed params / runtime context，而非 `ResolvedCommand`。

调整原则：

1. `requestbuilder` 可以保留，但其输入必须去壳。
2. 不允许为了保留 `requestbuilder` 而继续保留 `ResolvedCommand`。
3. `requestbuilder` 中仅作为旧桥接层存在的辅助函数应一并清理。

## 6. 目标运行时模型

### 6.1 命令注册模型

命令需要在注册时完整提供以下信息：

1. `Commands`
2. `Path`
3. `Priority`
4. `Helper`
5. PJSK 专用解析开关，例如：
   - `Regions`
   - `PrefixArgs`
   - `ParseUIDArg`

命令注册结果既用于：

1. 消息命中。
2. 精确 command lookup。
3. Bot route 注册。
4. manifest seed。

### 6.2 Bot API 入口模型

新的 `api/bot/pjsk/handler.go` 应改为：

1. 解析请求体，恢复 `onebot11.Message`。
2. 根据 `matched_command` 精确查找命令。
3. 校验该命令 `path == 当前 endpoint path`。
4. 使用 `ExtractCommandArgs(messageText, matched_command)` 校验原始文本确实由该命令前缀触发，并提取剩余参数。
5. 构造 PJSK 命令上下文。
6. 进入统一执行前置。
7. 直接执行命令。
8. 返回 `onebot11.Message`。

明确删除：

1. `resolveBotCommand(...)`
2. 重新 `MatchCommandHandler(...)`
3. `ResolvedCommand -> Execute(...)`
4. 历史兼容分支，例如旧 path 迁移兼容判断

### 6.3 PJSK 统一执行前置模型

建议抽象一个统一入口，例如：

```text
prepare -> runtime -> command execute
```

其中：

1. `prepare`
   - 补 requester 信息
   - 处理 transport-level server
   - 默认区服决议
   - ban 检查
   - request timezone 注入
2. `runtime`
   - 构造 `RequestContext`
   - 提供 binding / snapshot / profile / target 等懒加载能力
3. `command execute`
   - 命令直接产出 `onebot11.Message`

目标是让横切逻辑只存在一份，不在各命令中复制。

## 7. 明确删除清单

本次重构结束后，以下结构不应继续存在于主链路中：

1. `parser.ResolvedCommand`
2. `parser.GlobalCommandResolver`
3. 旧桥接总分发文件及其命名残留
4. `internal/pjsk/handler/bridge_*.go` 中仅承担 module/mode 分发职责的外层
5. `internal/pjsk/handler/sekai/`
6. `internal/pjsk/handler/handler.go`
7. `internal/pjsk/handler/bot_route.go`
8. `internal/pjsk/onebot11`
9. `api/bot/pjsk/handler.go` 中依赖 `ResolvedCommand` 的旧分发逻辑
10. 不再被主链路使用的 parser / helper / test

## 8. 实施步骤

### 阶段 1：先落文档并冻结目标

1. 写入本方案文档。
2. 明确最终目标结构。
3. 明确删除范围与测试计划。

### 阶段 2：通用基础层收敛

1. 迁移 `internal/pjsk/onebot11` 到 `internal/onebot11`。
2. 收敛 `internal/handler` 为唯一通用 registry。
3. 删除 `internal/pjsk/handler/handler.go` 与 `internal/pjsk/handler/bot_route.go`。
4. 确保 route / manifest 仍可从唯一 registry 获取。

### 阶段 3：PJSK 命令层提平

1. 将 `internal/pjsk/handler/sekai` 的命令文件提平到 `internal/pjsk/handler`。
2. 保留反射注册，但改为扫描同包中的命令集合结构体。
3. 将命令定义与其执行 helper 收拢到同一个 package。

### 阶段 4：建立统一执行前置

1. 从旧桥接入口中抽取横切逻辑。
2. 建立新的统一执行入口。
3. 将 `RequestContext` 与 runtime 绑定到新执行模型。

### 阶段 5：逐域合并执行逻辑

建议按功能域逐批改造：

1. `event`
2. `music`
3. `profile`
4. `education`
5. `score`
6. `deck`
7. `mysekai`
8. `sk`
9. 其他尾部模块

其中已完成回收的域：

1. `misc`
2. `vlive`
3. `alias`
4. `gacha`
5. `arrest`
6. `regtime`
7. `stamp`
8. `checkdata`
9. `card`
10. `event`
11. `profile`
12. `education`
13. `score`
14. `sk`
15. `mysekai`
16. `deck`
17. `music`

每一批都完成：

1. 参数提取收口。
2. 旧 `bridge_x.go` 分发移除。
3. 命令直接执行。
4. 对应测试改写。

### 阶段 6：清理 parser / requestbuilder

1. 删除 `ResolvedCommand`。
2. 删除 `GlobalCommandResolver`。
3. 将 `requestbuilder` 改为 typed API。
4. 删除已失效测试与遗留适配代码。

当前状态：

1. 已完成。

### 阶段 7：重写 Bot API 入口

1. 切换为 `matched_command` 精确命中。
2. 用 `ExtractCommandArgs` 代替重新全局匹配。
3. 直接执行命令。
4. 清除旧兼容逻辑。

### 阶段 8：manifest 与测试收尾

1. 校验 `ListBotRoutes()` 输出稳定。
2. 校验 `SeedCommandManifests()` 行为保持正确。
3. 跑完新测试矩阵。
4. 清理所有已废弃结构与文档引用。

## 9. 测试计划

### 9.1 通用 registry 测试

覆盖：

1. 命令注册。
2. 前缀匹配。
3. 精确 `matched_command` 查找。
4. `ExtractCommandArgs(...)` 行为。
5. `ListBotRoutes()` 输出。

### 9.2 OneBot11 / 上下文测试

覆盖：

1. 消息段解析。
2. 文本提取。
3. `at` 提取。
4. PJSK command context 构造。

### 9.3 PJSK 统一前置测试

覆盖：

1. requester ban。
2. 默认区服解析。
3. transport-level server 覆盖。
4. request timezone 注入。
5. runtime / `RequestContext` 构造。

### 9.4 命令级测试

命令测试不再以“是否产出 `ResolvedCommand`”为主，而改为：

1. 参数提取 helper 单测。
2. 命令直接执行单测。
3. 错误路径单测。
4. 典型 route/path 校验。

### 9.5 Bot API 集成测试

覆盖：

1. `matched_command` 必填。
2. `matched_command` 不属于当前 path。
3. 消息文本与 `matched_command` 前缀不一致。
4. 正常直达执行。
5. manifest seed 正确生成 path 与 commands。

## 10. 风险与注意点

### 10.1 风险：重构范围过大

缓解方式：

1. 先收敛通用基础层。
2. 再按命令域逐批迁移。
3. 每一阶段都删除旧路径，而不是长期双轨并存。

### 10.2 风险：测试大面积失效

说明：

1. 旧测试大量断言 `ResolvedCommand`。
2. 本次重构完成后，这类测试本身不再成立。

处理原则：

1. 允许删除旧测试。
2. 允许重写测试结构。
3. 不为了“保留旧测试”而保留旧运行时壳。

### 10.3 风险：manifest 内部实现被牵动

说明：

1. manifest 当前依赖 route registry。
2. route registry 将被重写到 `internal/handler`。

处理原则：

1. manifest 外部契约不扩。
2. 优先维持 `ListBotRoutes()` 接口稳定。
3. 若后续实现过程中发现 manifest 必须调整，则单独记录并向需求方确认。

## 11. 里程碑判定

本次重构完成的判定标准：

1. `internal/pjsk/handler/sekai` 目录不存在。
2. 旧桥接总分发不存在，`bridge` 命名残留不再承担主链路职责。
3. `ResolvedCommand` 不再参与 Bot 主运行链路。
4. `api/bot/pjsk/handler.go` 不再重新全局匹配命令。
5. `internal/handler` 成为唯一通用命令 registry。
6. `internal/onebot11` 成为唯一 OneBot11 基础包。
7. manifest 仍能正确 seed。
8. 新测试矩阵通过。

## 12. 后续执行原则

后续所有实际代码修改都应遵守以下原则：

1. 优先删除不合理结构，而不是给旧结构再包一层。
2. 不保留与最终目标冲突的兼容壳。
3. 保留反射注册，但不保留跨包割裂。
4. 命令解析与执行应收口为单条清晰链路。
5. 允许大改测试，不允许为了测试而保留旧架构。
