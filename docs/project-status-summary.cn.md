# Haruki-Cloud 项目进展总结

> 最后更新：2026-03-23 04:55 UTC

## 📊 项目概览

**Haruki-Cloud** 是 HarukiBot 的核心后端服务，当前正在进行**多个独立服务的统一合并**工作。

## 🎯 合并项目状态一览

| 项目 | 原用途 | 状态 | 备注 |
|------|--------|------|------|
| **Service-Test** | 渲染服务（Part2） | ✅ 已完成 | 已合并进 `internal/pjsk/render` |
| **Test_Request_Construction** | Service-Test 的别名/镜像 | ✅ 已覆盖 | 实际就是 Service-Test |
| **Test_Instruction_Parser** | 指令解析（Part1） | ✅ 已完成 | 已合并进 `internal/pjsk/parser` + `handler` + `chardata` |

## ✅ Service-Test 合并（已完成）

**完成时间**：2026-03-21  
**详细文档**：`docs/service-test-merge-status.cn.md`

### 已完成内容

1. **渲染子系统完全迁移**（12 个模块）：
   - ✅ card, music, gacha, event
   - ✅ education, honor, profile, stamp
   - ✅ misc, score, deck, sk, mysekai

2. **统一路由体系**：
   - ✅ `POST /internal/pjsk/render` - 统一分发入口
   - ✅ `POST /internal/pjsk/<module>/<action>/build|render` - 模块化路由

3. **测试覆盖**：全部通过

### 临时实现（技术债）

1. **本地用户快照** - 仍使用 `user.json`、`music_metas.json`、`mysekai.json`
2. **MySekai masterdata** - 依赖本地文件，未完全转为 DB 驱动
3. **Deck 引擎** - 简化版实现，未迁入原生 CGo 引擎

## ✅ Test_Instruction_Parser 合并（已完成）

**完成时间**：2026-03-23

### 已完成内容

1. **Parser 核心迁移**（`internal/pjsk/parser/`，8 个文件）：
   - ✅ `GlobalCommandResolver` — 正则路由表，指令→模块+模式
   - ✅ `Extractor` — 角色昵称/稀有度/属性/技能/区服/年份 提取器
   - ✅ `CardParser` / `MusicParser` / `EventParser` — 类型化查询解析
   - ✅ `CommandParser` — SK/bind 命令解析
   - ✅ 测试全部通过

2. **Handler 子系统迁移**（`internal/pjsk/handler/`）：
   - ✅ Trie 树指令分发器（`handler.go`）
   - ✅ `SekaiCommandHandler` — 反射自动注册（`sekai/handler.go`）
   - ✅ 14 个功能模块 handler（card, event, chart, music, deck, gacha, education, entertainment, misc, mysekai, profile, score, sk, stamp, vlive）

3. **区域类型统一**：
   - ✅ `sekai_region.SekaiRegion` → `render/region.Value` 全量替换
   - ✅ 不再有重复的区域类型定义

4. **Chardata 加载器**（`internal/pjsk/chardata/loader.go`）：
   - ✅ 从数据库加载角色昵称映射
   - ✅ 后台定时刷新

5. **Bridge 调用桥**（`internal/pjsk/handler/bridge.go`）：
   - ✅ `Execute(ctx, resolved, app)` — ResolvedCommand 直接路由到 render Controller
   - ✅ 覆盖全部 12 个模块的所有模式
   - ✅ 零 HTTP 开销，纯 Go 函数调用

6. **配置接入**：
   - ✅ `config.PJSKParserConfig` 添加（chardata_region, refresh_interval）
   - ✅ `cmd/server/main.go` 中初始化 chardata + resolver
   - ✅ `haruki-db-configs.example.yaml` 更新

### 调用链（已实现）

```
Bot → API 层（待定义）
    → parser.GlobalCommandResolver.Resolve(message)
    → parser.ResolvedCommand{Module, Mode, Query, Region, Params}
    → handler.Execute(ctx, resolved, renderApp)
    → render Controller 直接调用
    → Drawing API → PNG
```

## 📋 后续待办事项

### P0 - 必须完成（阻塞 Bot 上线）

1. **定义 Bot API 层端点**（协议待确认）
   - 新增 `POST /internal/pjsk/process` 或类似端点
   - 接收 Bot 消息 payload（格式 TBD）
   - 串联：`GlobalCommandResolver.Resolve()` → `handler.Execute()` → 返回 PNG
   - **前置条件**：Bot 侧调用协议（消息格式、鉴权方式、用户上下文字段）需先确认
   - 技术准备：`GlobalCommandResolver` 已在 `main.go` 初始化，但返回值当前未使用

### P1 - 重要但不紧急

2. **正式用户快照 Provider**（技术债偿还）
   - 设计已完成：`docs/pjsk-user-snapshot-provider-design.cn.md`
   - 当前仍依赖本地 `user.json`、`music_metas.json`、`mysekai.json`

3. **Bot 调用方切换**
   - Haruki-ZeroBot 改调新接口
   - 详见：`docs/zerobot-render-followup.cn.md`

### P2 - 可选优化

4. **MySekai 完全 DB 驱动**（当前依赖本地 masterdata 文件）
5. **Deck 引擎收口**（如需原生 CGo 引擎）

## 📈 项目进度总览

```
总体进度: ████████████████░░░░ 80%

Service-Test 合并:     ████████████████████ 100% ✅
Test_Request_Constr:   ████████████████████ 100% ✅
Parser 合并:           ████████████████████ 100% ✅
Bot API 层:            ░░░░░░░░░░░░░░░░░░░░   0% ⚠️ (协议待确认)
用户快照 Provider:     ░░░░░░░░░░░░░░░░░░░░   0% 📝
Bot 切换:              ░░░░░░░░░░░░░░░░░░░░   0% 📝
```

### 当前阶段小结（2026-03-23）

两大子系统（渲染 + 解析）已完全合并并通过编译和测试，调用链从指令解析到图像输出的技术路径已打通。  
唯一阻塞项是 Bot API 层端点——协议确认后可快速实现（基础设施已就位）。

## 📚 相关文档

### 已完成合并
- `docs/service-test-merge-plan.cn.md` - Service-Test 合并方案
- `docs/service-test-merge-status.cn.md` - Service-Test 合并状态

### 后续事项
- `docs/pjsk-user-snapshot-provider-design.cn.md` - 用户快照 Provider 设计
- `docs/zerobot-render-followup.cn.md` - ZeroBot 接入说明

---

**维护者**：Haruki-Cloud Team  
**文档版本**：v2.1  
**创建日期**：2026-03-23
