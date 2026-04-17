package api

// ================= Response Structs =================

type HarukiAPIResponse struct {
	Status  int    `json:"status" msgpack:"status"`
	Message string `json:"message" msgpack:"message"`
}

type HarukiAPIDataResponse[T any] struct {
	HarukiAPIResponse
	Data T `json:"data,omitempty" msgpack:"data,omitempty"`
}

// ================= User Info =================

type UserInfo struct {
	HarukiUserID int    `json:"haruki_user_id"`
	Platform     string `json:"platform"`
	UserID       string `json:"user_id"`
	BanState     bool   `json:"ban_state"`
	BanReason    string `json:"ban_reason,omitempty"`
}

// ================= Context Keys =================

const UserContextKey = "haruki_user"

// ================= Length Constants =================

const (
	MaxAliasLength    = 100
	MaxUserIDLength   = 50
	MaxServerLength   = 20
	MaxReasonLength   = 255
	MaxOptionLength   = 50
	MaxValueLength    = 50
	MaxPlatformLength = 20
)

// ================= Response Messages =================

const (
	ResponseOK = "ok"
)

// ================= Content Types =================

const (
	ContentTypeJSON    = "application/json"
	ContentTypeMsgPack = "application/msgpack"
)

// ================= Auth =================

const (
	AuthBearerPrefix = "Bearer "
)

// ================= Error Messages =================

const (
	ErrInvalidRequest      = "Invalid request"
	ErrInvalidUserID       = "Invalid user_id"
	ErrInvalidHarukiUserID = "Invalid haruki_user_id"
	ErrUserNotFound        = "User not found"
	ErrAliasNotFound       = "Alias not found"
	ErrBindingNotFound     = "Binding not found"
	ErrPreferenceNotFound  = "Preference not found"
	ErrPermissionDenied    = "Permission denied"
	ErrAlreadyExists       = "Already exists"
	ErrInternalServer      = "Internal server error"
	ErrUserBanned          = "user is banned"
	ErrMissingPlatformInfo = "platform and platform_user_id are required"
)

// ================= Cache Keys =================

const (
	UserCacheKeyPrefix = "user:info:"
	UserCacheTTL       = 5 * 60
)

// ================= Redis Key Patterns =================

const (
	// RedisKeyBotSession is the Redis key pattern for bot session tokens.
	// Used by both the session middleware (api/) and the auth package (api/bot/auth/).
	RedisKeyBotSession = "hdb:bot:session:%s" // bot_id
)
