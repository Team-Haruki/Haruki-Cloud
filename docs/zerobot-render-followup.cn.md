# ZeroBot 渲染接入后续事项

## 目的

这份文档只记录 `Haruki-ZeroBot` 在 `Service-Test` 合并完成后需要跟进的接入事项。

当前阶段的原则是：

- 只做 `Haruki-Cloud` 内部合并；
- 不修改 `Haruki-ZeroBot` 代码；
- 不保留 `Service-Test` 运行时兼容层；
- 必要说明保留在文档中。

## 当前结论

`Haruki-Cloud` 已经逐步承接原 `Service-Test` 的渲染能力，标准内部入口应当收敛到：

- 模块化内部路由，例如 `POST /internal/pjsk/card/detail/build`
- 统一分发入口 `POST /internal/pjsk/render`

不再建议在机器人侧继续依赖旧的：

- `/api/render`
- `/api/card/...`
- `/api/music/...`
- 其他 `Service-Test` 风格路径

## ZeroBot 后续需要做的事

### 1. 更新云端渲染调用目标

`ZeroBot` 侧后续应改为直接调用 `Haruki-Cloud` 的内部渲染接口，而不是再经过 `Service-Test`。

推荐优先使用：

- `POST /internal/pjsk/render`

请求体采用 `Haruki-Cloud` 当前持有的统一契约：

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

其中：

- `target` 对应 `Haruki-Cloud` 内部模块路径，如 `music/detail`、`event/list`、`mysekai/resource`
- `operation` 只允许 `build` 或 `render`
- `payload` 为目标模块原本的 JSON 请求体

### 2. 调整命令解析到渲染契约的映射

如果 `ZeroBot` 仍然保留类似旧 `module + mode + params` 的解析结果，则需要在机器人侧增加一层显式映射：

- 从旧的命令语义映射到新的 `target`
- 明确选择 `build` 或 `render`
- 按 `Haruki-Cloud` 各模块请求结构组装 `payload`

这里不要再把旧 `Service-Test` 的宽松行为带回来：

- 不要静默吞掉 JSON 组装错误
- 不要依赖缺省零值继续往下走
- 不要假设旧的 `/api/render` 会长期存在

### 3. 保留内部鉴权约定

`ZeroBot` 后续接入时应继续遵循 `Haruki-Cloud` 当前内部接口鉴权方式：

- `Authorization`
- `User-Agent`

具体值继续由 `Haruki-Cloud` 配置决定，不在机器人侧硬编码旧服务约定。

### 4. 清理旧 Service-Test 假设

机器人侧后续改造时，应一并清理这些遗留假设：

- 单独的 `Service-Test` 基础 URL
- 面向旧 `/api/render` 的固定请求结构
- 依赖旧服务返回格式的分支逻辑

如果未来还需要兼容历史调用方，也应放在边缘入口层做短期转换，而不是重新写回 `Haruki-Cloud` 或 `ZeroBot` 主干逻辑。

### 5. 补测试

`ZeroBot` 真正开始接入时，至少应补两类测试：

- 命令解析结果到 `target/operation/payload` 的映射测试
- 面向 `Haruki-Cloud` 内部接口的 client contract tests

## 建议落地顺序

1. 先以文档确认 `target` 映射表。
2. 再修改 `ZeroBot` cloud client，请求 `Haruki-Cloud` 内部接口。
3. 最后删除残留的 `Service-Test` 专用调用路径和协议适配代码。

## 不在本阶段实现的内容

以下内容不属于当前这轮合并任务：

- `Haruki-ZeroBot` 代码修改
- 机器人侧命令路由重写
- 机器人侧兼容层保留
- 机器人侧对旧 `Service-Test` 协议的继续扩展
