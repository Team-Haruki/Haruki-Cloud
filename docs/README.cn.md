# Haruki-Cloud 文档索引

> 快速找到你需要的文档

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
- `internal/pjsk/parser/` — 指令解析核心（GlobalCommandResolver、Extractor）
- `internal/pjsk/handler/` — 指令分发与调用桥（Trie 树、SekaiCommandHandler、Bridge）
- `internal/pjsk/chardata/` — 角色昵称数据加载器

详见 [项目进展总结](project-status-summary.cn.md) 中的完整说明。

## 📐 后续设计

- **[用户快照 Provider 设计](pjsk-user-snapshot-provider-design.cn.md)**
  - 正式 Provider 架构设计

- **[ZeroBot 后续接入](zerobot-render-followup.cn.md)**
  - Bot 端调用方式更新说明

## 🆕 最近更新

| 日期 | 文档 | 变更 |
|------|------|------|
| 2026-03-23 | 项目进展总结 | v2.1：补充 P0 阻塞项说明及阶段小结 |
| 2026-03-23 | 项目进展总结 | v2.0：Parser 合并完成 |
| 2026-03-23 | README 索引 | 清理已过时文档链接 |
| 2026-03-21 | Service-Test 合并状态 | 完成总结 |

---

**维护者**：Haruki-Cloud Team  
**最后更新**：2026-03-23 04:55 UTC
