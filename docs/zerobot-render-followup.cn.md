# ZeroBot 渲染接入后续事项

> 最后更新：2026-03-24

这份文档现在只保留一个作用：

提醒后续对接应以新的联调方案为准，而不是继续按旧的内部渲染接口思路推进。

## 当前结论

`Haruki-ZeroBot` 后续对接 `Haruki-Cloud` 时，应采用：

1. 拉取 manifest
2. 构建本地前缀树
3. 命中 `/api/v2/bot/:botId/pjsk/*`
4. 由命中的云端端点解析原文并渲染

不再以“客户端直接调用 `/internal/pjsk/render`”作为接入目标。

## 主要参考文档

- [ZeroBot 与 Cloud 联调方案](zerobot-cloud-integration-plan.cn.md)
- [PJSK 指令系统技术文档](pjsk-command-system.cn.md)

## 当前不再采用的方案

以下方向不再作为 ZeroBot 对接目标：

1. 直接调用 `/internal/pjsk/render`
2. 直接调用 `/internal/pjsk/command`
3. 客户端承担最终命令语义解析
4. 继续沿用旧 `/api/render` 或 `Service-Test` 风格路径
