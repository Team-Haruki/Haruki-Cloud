# Haruki-Cloud 文档索引

> 快速找到你需要的文档

## ⭐ 当前联调入口

- **[ZeroBot 与 Cloud 联调方案](zerobot-cloud-integration-plan.cn.md)** ⭐
  - 当前客户端联调的权威方案
  - 明确 manifest、前缀树、本地命中、`/api/v2/bot/*` 端点、端点内解析原文

## 📌 项目概览

- **[项目进展总结](project-status-summary.cn.md)** ⭐
  - 当前状态、已完成工作、待办事项
  - 适合：快速了解项目全貌

## 🎯 Service-Test 合并（已完成）

- **[合并方案](service-test-merge-plan.cn.md)**
  - 详细的合并计划和架构设计

- **[合并状态](service-test-merge-status.cn.md)** ⭐
  - 实际落地结果

## 🎯 Test_Instruction_Parser 合并（已完成）

Parser 已作为内部子系统合并进 Haruki-Cloud：
- `internal/pjsk/parser/` — 通用提取器与类型化解析器
- `internal/pjsk/handler/` — 业务处理与执行桥
- `internal/pjsk/chardata/` — 角色昵称数据加载器

详见 [项目进展总结](project-status-summary.cn.md) 中的完整说明。

## 🔌 Bot 指令 API（已完成）

客户端联调应优先使用：

- `GET /api/v2/bot/:botId/command/manifests`
- `GET|POST /api/v2/bot/:botId/pjsk/<path>`

```json
{
  "im_platform": "qq",
  "im_user_id": "12345",
  "command": "/卡面 1001",
  "matched_command": "/卡面",
  "server": "jp"
}
```

`matched_command` 表示客户端前缀树实际命中的那条命令。

`POST /internal/pjsk/command` 仅保留给内部兼容场景，不是客户端主协议。

详见 [ZeroBot 与 Cloud 联调方案](zerobot-cloud-integration-plan.cn.md)。

## 📐 后续设计

- **[用户快照 Provider 设计](pjsk-user-snapshot-provider-design.cn.md)**
  - 正式 Provider 架构设计

- **[ZeroBot 后续接入](zerobot-render-followup.cn.md)**
  - ZeroBot 接入提醒，具体以联调方案为准

## 🆕 最近更新

| 日期 | 文档 | 变更 |
|------|------|------|
| 2026-03-24 | README 索引 | 补充 `matched_command`，并改正 Bot 业务路径描述 |
| 2026-03-24 | ZeroBot 与 Cloud 联调方案 | 明确 `/api/v2/bot/*` 为客户端联调主协议 |
| 2026-03-24 | ZeroBot 后续接入 | 改为引用新的联调方案 |
| 2026-03-24 | README 索引 | 改正客户端主调用入口说明 |
| 2026-03-21 | Service-Test 合并状态 | 完成总结 |

---

**维护者**：Haruki-Cloud Team  
**最后更新**：2026-03-24
