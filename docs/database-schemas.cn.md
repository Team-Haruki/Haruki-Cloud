# Haruki-Cloud 数据库 Schema 文档

> 最后更新：2026-03-23（v1.1）

---

## 1. 概览

项目使用 **[Ent ORM](https://entgo.io/)** 管理所有数据库 Schema。Schema 定义（Go 结构体）位于 `ent/<module>/schema/`，通过 `go generate` 自动将其编译为完整的 CRUD 客户端代码，输出至 `database/<module>/`。

### 数据库模块一览

| 模块 | Schema 目录 | 生成目录 | 数据库 | 表数量 |
|------|-------------|----------|--------|--------|
| **bot** | `ent/bot/schema/` | `database/bot/` | MySQL | 5 |
| **censor** | `ent/censor/schema/` | `database/censor/` | MySQL | 3 |
| **chunithm/maindb** | `ent/chunithm/maindb/schema/` | `database/chunithm/maindb/` | MySQL/PostgreSQL | 3 |
| **chunithm/music** | `ent/chunithm/music/schema/` | `database/chunithm/music/` | MySQL/PostgreSQL | 3 |
| **pjsk** | `ent/pjsk/schema/` | `database/pjsk/` | PostgreSQL | 8 |
| **sekai** | `ent/sekai/schema/` | `database/sekai/` | PostgreSQL | 83 |
| **users** | `ent/users/schema/` | `database/users/` | PostgreSQL | 1 |

---

## 2. 代码生成工作流

每个模块的目录结构如下：

```
ent/<module>/
  schema/          # Schema 定义（手工维护）
    *.go
  cmd/entc.go      # 代码生成入口（//go:build ignore）
  generate.go      # //go:generate go run -mod=mod ./cmd/entc.go
```

生成命令：

```bash
# 重新生成单个模块
go generate ./ent/bot/...
go generate ./ent/pjsk/...
go generate ./ent/sekai/...
# 以此类推
```

`cmd/entc.go` 指定输出包路径和目标目录：

```go
entc.Generate("./schema", &gen.Config{
    Package: "haruki-cloud/database/<module>",
    Target:  "../../database/<module>",
})
```

生成产物包括：完整 CRUD 方法、条件查询构建器、类型安全的 Edge 遍历、迁移 SQL、断言工具（`IsNotFound`）等。

---

## 3. Bot 数据库（`database/bot/`）

**用途**：Bot 账号管理 + 请求统计

### 3.1 `user` 表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | auto | PK | 自动主键 |
| `owner_user_id` | int64 | UNIQUE index | 所属用户 ID（绑定到 users 表） |
| `bot_id` | int | UNIQUE | Bot 数字 ID，业务主键 |
| `credential` | string(512) | optional | Bot 凭据（AES 加密后存储） |

### 3.2 `daily_requests` 表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | auto | PK | 自动主键 |
| `date_key` | date | UNIQUE, immutable | 日期（MySQL DATE 类型） |
| `count` | int | default 0 | 当日请求总计 |

### 3.3 `hourly_requests` 表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | auto | PK | 自动主键 |
| `hour_key` | datetime | UNIQUE, immutable | 小时键（MySQL DATETIME 类型） |
| `count` | int | default 0 | 当小时请求总计 |

### 3.4 `requests_ranking` 表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | auto | PK | 自动主键 |
| `bot_id` | int | UNIQUE | Bot 数字 ID |
| `counts` | int64 | optional | 累计请求总量 |

### 3.5 `command_manifests` 表

Bot 客户端启动时下载的指令路由表，每行对应一个 API 端点。管理员可在数据库中调整 `command_priority` 改变前缀匹配顺序。首次启动时由 `SeedCommandManifests` 自动填充 41 条默认记录。

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | auto | PK | 自动主键 |
| `command_prefixes` | JSON `[]string` | NOT NULL | 触发前缀列表，如 `["/查卡","/card"]` |
| `command_priority` | int | default 0 | 匹配优先级，越大越优先 |
| `command_mode` | string(16) | NOT NULL | 请求方法，如 `"GET,POST"` |
| `command_module` | string(64) | NOT NULL | 功能模块，如 `"pjsk"` |
| `command_path` | string(256) | NOT NULL | 路径（无前导斜杠），如 `"card/detail"` |
| `command_additional_params` | JSON `[]string` | optional | 端点额外接受的参数名列表 |

唯一索引：`(command_module, command_path)`

---

## 4. Censor 数据库（`database/censor/`）

**用途**：内容审核记录（**API 层已删除，DB 层保留**）

### 4.1 `name_log` 表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | int | PK, immutable | 主键 |
| `user_id` | string(30) | optional | 平台用户 ID |
| `name` | string(300) | optional | 被审核的昵称 |
| `haruki_user_id` | int | optional | Haruki 用户 ID（index） |
| `time` | timestamp | optional | 审核时间 |
| `result` | string(10) | optional | 审核结果码 |

索引：`haruki_user_id`

### 4.2 `censor_result` 表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | int | PK, immutable | 主键 |
| `name` | string(300) | NOT NULL | 被审核内容 |
| `result` | int | optional | 审核结果整数码 |
| `time` | timestamp | optional | 审核时间 |

### 4.3 `short_bio` 表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | int | PK, immutable | 主键 |
| `user_id` | string(30) | optional | 平台用户 ID |
| `content` | string(60) | optional | 短简介内容 |
| `haruki_user_id` | int | optional | Haruki 用户 ID（index） |
| `result` | string(10) | optional | 审核结果码 |

索引：`haruki_user_id`

---

## 5. CHUNITHM 数据库（分两库）

CHUNITHM 数据分为两个独立数据库：**maindb**（用户数据）和 **music**（曲目数据）。

### 5.1 maindb（`database/chunithm/maindb/`）

**用途**：用户账号绑定 + 曲目别名

#### `chunithm_bindings` 表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | auto | PK | 自动主键 |
| `haruki_user_id` | int | — | 用户 ID（关联 users 表） |
| `server` | string(10) | — | 服务器区域（如 jp） |
| `aime_id` | string(50) | — | 用户 Aime ID |

唯一索引：`(haruki_user_id, server)`

#### `chunithm_default_servers` 表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | auto | PK | 自动主键 |
| `haruki_user_id` | int | UNIQUE | 用户 ID（关联 users 表） |
| `server` | string(10) | — | 默认服务器区域 |

#### `chunithm_music_aliases` 表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | int64 | PK, immutable | 主键 |
| `music_id` | int | — | 曲目 ID |
| `alias` | string(100) | — | 别名字符串 |

唯一索引：`(music_id, alias)`

### 5.2 music（`database/chunithm/music/`）

**用途**：曲目信息 + 难度数据 + 谱面数据

#### `music` 表（ChunithmMusic）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | auto | PK | 自动主键 |
| `music_id` | int | UNIQUE, immutable | 曲目 ID（业务主键） |
| `title` | string(255) | — | 曲名 |
| `artist` | string(255) | — | 艺术家 |
| `category` | string(50) | optional | 分类 |
| `version` | string(10) | optional | 首次收录版本 |
| `release_date` | time | optional | 发布日期（用于过滤未发布曲目） |
| `is_deleted` | bool | default false | 是否已删除 |
| `deleted_version` | string(10) | optional | 删除时的版本 |

#### `music_difficulties` 表（ChunithmMusicDifficulty）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | auto | PK | 自动主键 |
| `music_id` | int | — | 曲目 ID |
| `version` | string(10) | — | 版本号（同一曲多版本） |
| `diff0_const` | float64 | optional | BASIC 定数 |
| `diff1_const` | float64 | optional | ADVANCED 定数 |
| `diff2_const` | float64 | optional | EXPERT 定数 |
| `diff3_const` | float64 | optional | MASTER 定数 |
| `diff4_const` | float64 | optional | ULTIMA 定数 |

唯一索引：`(music_id, version)`

#### `chart_data` 表（ChunithmChartData）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | auto | PK | 自动主键 |
| `music_id` | int | — | 曲目 ID |
| `difficulty` | int | — | 难度等级（0=BASIC … 4=ULTIMA） |
| `creator` | string(100) | optional | 谱面制作者 |
| `bpm` | float64 | optional | BPM |
| `tap_count` | int | optional | TAP 数量 |
| `hold_count` | int | optional | HOLD 数量 |
| `slide_count` | int | optional | SLIDE 数量 |
| `air_count` | int | optional | AIR 数量 |
| `flick_count` | int | optional | FLICK 数量 |
| `total_count` | int | optional | 总 note 数 |

唯一索引：`(music_id, difficulty)`

---

## 6. PJSK 数据库（`database/pjsk/`）

**用途**：Project SEKAI 别名系统 + 用户绑定 + 偏好设置

### 6.1 `alias` 表（全局别名）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | int64 | PK | 主键 |
| `alias_type` | string(20) | — | 别名类型（music / character / card 等） |
| `alias_type_id` | int | — | 游戏内 ID |
| `alias` | string(100) | — | 别名字符串 |

唯一索引：`(alias_type, alias_type_id, alias)`

### 6.2 `group_aliases` 表（群组私有别名）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | auto | PK | 自动主键 |
| `platform` | string(20) | — | 平台（如 qq） |
| `group_id` | string(50) | — | 群组 ID |
| `alias_type` | string(20) | — | 别名类型 |
| `alias_type_id` | int | — | 游戏内 ID |
| `alias` | string(100) | — | 别名字符串 |

唯一索引：`(platform, group_id, alias_type, alias_type_id, alias)`

### 6.3 `user_bindings` 表（PJSK 账号绑定）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | int | PK | 主键 |
| `haruki_user_id` | int | — | Haruki 用户 ID（关联 users 表） |
| `user_id` | string(30) | — | PJSK 游戏内 UID |
| `server` | string(2) | — | 服务器区域（jp/cn/tw/en/kr） |
| `visible` | bool | default true | 是否公开展示 |

唯一索引：`(haruki_user_id, server, user_id)`  
Edge：`→ user_default_bindings`（一对多）

### 6.4 `user_default_bindings` 表（默认绑定指针）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | int | PK | 主键 |
| `haruki_user_id` | int | — | Haruki 用户 ID |
| `server` | string(7) | — | 服务器（jp/cn/tw/en/kr/default） |
| `binding_id` | int | FK, CASCADE | 指向 `user_bindings.id` |

唯一索引：`(haruki_user_id, server)`  
Edge：`← user_bindings`（多对一，CASCADE 删除）

### 6.5 `user_preferences` 表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | auto | PK | 自动主键 |
| `haruki_user_id` | int | — | Haruki 用户 ID |
| `option` | string(50) | — | 偏好键（如 theme / difficulty） |
| `value` | string(50) | — | 偏好值 |

唯一索引：`(haruki_user_id, option)`

### 6.6 `alias_admins` 表（别名管理员）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | auto | PK | 自动主键 |
| `haruki_user_id` | int | UNIQUE | Haruki 用户 ID（一人一条） |
| `name` | string(100) | — | 管理员昵称 |

### 6.7 `pending_aliases` 表（待审核别名）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | int64 | PK | 主键 |
| `alias_type` | string(20) | — | 别名类型 |
| `alias_type_id` | int | — | 游戏内 ID |
| `alias` | string(100) | — | 别名字符串 |
| `submitted_by` | string(100) | — | 提交者标识 |
| `submitted_at` | time | — | 提交时间 |

唯一索引：`(alias_type, alias_type_id, alias)`

### 6.8 `rejected_aliases` 表（已拒绝别名）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | int64 | PK | 主键 |
| `alias_type` | string(20) | — | 别名类型 |
| `alias_type_id` | int | — | 游戏内 ID |
| `alias` | string(100) | — | 别名字符串 |
| `reviewed_by` | string(100) | — | 审核者标识 |
| `reason` | string(255) | — | 拒绝原因 |
| `reviewed_at` | time | — | 审核时间 |

---

## 7. Users 数据库（`database/users/`）

**用途**：跨游戏通用用户系统，统一管理平台账号 → Haruki ID 映射及封禁状态

### `users` 表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | int | PK | 6 位随机数，业务用户 ID |
| `platform` | string(20) | — | 平台名称（如 qq、discord） |
| `user_id` | string(50) | — | 平台上的用户 ID |
| `ban_state` | bool | default false | 全局封禁 |
| `ban_reason` | string(255) | optional | 全局封禁原因 |
| `pjsk_ban_state` | bool | default false | PJSK 功能封禁 |
| `pjsk_ban_reason` | string(255) | optional | — |
| `chunithm_ban_state` | bool | default false | CHUNITHM 功能封禁 |
| `chunithm_ban_reason` | string(255) | optional | — |
| `pjsk_main_ban_state` | bool | default false | PJSK Main 功能封禁 |
| `pjsk_main_ban_reason` | string(255) | optional | — |
| `pjsk_ranking_ban_state` | bool | default false | PJSK 排名功能封禁 |
| `pjsk_ranking_ban_reason` | string(255) | optional | — |
| `pjsk_alias_ban_state` | bool | default false | PJSK 别名功能封禁 |
| `pjsk_alias_ban_reason` | string(255) | optional | — |
| `pjsk_mysekai_ban_state` | bool | default false | PJSK MySekai 功能封禁 |
| `pjsk_mysekai_ban_reason` | string(255) | optional | — |
| `chunithm_main_ban_state` | bool | default false | CHUNITHM Main 功能封禁 |
| `chunithm_main_ban_reason` | string(255) | optional | — |
| `chunithm_alias_ban_state` | bool | default false | CHUNITHM 别名功能封禁 |
| `chunithm_alias_ban_reason` | string(255) | optional | — |

唯一索引：`(platform, user_id)`

> 细粒度封禁设计：全局封禁 + 功能维度封禁各自独立，系统可精确控制单个用户对每个游戏功能模块的访问权限。

---

## 8. Sekai Masterdata 数据库（`database/sekai/`）

**用途**：Project SEKAI 游戏数据镜像。存储 5 个区服（JP/CN/TW/EN/KR）的全量 Masterdata。

### 8.1 通用设计模式

**所有 83 张表均遵循相同模式**：

```
server_region  string     必填，区服标识（jp/cn/tw/en/kr）
game_id        int64      optional，游戏内 ID
...            其他字段   均为 optional
```

- 唯一索引：**`(game_id, server_region)`**（跨区服存储）
- 几乎所有字段均为 `Optional()`，字段值直接来自游戏 Masterdata JSON
- JSON 类型字段用于存储内嵌数组/对象（如 `gacha_details[]`、`skill_effects[]`）

**运行时查询示例**：

```go
// 查询日服所有卡片
cards, _ := client.Card.Query().
    Where(card.ServerRegionEQ("jp")).
    All(ctx)

// 按 game_id + 区服查单张卡
c, _ := client.Card.Query().
    Where(card.GameIDEQ(100), card.ServerRegionEQ("jp")).
    First(ctx)
```

### 8.2 核心游戏实体（8 张）

| Schema 类型 | 表名（推测） | 主要字段 | 说明 |
|------------|-------------|---------|------|
| `Card` | `cards` | `character_id`, `card_rarity_type`, `attr`, `skill_id`, `release_at`, `special_training_costs[]` | 卡片信息（含特训、技能、资产包） |
| `Event` | `events` | `event_type`, `name`, `start_at`, `aggregate_at`, `unit`, `event_ranking_reward_ranges[]` | 活动 |
| `Music` | `music` | `title`, `composer`, `lyricist`, `published_at`, `categories[]`, `infos[]` | 歌曲 |
| `Gacha` | `gachas` | `gacha_type`, `name`, `start_at`, `end_at`, `gacha_details[]`, `gacha_pickups[]` | 卡池 |
| `Skill` | `skills` | `short_description`, `description`, `skill_effects[]` | 技能 |
| `Stamp` | `stamps` | `stamp_type`, `name`, `character_id1`, `game_character_unit_id` | 贴纸 |
| `Honor` | `honors` | `honor_rarity`, `group_id`, `name`, `levels[]`, `honor_mission_type` | 称号 |
| `Gamecharacter` | `gamecharacters` | `first_name`, `given_name`, `unit`, `gender`, `height`, `figure` | 游戏角色（含体型/声部信息） |

### 8.3 卡片系统（5 张）

| Schema 类型 | 主要字段 | 说明 |
|------------|---------|------|
| `Cardraritie` | `card_rarity_type`, `seq`, `training_max_power*` | 卡片稀有度定义 |
| `Cardepisode` | `card_id`, `title`, `assetbundle_name`, `reward_resource_box_ids[]` | 卡片剧情 |
| `Cardcostume3d` | `card_id`, `costume_3d_id` | 卡片 3D 服装 |
| `Cardsupplie` | `card_supply_type`, `seq` | 卡片获取途径（供给类型） |
| `Cardmysekaicanvasbonuse` | `mysekai_canvas_bonus_type`, `bonus_rate` | 卡片烤森画布加成 |

### 8.4 角色系统（5 张）

| Schema 类型 | 主要字段 | 说明 |
|------------|---------|------|
| `Gamecharacterunit` | `game_character_id`, `unit`, `colorcode` | 角色所属组合（VS SEKAI 中可跨组合） |
| `Character2d` | `character_type`, `character_id`, `unit` | 2D 资源关联 |
| `Outsidecharacter` | `name` | 虚拟歌手（MIKU/MEIKO 等） |
| `Characterrank` | `character_id`, `character_rank`, `power_up_bonuses[]` | 角色等级（粉丝等级）加成 |
| `Charactermissionv2parametergroup` | `character_id`, `parameters[]` | 角色任务 V2 参数组 |

### 8.5 音乐系统（5 张）

| Schema 类型 | 主要字段 | 说明 |
|------------|---------|------|
| `Musicdifficultie` | `music_id`, `music_difficulty`, `play_level`, `note_count` | 难度信息 |
| `Musicvocal` | `music_id`, `music_vocal_type`, `character_ids[]`, `assetbundle_name` | 歌曲演唱版本（原创/翻唱） |
| `Musicartist` | `name` | 艺术家 |
| `Musictag` | `music_id`, `music_tag` | 歌曲标签 |
| `Limitedtimemusic` | `music_id`, `music_limited_type`, `start_at`, `end_at` | 限时歌曲 |

### 8.6 活动系统（9 张）

| Schema 类型 | 主要字段 | 说明 |
|------------|---------|------|
| `Eventcard` | `event_id`, `card_id`, `bonus_rate` | 活动加成卡 |
| `Eventitem` | `event_id`, `event_item_type`, `name` | 活动道具 |
| `Eventmusic` | `event_id`, `music_id`, `seq` | 活动歌曲 |
| `Eventdeckbonuse` | `event_id`, `game_character_unit_id`, `bonus_rate` | 活动组合加成 |
| `Eventstorie` | `event_id`, `title`, `banner_game_character_unit_id` | 活动故事 |
| `Eventstoryunit` | `event_story_id`, `seq`, `story_type` | 活动故事章节 |
| `Eventexchangesummarie` | `event_exchange_id`, `resource_box_id` | 活动兑换汇总 |
| `Eventraritybonusrate` | `card_rarity_type`, `master_rank`, `bonus_rate` | 活动稀有度加成率 |
| `Cheerfulcarnivalteam` | `event_id`, `seq`, `team_name`, `assetbundle_name` | 欢乐嘉年华队伍 |

### 8.7 卡池系统（2 张）

| Schema 类型 | 主要字段 | 说明 |
|------------|---------|------|
| `Gachaceilitem` | `gacha_ceil_item_type`, `name`, `assetbundle_name` | 天井道具 |
| `Gachaticket` | `name`, `assetbundle_name` | 抽卡票 |

### 8.8 WorldBloom 系统（4 张）

| Schema 类型 | 主要字段 | 说明 |
|------------|---------|------|
| `Worldbloom` | `event_id`, `game_character_id`, `chapter_no` | WL 章节 |
| `Worldbloomsupportdeckbonuse` | `unit`, `support_deck_bonus_type`, `bonus_rate` | WL 支援组合加成 |
| `Worldbloomsupportdeckuniteventlimitedbonuse` | `unit`, `bonus_rate` | WL 活动限定组合加成 |
| `Worldbloomdifferentattributebonuse` | `attr`, `bonus_rate` | WL 异属性加成 |

### 8.9 MySekai 系统（15 张）

| Schema 类型 | 主要字段 | 说明 |
|------------|---------|------|
| `Mysekaifixture` | `name`, `grid_size{}`, `mysekai_fixture_type{}`, `first_put_cost`, `is_assembled` | 家具（含尺寸/类型/颜色信息） |
| `Mysekaifixturemaingenre` | `name`, `seq` | 家具主分类 |
| `Mysekaifixturesubgenre` | `name`, `seq`, `mysekai_fixture_main_genre_id` | 家具子分类 |
| `Mysekaifixturetag` | `name` | 家具标签 |
| `Mysekaifixturegamecharactergroup` | `game_character_ids[]` | 家具角色组 |
| `Mysekaifixturegamecharactergroupperformancebonuse` | `mysekai_fixture_game_character_group_id`, `bonus_rate` | 家具角色组演出加成 |
| `Mysekaifixtureonlydisassemblematerial` | `mysekai_fixture_id`, `mysekai_material_id` | 家具拆解材料 |
| `Mysekaiblueprint` | `name`, `mysekai_fixture_id`, `mysekai_blueprint_type` | 家具蓝图 |
| `Mysekaiblueprintmysekaimaterialcost` | `mysekai_blueprint_id`, `mysekai_material_id`, `quantity` | 蓝图材料费用 |
| `Mysekaimaterial` | `name`, `mysekai_material_type` | 材料 |
| `Mysekaimaterialgamecharacterrelation` | `mysekai_material_id`, `game_character_id` | 材料与角色关联 |
| `Mysekaiitem` | `name`, `mysekai_item_type` | MySekai 道具 |
| `Mysekaimusicrecord` | `music_id`, `mysekai_music_record_category_id` | 音乐唱片记录 |
| `Mysekaimusicrecordcategorie` | `name` | 音乐唱片分类 |
| `Mysekaisiteharvestfixture` | `mysekai_fixture_id`, `harvest_power` | 场地采集家具 |

### 8.10 MySekai 大门系统（4 张）

| Schema 类型 | 主要字段 | 说明 |
|------------|---------|------|
| `Mysekaigate` | `name`, `mysekai_gate_type`, `character_id` | 大门 |
| `Mysekaigatelevel` | `mysekai_gate_id`, `level`, `open_condition_type` | 大门升级等级 |
| `Mysekaigatematerialgroup` | `mysekai_gate_level_id`, `mysekai_material_id`, `quantity` | 大门升级材料组 |
| `Mysekaigatecharacterlotterie` | `mysekai_gate_id`, `game_character_id`, `weight` | 大门角色抽奖权重 |

### 8.11 MySekai 角色对话系统（6 张）

| Schema 类型 | 主要字段 | 说明 |
|------------|---------|------|
| `Mysekaicharactertalk` | `character_archive_display_type`, `scenario_id`, `talk_text` | 角色对话 |
| `Mysekaicharactertalkfixturecommon` | `mysekai_character_talk_id`, `fixture_conditions[]` | 对话家具触发条件 |
| `Mysekaicharactertalkfixturecommonmysekaifixturegroup` | `group`, `mysekai_fixture_ids[]` | 触发家具组 |
| `Mysekaicharactertalkcondition` | `mysekai_character_talk_id`, `condition_type`, `condition_value` | 对话触发条件 |
| `Mysekaicharactertalkconditiongroup` | `group_id`, `condition_ids[]` | 触发条件组 |
| `Characterarchivemysekaicharactertalkgroup` | `game_character_id`, `talk_group_ids[]` | 角色对话组存档 |

### 8.12 MySekai 现象系统（2 张）

| Schema 类型 | 主要字段 | 说明 |
|------------|---------|------|
| `Mysekaiphenomenon` | `name`, `phenomenon_type`, `mysekai_gate_id` | 现象事件 |
| `Mysekaiphenomenabackgroundcolor` | `phenomenon_id`, `color_code` | 现象背景色 |

### 8.13 MySekai 游戏角色单元组（1 张）

| Schema 类型 | 主要字段 | 说明 |
|------------|---------|------|
| `Mysekaigamecharacterunitgroup` | `game_character_unit_ids[]` | 游戏角色单元分组 |

### 8.14 成长/奖励系统（5 张）

| Schema 类型 | 主要字段 | 说明 |
|------------|---------|------|
| `Level` | `level_type`, `level`, `exp`, `limit_exp` | 等级经验表 |
| `Masterlesson` | `card_rarity_type`, `master_rank`, `costs[]` | 卡片 Master 等级升级费用 |
| `Bond` | `game_character_id`, `bondsHonorWordId`, `level` | 角色羁绊等级 |
| `Bondshonor` | `bond_honor_type`, `rank`, `name`, `levels[]` | 羁绊称号 |
| `Challengelivehighscorereward` | `character_id`, `high_score`, `resource_box_id` | 挑战 Live 高分奖励 |

### 8.15 其他系统（12 张）

| Schema 类型 | 主要字段 | 说明 |
|------------|---------|------|
| `Area` | `area_type`, `name`, `view_type` | 区域（主页区域） |
| `Areaitem` | `area_id`, `area_item_type`, `name` | 区域道具 |
| `Areaitemlevel` | `area_item_id`, `area_item_level`, `costs[]` | 区域道具升级等级 |
| `Virtuallive` | `virtual_live_type`, `name`, `start_at`, `end_at` | 虚拟 LIVE 活动 |
| `Shopitem` | `shop_item_type`, `seq`, `cost_resource_type`, `cost_resource_quantity` | 商店道具 |
| `Boostitem` | `name`, `boost_item_type`, `recover_exp` | 体力恢复道具 |
| `Resourceboxe` | `resource_box_purpose`, `resource_box_type`, `details[]` | 资源礼盒 |
| `Playerframe` | `name`, `seq`, `playerframe_type`, `assetbundle_name` | 玩家边框 |
| `Playerframegroup` | `name`, `seq` | 边框组 |
| `Honorgroup` | `name`, `honor_type`, `bg_asset_bundle_name` | 称号组 |
| `Ngword` | `word` | NG 词表 |
| `Costume3d` | `costume_3d_type`, `name`, `asset_bundle_name`, `character_id` | 3D 服装 |

---

## 9. PJSK 别名类型说明

PJSK 数据库中的 `alias_type` 字段使用以下值（定义于 `utils/utils.go`）：

| 值 | 含义 |
|----|------|
| `music` | 歌曲别名 |
| `character` | 角色别名 |
| `card` | 卡片别名 |
| `event` | 活动别名 |
| `gacha` | 卡池别名 |

---

## 10. 数据库间关系

各数据库**物理隔离**（独立连接），无跨库外键约束，通过应用层代码关联：

```
users.id (haruki_user_id)
    ↓ 应用层关联
pjsk.user_bindings.haruki_user_id
pjsk.user_preferences.haruki_user_id
chunithm.maindb.chunithm_bindings.haruki_user_id
chunithm.maindb.chunithm_default_servers.haruki_user_id
```

同一个 `haruki_user_id` 可同时在 PJSK 和 CHUNITHM 数据库中有记录，`utils/query` 包封装了跨库查询逻辑。

---

## 11. 构建与迁移

```bash
# 重新生成某个模块的 Ent 代码
go generate ./ent/bot/...
go generate ./ent/pjsk/...
go generate ./ent/chunithm/maindb/...
go generate ./ent/chunithm/music/...
go generate ./ent/sekai/...
go generate ./ent/users/...

# 执行数据库迁移（在目标环境）
go run ./cmd/migrate/...
```

---

**维护者**：Haruki-Cloud Team  
**文档版本**：v1.0  
**创建日期**：2026-03-23
