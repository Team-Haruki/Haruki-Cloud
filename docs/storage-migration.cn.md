# Haruki-Cloud 存储迁移发布表（Phase-2）

本文是运维视角的发布与回滚参考：Cloud 从本地目录迁到 `internal/storage` 槽位与 Garage，渲染缓存索引迁到 PostgreSQL。每一步的回滚都是配置翻转或重新部署上一个 tag，**没有任何一步需要恢复数据**。

配置优先级沿用 `AGENTS.md` §10：环境变量高于 `haruki-cloud.yaml`，排查时先看 `.env`。

## 1. 硬性顺序

1. **Cloud 读侧 → DDL → Drawing 写索引**，不可调换。
2. Cloud 永久接受“请求了 Artifact、收到 `image/*` 字节”（`X-Haruki-Cache-Store: 0`、Garage 写入降级、未升级的 Drawing）。
3. Cloud 的 C1 候选数组（R7 中的 T15）必须在 Drawing 接受候选列表之后发布。

## 2. 发布步骤

| 步骤 | 二进制 | 必需的配置变更 | 可观察效果 | 回滚 |
| --- | --- | --- | --- | --- |
| **R1** | T1–T8 | 必须设置 `asset_dirs.assets_base_url` **或** `assets_base_urls`（公开素材主机为空现在是启动错误）。其他无需变更，旧目录派生覆盖所有槽位 | 行为不变，存储代码处于休眠 | 上一个 tag |
| **R1b** | 同上 | 把 `storage.{assets,user_upload,static,cache,image_cache}` 显式写成 `scheme: fs` 与现有目录 | 在生产上验证槽位与旧路径等价 | 删除 `storage:` 块（恢复派生） |
| **R2** | T9（E1） | `asset_dirs.primary: ""` **并且** `storage.{assets,user_upload,static}.root` 设为原 primary 值 | stamp 门控移除，preview3d 与 profile_bg 通过槽位写入 | 恢复 `asset_dirs.primary` 并部署上一个 tag（T9 是单个可回退提交） |
| **R3** | T10、T11 | `image_cache.render_index.ddl_enabled: true` —— **安排在低峰期**：五条 `ALTER TABLE image_cache_entries`（4 个 `ADD COLUMN` + 1 个 `ALTER COLUMN`）在启动时短暂持有 ACCESS EXCLUSIVE 锁 | 表结构加宽，创建 `render_cache_index`；尚无读取 | 关闭开关；新增列和表保留（可空、有默认值，无害） |
| — | **Drawing 发布索引写入** | — | Drawing 开始为白名单端点写 `render_cache_index` | Drawing 自身的放量开关 |
| **R4** | T12、T13 | 先配置 `image_cache.hosts`（及 `host_order`），再设置 `drawing_artifact.endpoints: ["<一个 api path>"]`，最后 `image_cache.render_index.lookup_enabled: true` | 该端点返回 Artifact 引用，命中来自 PG；其他端点不变 | 清空白名单和/或 `lookup_enabled: false`，旧 `/cache` 路径恢复（T16 之前仍在编译） |
| **R5** | T14 | `image_cache.gc_enabled: true` 且 `gc_dry_run: true` → 观察至少一个周期 → `gc_dry_run: false`（保持 `gc_object_retention_days: 30`）；`image_cache.legacy_redirect.enabled: true`（**至少保留 30 天**）；翻转重定向之前先把旧 `image_cache.dir` `rclone` 到 `image-cache` 桶根目录 | GC 立即回收过期索引行，对象在 30 天保留期后才删除；`/ic/*` 返回 301 | 两个开关都关闭 |
| **R6** | 同一二进制 | profile_bg 翻转：`storage.user_upload.scheme: s3` + `mirror: {scheme: fs, root: <原 primary>}` → `rclone` 回填已有 `user_upload/profile_bg/**` → 删除 `mirror` → **在从节点停止 `sync-static-assets-from-primary.sh` 定时器** | 新上传同时写入 Garage 和本地镜像，之后只写 Garage | 交换 `scheme` / `mirror`（镜像期内本地副本仍是权威） |
| **R7** | T15（依赖 **Drawing T8**）、T16（依赖 T12/T13 + 运维确认 `/cache` 调用方） | 删除 `drawing_cache.*` 与 `image_cache.charts_uri` | 生日图标与五个存在性分叉改发候选数组（一次缓存未命中波）；`/cache`、SQLite 缓存与谱面缓存移除，谱面端点走 Artifact 路径 | 上一个 tag（仅这两步的回滚不是开关） |

## 3. 各步骤细节

### 3.1 DDL（R3）

- DDL 文本以 Cloud 的 `utils/imagecache/pgstore_ddl.go` 为唯一权威（Drawing 为逐字副本）。
- 语句全部可重复执行（`IF NOT EXISTS` / `DROP NOT NULL` / 仅回填 `storage_backend IS NULL`），首次开启后保持开启无额外成本。
- `render_cache_index.content_hash` 外键**不带级联**：误删仍被引用的 `image_cache_entries` 行会直接失败，这是 GC 顺序之外的安全网。

### 3.2 GC（R5）

- 开关：`image_cache.gc_enabled`（默认 `false`）、`gc_dry_run`（默认 `true`）、`gc_interval`（默认 `1h`）、`gc_batch`（默认 `500`）、`gc_object_retention_days`（默认 `30`）；环境变量 `HARUKI_PJSK_RENDER_IMAGE_CACHE_GC_{ENABLED,DRY_RUN,INTERVAL,BATCH,OBJECT_RETENTION_DAYS}`。不存在嵌套的 `render_index.gc.*` 写法。
- 需要 `image_cache.pg_url` 可用；索引不可用时记录 Warn 并不启动。
- 顺序：重试上轮遗留对象 → 阶段 1 删除过期 `render_cache_index` 行 → 阶段 2 对每个孤儿 `garage` 行**先删行、再删记录的 `cdn_path` 对象**。
- 阶段 2 判断：`storage_backend = 'garage'`、无 `render_cache_index` 引用、`last_referenced_at < now - retention`。`image_cache_entries.expires_at` 不参与；`legacy_disk` 行与永久 TTL 的渲染行（`expires_at IS NULL`）永不回收。
- 删除行时重新套用阶段 2 的全部条件（`hash`、`garage`、`last_referenced_at < cutoff`、无引用）：SELECT 之后被重新引用的行不会被删，也就不会删它的对象。
- 对象删除失败计入 `object_leaks` 并在下一周期重试（内存列表上限 10 000，满时丢弃最旧并 Warn；进程重启会丢失该列表，遗留对象需按 `pjsk/api/` 前缀离线核对）。
- 对象 key 按内容寻址：删行之后、删对象之前（含下一周期重试），Drawing 或 Cloud 可能用同样字节重新写入同一 key 并插入新行。因此每次删对象前按主键查一次 `image_cache_entries`，若已有行记录同一 `cdn_path`，放弃这次删除并计入 `skipped_live_object_deletes`；查询失败时本周期不删任何待删对象。
- `last_referenced_at` 由写入方维护：Drawing 的 `UPSERT_CONTENT` 冲突时总是更新；Cloud 的 `StoreAndGetURL` 在 `garage` 行去重命中时更新（每个 hash 每小时最多一次），插入冲突时也总是更新（位置列仍受 `garage` 保护）。因此仍在被 Cloud 复用的 `pjsk/<sha256>.<ext>` 行不会在插入 30 天后被回收。**Drawing 的 A2 复用命中（不经过 `UPSERT_CONTENT` 的路径）也必须更新 `last_referenced_at`，否则同样会在 30 天后被回收——这是对 A2/A6 的契约补充。**
- 日志：dry-run 每阶段一行（计数 + 最多 10 个样本），每个周期一行汇总 `image cache gc cycle`（失败时为 `image cache gc cycle failed`）。

### 3.3 `/ic/*` 重定向（R5）

- 默认：`static.New(image_cache.dir)`，与旧版本逐字节一致。
- `image_cache.legacy_redirect.enabled: true` 且配置了 `image_cache.hosts`（或旧 `image_cache.uri`）时：`/ic/<key>` 返回 301 到 `<主机>/<key>`；非法 key（`..`、控制字符、非法转义）返回 404；重定向响应带 `Cache-Control: public, max-age=86400`。开关开启但没有主机时继续提供目录并记录 Warn。
- 路由最早在开关开启 **30 天后**才能删除；把开启日期记在部署记录里，不写进代码。旧 `image_cache.uri` 主机名的 301 属于节点 Caddy 配置，不在本仓库。
- **rclone 回填**：旧 key 为 `<group>/<hash>.<ext>`，Cloud 只有 `pjsk` 分组，回填后位于 `pjsk/<hash>.<ext>`，与 Drawing 的 `pjsk/api/<api_path>/<hash>.<ext>` 同一前缀但路径不冲突。任何只针对 Drawing 产物的工具必须使用 `pjsk/api/` 前缀，不能用 `pjsk/`。

### 3.4 C10 观察（`user_data_file_path`）

- deck 旧协议分支在只有文件路径、没有用户数据字节时会把 Cloud 本地路径发给 deck-service。两端各记录一条 ERROR：生产端 `deck user_data_file_path fallback resolved`，消费端 `deck user_data_file_path fallback used`，只带 `region`、`path_base`（文件名）与累计 `count`。
- 观察一个完整周期零次出现后，删除该分支、`RecommendRequest.UserDataFilePath` 与 `resolveUserDataFilePath`。

## 4. 切换前检查（运维，一次性）

- R4 之前与 R6 之前各运行一次 Garage 集成测试：`HARUKI_RUN_INTEGRATION=1 go test ./internal/storage/s3/... -run Garage`，它是唯一能在真实服务端验证 SigV4 的测试。
- 确认 CN08 的 mihomo `NO_PROXY` 覆盖 tailnet 网段（Cloud 自己的 s3 客户端 `Proxy = nil`，其他访问 Garage 的进程仍需要）。
- 确认每个 Drawing/Garage 节点的公开 `image-cache` 与 `assets` 域名已在节点 Caddy 上线（只允许 GET/HEAD、禁止列目录），并已写入 `image_cache.hosts` / `asset_dirs.assets_base_urls`。
