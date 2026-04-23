package alias

import (
	"context"

	pjskdb "haruki-cloud/database/pjsk"
	sekaiDB "haruki-cloud/database/sekai"
)

// ── Mode constants ──────────────────────────────────────────────────────────

const (
	ModeDelete      = "alias-delete"
	ModeAdd         = "alias-add"
	ModeQuery       = "alias-query"
	ModePendingList = "alias-pending-list"
	ModeApprove     = "alias-approve"
	ModeReject      = "alias-reject"
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

// ── Service types ───────────────────────────────────────────────────────────

type IdentityResolver interface {
	ResolveOrCreate(ctx context.Context, platform, userID string) (int, error)
}

type Service struct {
	sekai    *sekaiDB.Client
	pjsk     *pjskdb.Client
	identity IdentityResolver
}

type EntityRef struct {
	AliasType string
	ID        int
	Name      string
}

type PjskAliasRecord struct {
	ReviewID int64
	Entity   EntityRef
	Alias    string
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
