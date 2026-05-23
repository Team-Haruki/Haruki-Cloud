# Neo Public Bot API

`/api/bot/v2/` 是面向外部调用方的公开查询入口。线上通过 Caddy 暴露在：

- `https://neo-api.haruki.seiunx.com/api/bot/v2`

两个域名提供相同的 API。该入口会反代到 Haruki-Cloud 内部的
`/api/v2/public/`，不需要 `Authorization`。

同一端口还提供 `/api/sekai/v6/*` 到 `haruki-sekai-api` 的反代；本文只说明
`/api/bot/v2/` 下的 Haruki-Cloud 公开查询接口。

## 可用性

公开查询接口由 Haruki-Cloud 的模块开关决定：

| 模块 | 开关 | 关闭时行为 |
|------|------|------------|
| PJSK | `HARUKI_PJSK_ENABLED` | `/pjsk/*` 路由不注册 |
| CHUNITHM | `HARUKI_CHUNITHM_ENABLED` | `/chunithm/*` 路由不注册，返回 `404 Not Found` |

生产环境当前启用了 PJSK，CHUNITHM 路由是否可用以运行中容器的
`HARUKI_CHUNITHM_ENABLED` 为准。下面列出代码支持的全部公开查询接口。

## 响应格式

常规响应是 JSON envelope：

```json
{
  "status": 200,
  "message": "ok",
  "data": {}
}
```

常见状态：

| HTTP 状态 | 含义 |
|-----------|------|
| `200` | 查询成功 |
| `400` | 参数不合法或请求体格式错误 |
| `404` | 查询对象不存在 |
| `500` | 服务端错误 |

## PJSK 别名查询

### 别名查对象 ID

```http
GET /api/bot/v2/pjsk/alias/{alias_type}/by-alias?alias={alias}
```

参数：

| 参数 | 位置 | 说明 |
|------|------|------|
| `alias_type` | path | 只能是 `music` 或 `character` |
| `alias` | query | 要查询的别名，不能为空，最多 100 个字符 |

示例：

```bash
curl 'https://neo-api.haruki.seiunx.com/api/bot/v2/pjsk/alias/music/by-alias?alias=sekai'
```

成功时：

```json
{
  "status": 200,
  "message": "ok",
  "data": {
    "match_ids": [1, 2]
  }
}
```

### 对象 ID 查别名列表

```http
GET /api/bot/v2/pjsk/alias/{alias_type}/{alias_type_id}
```

参数：

| 参数 | 位置 | 说明 |
|------|------|------|
| `alias_type` | path | 只能是 `music` 或 `character` |
| `alias_type_id` | path | 曲目或角色 ID，必须是非负整数 |

示例：

```bash
curl 'https://neo-api.haruki.seiunx.com/api/bot/v2/pjsk/alias/music/1'
```

成功时：

```json
{
  "status": 200,
  "message": "ok",
  "data": {
    "aliases": ["sekai", "セカイ"]
  }
}
```

## CHUNITHM 别名查询

### 别名查曲目 ID

```http
GET /api/bot/v2/chunithm/alias/music-id?alias={alias}
```

参数：

| 参数 | 位置 | 说明 |
|------|------|------|
| `alias` | query | 要查询的别名，不能为空，最多 100 个字符 |

成功时：

```json
{
  "status": 200,
  "message": "ok",
  "data": {
    "match_ids": [1001]
  }
}
```

### 曲目 ID 查别名列表

```http
GET /api/bot/v2/chunithm/alias/{music_id}
```

参数：

| 参数 | 位置 | 说明 |
|------|------|------|
| `music_id` | path | CHUNITHM 曲目 ID，必须大于 0 |

成功时：

```json
{
  "status": 200,
  "message": "ok",
  "data": {
    "aliases": ["example alias"]
  }
}
```

没有别名时 `aliases` 会返回空数组。

## CHUNITHM 曲目信息查询

### 全部曲目

```http
GET /api/bot/v2/chunithm/music/all-music
```

返回已发布或没有发布日期的曲目列表：

```json
{
  "status": 200,
  "message": "ok",
  "data": [
    {
      "music_id": 1001,
      "title": "Song Title",
      "artist": "Artist",
      "category": "POPS & ANIME",
      "version": "VERSE",
      "release_date": "2026-01-01T00:00:00Z",
      "is_deleted": false
    }
  ]
}
```

### 曲目基础信息

```http
GET /api/bot/v2/chunithm/music/{music_id}/basic-info
```

参数：

| 参数 | 位置 | 说明 |
|------|------|------|
| `music_id` | path | CHUNITHM 曲目 ID，必须大于 0 |

返回单个曲目的 `music_id`、`title`、`artist`、`category`、`version`、
`release_date`、`is_deleted`、`deleted_version`。

### 曲目定数信息

```http
GET /api/bot/v2/chunithm/music/{music_id}/difficulty-info?version={version}
```

参数：

| 参数 | 位置 | 说明 |
|------|------|------|
| `music_id` | path | CHUNITHM 曲目 ID，必须大于 0 |
| `version` | query | 版本号，不能为空 |

返回字段：

| 字段 | 说明 |
|------|------|
| `music_id` | 曲目 ID |
| `version` | 实际返回的版本 |
| `diff0_const` 到 `diff4_const` | BASIC、ADVANCED、EXPERT、MASTER、ULTIMA 的定数；缺失时省略 |

如果指定版本没有记录，服务会尝试返回该曲目的最新定数记录；若没有任何定数数据，
返回 `404`。

### 谱面数据

```http
GET /api/bot/v2/chunithm/music/{music_id}/chart-data
```

参数：

| 参数 | 位置 | 说明 |
|------|------|------|
| `music_id` | path | CHUNITHM 曲目 ID，必须大于 0 |

返回每个难度的谱面统计：

```json
{
  "status": 200,
  "message": "ok",
  "data": [
    {
      "difficulty": 3,
      "creator": "譜面作者",
      "bpm": 180,
      "tap_count": 100,
      "hold_count": 20,
      "slide_count": 30,
      "air_count": 40,
      "flick_count": 10,
      "total_count": 200
    }
  ]
}
```

### 批量查询

```http
POST /api/bot/v2/chunithm/music/query-batch
Content-Type: application/json

{
  "music_ids": [1001, 1002],
  "version": "VERSE"
}
```

兼容路径：

```http
POST /api/bot/v2/chunithm/query-batch
```

请求体：

| 字段 | 说明 |
|------|------|
| `music_ids` | 曲目 ID 数组 |
| `version` | 可选；指定时优先返回该版本定数，否则返回最新定数 |

成功时 `message` 为 `success`，`data` 是以曲目 ID 为 key 的对象：

```json
{
  "status": 200,
  "message": "success",
  "data": {
    "1001": {
      "version": "VERSE",
      "difficulty": [1.0, 5.0, 9.0, 13.0, null],
      "info": {
        "music_id": 1001,
        "title": "Song Title",
        "artist": "Artist",
        "category": "POPS & ANIME",
        "version": "VERSE",
        "is_deleted": false
      }
    }
  }
}
```
