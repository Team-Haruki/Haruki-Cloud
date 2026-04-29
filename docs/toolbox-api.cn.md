# Haruki Toolbox API 客户端文档

> 最后更新：2026-03-25
>
> 本文档描述 `utils/sekai.HarukiToolboxClient` 的全部能力、API 路由约定、参数语义、
> 返回类型与错误处理规范。Toolbox 是 Haruki-Cloud 访问私有用户游戏数据的唯一外部入口。

---

## 1. 概览

`HarukiToolboxClient`（`utils/sekai/client_toolbox.go`）封装了所有对 **Haruki Toolbox** 服务
的 HTTP 调用。Toolbox 是一个独立后端服务，负责：

1. 持有用户私有游戏数据快照（suite、mysekai 等）
2. 对访问者进行平台身份鉴权（`platform + platform_user_id`）
3. 维护游戏账号绑定关系（供快速验证路径使用）

客户端实例通过 `GetToolboxClient()` 获取（单例，线程安全）。

---

## 2. 配置

对应配置块（YAML 路径：`toolbox`）：

```yaml
toolbox:
  base_url: "https://toolbox.haruki.example.com"
  api_token: "Bearer xxxxxxxx"
  user_agent: "haruki-cloud/1.0"
```

对应 Go 类型：`config.ToolboxConfig{BaseURL, APIToken, UserAgent}`

请求默认启用：

- 重试：最大 `maxRetries` 次，等待 `retryWaitTime`
- 重试触发条件：网络级错误（`connection refused`、`no such host`、`EOF`、`i/o timeout`）或 HTTP 5xx

---

## 3. 通用鉴权规则

所有 Toolbox API 请求均携带：

| Header | 值 |
|--------|----|
| `Authorization` | `config.APIToken`（含 `Bearer ` 前缀） |
| `User-Agent` | `config.UserAgent` |

`platform` 和 `platform_user_id` 作为 Query 参数传递，由 Toolbox 在服务端验证调用方的 IM 平台身份。

---

## 4. 数据类型枚举

```go
// ToolboxDataType 标识快照类型
type ToolboxDataType string

const (
    ToolboxDataTypeSuite   ToolboxDataType = "suite"    // 用户游戏数据快照（等价于 user.json）
    ToolboxDataTypeMySekai ToolboxDataType = "mysekai"  // MySekai 世界快照（等价于 mysekai.json）
)
```

---

## 5. API 端点详细说明

### 5.1 GET /api/private/game-data/:server/:data_type/:user_id

**获取用户私有游戏数据快照（完整 JSON）**

#### 调用方式

```
GET /api/private/game-data/{server}/{data_type}/{user_id}
    ?platform={platform}
    &platform_user_id={platform_user_id}
    [&key={key}]
```

#### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| `server` | string | 区服标识，如 `jp`、`en`、`tw`、`kr`、`cn` |
| `data_type` | string | 快照类型，`suite` 或 `mysekai` |
| `user_id` | int64 | PJSK 游戏 UID |

#### Query 参数

| 参数 | 类型 | 必须 | 说明 |
|------|------|------|------|
| `platform` | string | ✅ | 调用方 IM 平台标识 |
| `platform_user_id` | string | ✅ | 调用方 IM 用户 ID |
| `key` | string | ❌ | 若指定，只返回该顶级字段的值（不含完整 JSON） |

#### 响应

- `200 OK`：原始 JSON 体（完整快照）；若服务端支持，响应头可能带 `Content-Encoding: zstd`，客户端自动解压。
- `200 OK`（指定 `key`）：该 key 的原始值（如整数 `1774339266`），不是 JSON 对象；若指定多个 key，则返回 JSON 对象。响应同样支持 `Content-Encoding: zstd` 自动解压。

#### Go 封装函数

```go
// 获取完整快照（自动 zstd 解压）
func (c *HarukiToolboxClient) GetPrivateData(
    server string,
    dataType ToolboxDataType,
    userID int64,
    platform, platformUserID string,
) ([]byte, error)

// 获取单个顶级 key 的值
func (c *HarukiToolboxClient) GetPrivateDataValue(
    server string,
    dataType ToolboxDataType,
    userID int64,
    platform, platformUserID string,
    key string,
) ([]byte, error)

// 获取多个顶级 key（逗号拼接发送，返回 JSON 对象）
func (c *HarukiToolboxClient) GetPrivateDataValues(
    server string,
    dataType ToolboxDataType,
    userID int64,
    platform, platformUserID string,
    keys ...string,
) ([]byte, error)

// 语法糖：获取 upload_time 字段（返回整数字节，如 "1774339266"）
func (c *HarukiToolboxClient) GetUploadTime(
    server string,
    dataType ToolboxDataType,
    userID int64,
    platform, platformUserID string,
) ([]byte, error)

// 语法糖：获取 suite 完整快照
func (c *HarukiToolboxClient) GetSuiteData(
    server string,
    userID int64,
    platform, platformUserID string,
) ([]byte, error)

// 语法糖：获取 mysekai 完整快照
func (c *HarukiToolboxClient) GetMySekaiData(
    server string,
    userID int64,
    platform, platformUserID string,
) ([]byte, error)
```

#### 错误处理

| HTTP 状态码 | 响应体 message 关键词 | 返回的 Go 错误 |
|------------|----------------------|---------------|
| `403` | `"invalid platform or platform_user_id"` | `ErrInvalidPlatformUser` |
| `403` | `"account owner is banned"` | `ErrAccountOwnerBanned` |
| `404` | `"account binding not found"` | `ErrAccountBindingNotFound` |
| `404` | `"game data not found"` | `ErrGameDataNotFound` |
| `503` | 任意 | `&ToolboxAPIError{StatusCode: 503}` |
| 其他 | 任意 | `&ToolboxAPIError{StatusCode: N, Message: ...}` |

---

### 5.2 GET /api/private/game-binding

**快速验证路径：通过平台身份获取所有关联游戏账号绑定**

#### 调用方式

```
GET /api/private/game-binding
    ?platform={platform}
    &platform_user_id={platform_user_id}
```

#### 行为说明

此端点实现的是"快速验证（fast verification）"路径：

1. 在 `authorize_social_platform_infos` 表中查找所有满足
   `platform = ? AND platform_user_id = ? AND allow_fast_verification = true` 的记录
2. 通过 eager load 获取每条记录关联用户的所有游戏账号绑定
3. 对结果去重，扁平化返回
4. 若没有任何匹配绑定（即该 platform/platform_user_id 在 fast-verification 记录中不存在），返回空数组 `[]`（**200，不返回 404**）
5. 若关联的 toolbox 账号**被封禁**，返回 **HTTP 403**（与 `/api/private/game-data` 的 ban 行为一致）——这与"没有绑定"是不同的语义，被封禁不会返回空数组

> **关键区别**：
> - 找不到绑定 → `200 []`（空列表，无错误）
> - 账号被封禁 → `403` → Go 侧 `ErrAccountOwnerBanned`

> **注意**：此端点依赖 `authorize_social_platform_infos` 表的 `allow_fast_verification` 列，
> 该列需要通过数据库迁移添加（Ent auto-migration 或手动 `ALTER TABLE`）。

#### Query 参数

| 参数 | 类型 | 必须 | 说明 |
|------|------|------|------|
| `platform` | string | ✅ | IM 平台标识 |
| `platform_user_id` | string | ✅ | IM 用户 ID |

#### 响应

```json
[
  {"server": "jp", "gameUserId": "1234567890"},
  {"server": "en", "gameUserId": "9876543210"}
]
```

空结果：

```json
[]
```

#### 响应类型

```go
// UserGameBinding 表示快速验证端点返回的单条游戏账号绑定
type UserGameBinding struct {
    Server     string `json:"server"`
    GameUserID string `json:"gameUserId"`
}
```

注意：此端点**不返回** `verified` 字段（与早期设计不同）。

#### Go 封装函数

```go
// GetToolboxUserFastVerificationGameAccountBindings 通过平台身份获取
// 所有开启了快速验证（allow_fast_verification=true）的关联游戏账号绑定。
//
// 返回空切片（而非错误）表示该平台用户没有任何关联绑定。
// 若关联的 toolbox 账号被封禁，返回 ErrAccountOwnerBanned（HTTP 403），
// 而非空切片——被封禁与无绑定是不同的语义。
func (c *HarukiToolboxClient) GetToolboxUserFastVerificationGameAccountBindings(
    platform, platformUserID string,
) ([]UserGameBinding, error)
```

#### 错误处理

| HTTP 状态码 | 响应体 message 关键词 | 返回的 Go 错误 |
|------------|----------------------|---------------|
| `403` | `"invalid platform or platform_user_id"` | `ErrInvalidPlatformUser` |
| `403` | `"account owner is banned"` | `ErrAccountOwnerBanned` |
| `404` | `"account binding not found"` | `ErrAccountBindingNotFound` |
| `503` | 任意 | `&ToolboxAPIError{StatusCode: 503}` |
| 其他 | 任意 | `&ToolboxAPIError{StatusCode: N, Message: ...}` |

> `200 + []`（空数组）表示该平台用户存在但无关联绑定，是正常结果，不属于错误情形。
> `403 ErrAccountOwnerBanned` 表示账号被封禁，与空列表是不同语义，调用方应区分处理。

---

## 6. 错误类型参考

所有 Toolbox 相关错误定义在 `utils/sekai/errors.go`：

### 哨兵错误（用 `errors.Is` 匹配）

```go
var (
    // 403 — platform/user 组合不存在或无权访问
    ErrInvalidPlatformUser = errors.New("forbidden: invalid platform or platform_user_id for this user")

    // 403 — 游戏账号所有者已被封禁
    ErrAccountOwnerBanned = errors.New("forbidden: account owner is banned")

    // 404 — 用户在 suite 服务上没有绑定游戏账号
    ErrAccountBindingNotFound = errors.New("account binding not found on suite service")

    // 404 — 用户从未上传过数据
    ErrGameDataNotFound = errors.New("game data not found: user has not uploaded data")
)
```

### 结构化错误（用 `errors.As` 匹配）

```go
type ToolboxAPIError struct {
    StatusCode int
    Message    string
}

func (e *ToolboxAPIError) Error() string {
    return fmt.Sprintf("toolbox api error: status %d, message: %q", e.StatusCode, e.Message)
}
```

---

## 7. 使用示例

### 获取用户套件快照（profile 渲染）

```go
data, err := toolbox.GetSuiteData("jp", 1234567890, "qq", "123456789")
if err != nil {
    switch {
    case errors.Is(err, sekai.ErrGameDataNotFound):
        // 用户还没有上传数据
    case errors.Is(err, sekai.ErrInvalidPlatformUser):
        // 没有权限访问
    case errors.Is(err, sekai.ErrAccountOwnerBanned):
        // 账号已被封禁
    default:
        // 其他错误
    }
}
// data 是原始 JSON bytes，直接传入渲染器
```

### 获取套件上传时间（抓包状态查询 /sud）

```go
raw, err := toolbox.GetUploadTime("jp", sekai.ToolboxDataTypeSuite, 1234567890, "qq", "123456789")
// raw = []byte("1774339266")
uploadTime, _ := strconv.ParseInt(string(raw), 10, 64)
```

### 获取平台用户关联的所有游戏账号（快速验证）

```go
bindings, err := toolbox.GetToolboxUserFastVerificationGameAccountBindings("qq", "123456789")
switch {
case err == nil:
    // bindings 可能为空切片（无关联绑定），也可能有内容
    // []UserGameBinding{{Server: "jp", GameUserID: "1234567890"}, ...}
case errors.Is(err, sekai.ErrAccountOwnerBanned):
    // 账号被封禁（HTTP 403）—— 注意：这与空列表是不同的语义，不可混淆
case errors.Is(err, sekai.ErrInvalidPlatformUser):
    // 平台身份无效或未授权
default:
    // 其他错误
}
```

---

## 8. 调用路径中的身份传递规范

访问私有数据时，`platform` 和 `platform_user_id` 必须是**发起请求的用户**的 IM 平台信息，
而**不是**被查询的目标用户。

| 场景 | platform | platform_user_id |
|------|----------|-----------------|
| 查自己的数据 | 自己的平台 | 自己的 ID |
| 查他人的数据 | **仍是自己的平台** | **仍是自己的 ID** |

Toolbox 服务端根据这个信息做授权校验（该调用方是否有权查看目标账号的数据）。
因此 Cloud 侧不应将目标用户的 IM 信息填入这两个参数。

---

## 9. 底层行为备注

- **zstd 解压**：`GetPrivateData`、`GetPrivateDataValue` 和 `GetPrivateDataValues` 都发送
  `Accept-Encoding: zstd`，并在响应头含 `Content-Encoding: zstd` 时自动解压。调用方收到的始终是解压后的原始 bytes。
- **`key` 参数**：单 key 通常很小，服务端可能不会压缩；多 key 返回 JSON 对象时也走同一套自动解压逻辑。
- **重试策略**：网络错误和 HTTP 5xx 均会触发重试，4xx 不重试。

---

## 10. 相关文档

- [PJSK 用户快照 Provider 设计](pjsk-user-snapshot-provider-design.cn.md)
- [PJSK 账号绑定实现说明](pjsk-profile-binding-implementation.cn.md)
- [项目进展总结](project-status-summary.cn.md)
