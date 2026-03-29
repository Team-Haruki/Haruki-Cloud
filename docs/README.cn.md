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

- **[PJSK 歌曲查询统一改造方案](pjsk-music-query-unification-plan.cn.md)** ⭐
  - 整理歌曲查询、难度提取、排行参数抽取与 `deck` 指定歌曲的统一方向
  - 适合：后续继续修 `music/score/deck` 相关参数语义时对照

- **[PJSK 账号绑定实现说明](pjsk-profile-binding-implementation.cn.md)** ⭐
  - 详细记录 2026-03-24 ~ 2026-03-25 这轮账号绑定、Profile 设置、Execute 返回类型、`handler`/`userdata` 分层收口
  - 适合：查看今天这轮代码修改的完整背景和落地结果

- **[PJSK Event Tracker 对接说明](pjsk-event-tracker-integration.cn.md)** ⭐
  - 记录 SK tracker 接入点、参数协议、`@用户` 绑定解析和能力矩阵
  - 适合：联调 `sk/skl` 与排查 tracker 查询链路

- **[PJSK Virtual Live 文本版实现方案](pjsk-vlive-text-plan.cn.md)** ⭐
  - 记录 `vlive` 文本链路的实现范围、过滤规则、代码落点与测试覆盖
  - 适合：查看 Virtual Live 当前已实现能力与未实现边界

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
- `GET /api/v2/bot/:botId/pjsk/<path>?command_payload=<base64(ob11 pack)>`

```http
GET /api/v2/bot/:botId/pjsk/card/detail?command_payload=<base64(ob11 pack)>
X-Haruki-Bot-Platform: qq
X-Haruki-Bot-Platform-User-Id: 12345
X-Haruki-Bot-Platform-Group-Id: 67890
X-Haruki-Bot-Pjsk-Server: jp
X-Haruki-Bot-Matched-Command: /卡面
```

其中：

- `command_payload` 是客户端通过 OneBot V11 协议拿到的消息原文包，做 Base64 后放到查询参数里。
- `X-Haruki-Bot-Matched-Command` 表示客户端前缀树实际命中的那条命令。

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
| 2026-03-29 | PJSK 歌曲查询统一改造方案 / README 索引 | 新增歌曲查询统一方案文档，整理 `music` `score/music-board` `score/music-meta` `deck` 的统一改造方向 |
| 2026-03-28 | 客户端对接指南 / 项目进展总结 | 同步 MySekai 指令别名约定（`msa/msm/msr`）与地图编号选图规则（`/msm 1-4`） |
| 2026-03-26 | PJSK Virtual Live 文本版实现方案 / README 索引 / 架构文档 / 项目进展总结 | 同步 `vlive` 文本功能落地、render 模块数量与 disabled stub 清单 |
| 2026-03-26 | PJSK 指令系统设计 / 项目进展总结 / 数据库 Schema 文档 | 将歌曲别名独立到 Alias 模块，并补充角色别名、审核与删除指令说明 |
| 2026-03-25 | PJSK 账号绑定实现说明 | 补充 Profile 设置能力、`suite_visible` / `mysekai_visible` 语义、验证现状与背景图存储规则 |
| 2026-03-25 | 项目进展总结 | 同步 Profile 设置落地状态与当前语义 |
| 2026-03-25 | PJSK Event Tracker 对接说明 | 新增 tracker 对接专题文档（含 `@用户` 解析方案） |
| 2026-03-24 | PJSK 账号绑定实现说明 | 新增账号绑定与执行链路收口专题文档 |
| 2026-03-24 | README 索引 | 补充 `matched_command`，并改正 Bot 业务路径描述 |
| 2026-03-24 | ZeroBot 与 Cloud 联调方案 | 明确 `/api/v2/bot/*` 为客户端联调主协议 |
| 2026-03-24 | ZeroBot 后续接入 | 改为引用新的联调方案 |
| 2026-03-24 | README 索引 | 改正客户端主调用入口说明 |
| 2026-03-21 | Service-Test 合并状态 | 完成总结 |

---

**维护者**：Haruki-Cloud Team  
**最后更新**：2026-03-29
