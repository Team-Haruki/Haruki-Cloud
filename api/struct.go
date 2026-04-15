package api

import "haruki-cloud/utils/types"

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
	MaxAliasLength    = types.MaxAliasLength
	MaxUserIDLength   = types.MaxUserIDLength
	MaxServerLength   = types.MaxServerLength
	MaxReasonLength   = types.MaxReasonLength
	MaxOptionLength   = types.MaxOptionLength
	MaxValueLength    = types.MaxValueLength
	MaxPlatformLength = types.MaxPlatformLength
)

// ================= Error Messages =================

const (
	ErrInvalidRequest      = types.ErrInvalidRequest
	ErrInvalidUserID       = types.ErrInvalidUserID
	ErrInvalidHarukiUserID = types.ErrInvalidHarukiUserID
	ErrUserNotFound        = types.ErrUserNotFound
	ErrAliasNotFound       = types.ErrAliasNotFound
	ErrBindingNotFound     = types.ErrBindingNotFound
	ErrPreferenceNotFound  = types.ErrPreferenceNotFound
	ErrPermissionDenied    = types.ErrPermissionDenied
	ErrAlreadyExists       = types.ErrAlreadyExists
	ErrInternalServer      = types.ErrInternalServer
	ErrUserBanned          = "user is banned"
	ErrMissingPlatformInfo = "platform and platform_user_id are required"
)

// ================= Cache Keys =================

const (
	UserCacheKeyPrefix = "user:info:"
	UserCacheTTL       = 5 * 60
)
