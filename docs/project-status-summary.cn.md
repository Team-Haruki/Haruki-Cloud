# Haruki-Cloud 项目进展总结

> 最后更新：2026-03-24
>
> 涉及 `Haruki-ZeroBot` 联调的协议边界，请优先参考 `docs/zerobot-cloud-integration-plan.cn.md`。

## 1. 当前结论

`Haruki-Cloud` 当前已经完成两部分核心合并：

1. `Service-Test` 的渲染子系统
2. `Test_Instruction_Parser` 的解析资源和处理资源

但 PJSK 指令主链路的目标模型已经重新明确：

1. 云端下发 manifest。
2. 客户端构建前缀树并命中 `path`。
3. 客户端按命中的 `path` 调用 `/api/v2/bot/:botId/pjsk/*`，并上传 `matched_command`。
4. 命中的端点在云端内部先校验 `matched_command -> handler.path`，再解析原文、提取参数，并进入统一执行链路返回图片或文本。

也就是说，客户端负责“命中哪个端点”，云端端点负责“这个端点到底怎么解释原文”。

## 2. 已完成的合并内容

### 2.1 渲染子系统

`Service-Test` 的渲染能力已经稳定落在：

- `internal/pjsk/render/`
- `/internal/pjsk/render`
- `/internal/pjsk/<module>/<action>/build|render`

覆盖模块包括：

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

### 2.2 解析与处理资源

`Test_Instruction_Parser` 中与解析、提取、处理相关的核心资源已经并入：

- `internal/pjsk/parser/`
- `internal/pjsk/handler/`
- `internal/pjsk/chardata/`

其中应继续保留的重点是：

1. 通用提取器
2. 类型化解析器
3. 业务执行与 render 桥接能力

## 3. 当前目标协议

当前 PJSK Bot 协议的目标边界如下：

### 3.1 客户端入口

- `GET /api/v2/bot/:botId/command/manifests`

### 3.2 业务端点

- `GET /api/v2/bot/:botId/pjsk/<path>?command_payload=<base64(ob11 pack)>`

### 3.3 内部兼容入口

- `POST /internal/pjsk/command`

需要明确：

`/internal/pjsk/command` 是内部兼容入口，不是客户端主协议。

## 4. 当前对指令系统的重新判断

经过本轮文档整理，已经明确下面这点：

PJSK 指令系统不应再把“云端全局 resolver 重新选 module + mode”写成客户端主链路。

主链路应当是：

1. `manifest -> 本地前缀树 -> path`
2. `path -> 对应端点`
3. `端点 -> 解析原文 -> 提参 -> 处理`

因此：

1. `GlobalCommandResolver` 不应再被视为 Bot 主协议核心
2. Trie 分发不应再被视为客户端主协议核心
3. 端点内解析原文，才是后续应收口的主模型

## 5. 当前代码状态说明

当前代码中已经具备：

1. manifest
2. `/api/v2/bot/:botId/pjsk/*` 直达型端点
3. render runtime
4. 提取器与类型化解析器
5. 基于 `internal/pjsk/handler.GetPath()` 的命令归属校验
6. 基于 handler 的 manifest 同步

但仍有一些收尾项需要继续注意：

1. 部分旧 handler 仍然带有历史别名和多义语义，需要继续梳理 path 归属
2. `/internal/pjsk/command` 仍是内部兼容入口，不能再当作客户端主入口
3. 现有客户端必须按新的 `matched_command` 协议联调

这些内容后续需要继续收口。

## 5.1 账号绑定能力状态

截至 2026-03-24，PJSK 账号绑定相关链路已经完成一轮收口：

1. 绑定命令已经接入 `/api/v2/bot/:botId/pjsk/profile/*`
2. handler 只返回 `ResolvedCommand`
3. 文本型绑定命令与图片型命令统一走 `commandhandler.Execute(...)`
4. `Execute(...)` 统一返回 `payload + data_type`
5. 绑定业务执行和文本格式化已下沉到 `internal/pjsk/userdata/`

更详细的实现说明、代码落点、测试覆盖和注意事项，请直接参考：

- [PJSK 账号绑定实现说明](pjsk-profile-binding-implementation.cn.md)

## 6. 当前保留项

下面这些内容目前明确保留：

1. `command_manifests`
2. 基于 `internal/pjsk/handler` registry 派生的 Bot route 元数据
3. `/api/v2/bot/:botId/pjsk/*` 直达型端点
4. `internal/pjsk/parser` 中的通用提取与类型化解析能力
5. render runtime 与内部 build/render 路由

## 7. 当前不再作为主链路的内容

下面这些内容不再应被文档描述为客户端主链路：

1. `GlobalCommandResolver -> module + mode -> bridge`
2. 云端 Trie 分发先重新决定目标端点
3. 客户端直接调用 `/internal/pjsk/render`
4. 客户端直接调用 `/internal/pjsk/command`

## 8. 仍然存在的技术债

当前主要技术债包括：

1. 部分 path 仍需继续剥离历史多义命令
2. 强用户态模块仍然依赖本地 JSON 快照
3. MySekai 仍有本地 masterdata fallback
4. Deck 当前仍是 Go 方案，旧 CGo 引擎未恢复为默认链路
5. 已存在的 `command_manifests` 若被人工特殊维护，仍需确认新的 handler-source 同步结果是否符合预期

## 9. 相关文档

- [PJSK 指令系统设计](pjsk-command-system.cn.md)
- [PJSK 账号绑定实现说明](pjsk-profile-binding-implementation.cn.md)
- [ZeroBot 与 Cloud 联调方案](zerobot-cloud-integration-plan.cn.md)
- [ZeroBot 渲染接入后续事项](zerobot-render-followup.cn.md)
