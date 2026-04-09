package alias

import (
	"context"

	pjskdb "haruki-cloud/database/pjsk"
	sekaiDB "haruki-cloud/database/sekai"
)

const (
	AliasTypeMusic     = "music"
	AliasTypeCharacter = "character"
)

var supportedAliasTypes = []string{AliasTypeMusic, AliasTypeCharacter}

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

type AliasRecord struct {
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

func NewService(sekai *sekaiDB.Client, pjsk *pjskdb.Client, identity IdentityResolver) *Service {
	if sekai == nil || pjsk == nil {
		return nil
	}
	return &Service{
		sekai:    sekai,
		pjsk:     pjsk,
		identity: identity,
	}
}

func (s *Service) IsReady() bool {
	return s != nil && s.sekai != nil && s.pjsk != nil
}
