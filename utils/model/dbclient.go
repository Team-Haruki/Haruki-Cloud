package model

import "time"

// ================= Common =================

// ApiResponse represents a standard API response
type ApiResponse[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

// ================= Users =================

// UserResponse represents the user information response
type UserResponse struct {
	ID                     int    `json:"id"`
	Platform               string `json:"platform"`
	UserID                 string `json:"user_id"`
	BanState               bool   `json:"ban_state"`
	BanReason              string `json:"ban_reason"`
	PjskBanState           bool   `json:"pjsk_ban_state"`
	PjskBanReason          string `json:"pjsk_ban_reason"`
	ChunithmBanState       bool   `json:"chunithm_ban_state"`
	ChunithmBanReason      string `json:"chunithm_ban_reason"`
	PjskMainBanState       bool   `json:"pjsk_main_ban_state"`
	PjskMainBanReason      string `json:"pjsk_main_ban_reason"`
	PjskRankingBanState    bool   `json:"pjsk_ranking_ban_state"`
	PjskRankingBanReason   string `json:"pjsk_ranking_ban_reason"`
	PjskAliasBanState      bool   `json:"pjsk_alias_ban_state"`
	PjskAliasBanReason     string `json:"pjsk_alias_ban_reason"`
	PjskMysekaiBanState    bool   `json:"pjsk_mysekai_ban_state"`
	PjskMysekaiBanReason   string `json:"pjsk_mysekai_ban_reason"`
	ChunithmMainBanState   bool   `json:"chunithm_main_ban_state"`
	ChunithmMainBanReason  string `json:"chunithm_main_ban_reason"`
	ChunithmAliasBanState  bool   `json:"chunithm_alias_ban_state"`
	ChunithmAliasBanReason string `json:"chunithm_alias_ban_reason"`
}

// CreateUserRequest represents a request to create a user
type CreateUserRequest struct {
	Platform string `json:"platform"`
	UserID   string `json:"user_id"`
}

// UpdateBanRequest represents a request to update user ban status
type UpdateBanRequest struct {
	BanState  bool   `json:"ban_state"`
	BanReason string `json:"ban_reason"`
}

// UpdateFeatureBanRequest represents a request to update feature ban status
type UpdateFeatureBanRequest struct {
	BanState  bool   `json:"ban_state"`
	BanReason string `json:"ban_reason"`
}

// ================= PJSK =================

// AliasToIDResponse represents a response for alias to ID query
type AliasToIDResponse struct {
	MatchIDs []int `json:"match_ids"`
}

// AliasListResponse represents a response containing a list of aliases
type AliasListResponse struct {
	Aliases []string `json:"aliases"`
}

// AliasRequest represents a request to add an alias
type AliasRequest struct {
	Alias string `json:"alias"`
}

// RejectRequest represents a request to reject a pending alias
type RejectRequest struct {
	Reason string `json:"reason"`
}

// PendingAlias represents a pending alias review
type PendingAlias struct {
	ID          int       `json:"id"`
	AliasType   string    `json:"alias_type"`
	AliasTypeID int       `json:"alias_type_id"`
	Alias       string    `json:"alias"`
	SubmittedAt time.Time `json:"submitted_at"`
	SubmittedBy string    `json:"submitted_by"`
}

// PJSKBinding represents a PJSK account binding
type PJSKBinding struct {
	ID           int    `json:"id"`
	HarukiUserID int    `json:"haruki_user_id"`
	Server       string `json:"server"`
	UserID       string `json:"user_id"`
	Visible      bool   `json:"visible"`
}

// PJSKPreference represents a user preference setting
type PJSKPreference struct {
	Option string `json:"option"`
	Value  string `json:"value"`
}

// PJSKBindingResponse represents the response for getting bindings
type PJSKBindingResponse struct {
	Bindings []PJSKBinding `json:"bindings"`
}

// CreatePJSKBindingRequest represents a request to create a PJSK binding
type CreatePJSKBindingRequest struct {
	Server  string `json:"server"`
	UserID  string `json:"user_id"`
	Visible bool   `json:"visible"`
}

// SetPJSKDefaultBindingRequest represents a request to set default binding
type SetPJSKDefaultBindingRequest struct {
	Server    string `json:"server"`
	BindingID int    `json:"binding_id"`
}

// DeletePJSKDefaultBindingRequest represents a request to delete default binding
type DeletePJSKDefaultBindingRequest struct {
	Server string `json:"server"`
}

// UpdatePJSKBindingVisibilityRequest represents a request to update binding visibility
type UpdatePJSKBindingVisibilityRequest struct {
	Visible bool `json:"visible"`
}

// PJSKPreferencesResponse represents response for getting all preferences
type PJSKPreferencesResponse struct {
	Options []PJSKPreference `json:"options"`
}

// ================= Chunithm =================

// ChunithmMusicInfo represents basic music info
type ChunithmMusicInfo struct {
	MusicID        int       `json:"music_id"`
	Title          string    `json:"title"`
	Artist         string    `json:"artist"`
	Category       string    `json:"category"`
	Version        string    `json:"version"`
	ReleaseDate    time.Time `json:"release_date"`
	IsDeleted      bool      `json:"is_deleted"`
	DeletedVersion string    `json:"deleted_version"`
}

// ChunithmMusicDifficulty represents music difficulty info
type ChunithmMusicDifficulty struct {
	MusicID int     `json:"music_id"`
	Version string  `json:"version"`
	Diff0   float64 `json:"diff_0"`
	Diff1   float64 `json:"diff_1"`
	Diff2   float64 `json:"diff_2"`
	Diff3   float64 `json:"diff_3"`
	Diff4   float64 `json:"diff_4"`
}

// ChunithmChartData represents chart data statistics
type ChunithmChartData struct {
	Difficulty int     `json:"difficulty"`
	Creator    string  `json:"creator"`
	BPM        float64 `json:"bpm"`
	TapCount   int     `json:"tap_count"`
	HoldCount  int     `json:"hold_count"`
	SlideCount int     `json:"slide_count"`
	AirCount   int     `json:"air_count"`
	FlickCount int     `json:"flick_count"`
	TotalCount int     `json:"total_count"`
}

// ChunithmBinding represents Chunithm binding info
type ChunithmBinding struct {
	UserID int    `json:"user_id"`
	Server string `json:"server"`
	AimeID string `json:"aime_id"`
}

// ChunithmDefaultServer represents default server setting
type ChunithmDefaultServer struct {
	UserID int    `json:"user_id"`
	Server string `json:"server"`
}

// SetChunithmDefaultServerRequest represents request to set default server
type SetChunithmDefaultServerRequest struct {
	Server string `json:"server"`
}

// UpdateChunithmBindingRequest represents request to update binding
type UpdateChunithmBindingRequest struct {
	AimeID int `json:"aime_id"`
}
