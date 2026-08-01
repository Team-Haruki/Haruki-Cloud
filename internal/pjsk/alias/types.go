package alias

import (
	"context"

	pjskdb "haruki-cloud/database/pjsk"
	sekaiDB "haruki-cloud/database/sekai"
)

// ── Mode constants ──────────────────────────────────────────────────────────

const (
	ModeDelete       = "alias-delete"
	ModeAdd          = "alias-add"
	ModeQuery        = "alias-query"
	ModePendingList  = "alias-pending-list"
	ModeSubmitter    = "alias-submitter"
	ModeBanSubmitter = "alias-ban-submitter"
	ModeApprove      = "alias-approve"
	ModeReject       = "alias-reject"
	ModeBatchReject  = "alias-batch-reject"
)

const (
	PjskAliasTypeMusic     = "music"
	PjskAliasTypeCharacter = "character"
)

var supportedAliasTypes = []string{PjskAliasTypeMusic, PjskAliasTypeCharacter}

// ── Command params ──────────────────────────────────────────────────────────

type DeleteCommandParams struct {
	AliasType      string   `json:"alias_type"`
	Platform       string   `json:"platform"`
	PlatformUserID string   `json:"platform_user_id"`
	Target         string   `json:"target"`
	Aliases        []string `json:"aliases"`
}

type AddCommandParams struct {
	AliasType      string   `json:"alias_type"`
	Platform       string   `json:"platform"`
	PlatformUserID string   `json:"platform_user_id"`
	Target         string   `json:"target"`
	Aliases        []string `json:"aliases"`
}

type QueryCommandParams struct {
	AliasType string `json:"alias_type"`
	Target    string `json:"target"`
}

type ReviewListCommandParams struct {
	Platform       string `json:"platform"`
	PlatformUserID string `json:"platform_user_id"`
}

type SubmitterCommandParams struct {
	Platform       string `json:"platform"`
	PlatformUserID string `json:"platform_user_id"`
	ReviewID       int64  `json:"review_id"`
}

type BanSubmitterCommandParams struct {
	Platform             string `json:"platform"`
	PlatformUserID       string `json:"platform_user_id"`
	TargetPlatform       string `json:"target_platform"`
	TargetPlatformUserID string `json:"target_platform_user_id"`
}

type ApproveCommandParams struct {
	Platform       string  `json:"platform"`
	PlatformUserID string  `json:"platform_user_id"`
	ReviewIDs      []int64 `json:"review_ids"`
}

type RejectCommandParams struct {
	Platform       string `json:"platform"`
	PlatformUserID string `json:"platform_user_id"`
	ReviewID       int64  `json:"review_id"`
	Reason         string `json:"reason"`
}

type BatchRejectCommandParams struct {
	Platform       string  `json:"platform"`
	PlatformUserID string  `json:"platform_user_id"`
	ReviewIDs      []int64 `json:"review_ids"`
}

// ── Service types ───────────────────────────────────────────────────────────

type IdentityResolver interface {
	ResolveOrCreate(ctx context.Context, platform, userID string) (int, error)
}

type Service struct {
	sekai    *sekaiDB.Client
	pjsk     *pjskdb.Client
	identity IdentityResolver
	readOnly bool
}

type EntityRef struct {
	AliasType string
	ID        int
	Name      string
}

type PjskAliasRecord struct {
	ReviewID    int64
	Entity      EntityRef
	Alias       string
	SubmittedBy string
}

type SubmissionBanRecord struct {
	Platform       string
	PlatformUserID string
	BannedBy       string
}

type ApprovedAliasRecord struct {
	AliasID int64
	Entity  EntityRef
	Alias   string
}

type QueryResult struct {
	Entity  EntityRef
	Aliases []string
}

type entityKey struct {
	aliasType string
	id        int
}
