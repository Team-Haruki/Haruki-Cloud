package userdata

import (
	"context"
	"fmt"

	usersdb "haruki-cloud/database/users"
	"haruki-cloud/database/users/user"
	"haruki-cloud/internal/pjsk/parser"
)

// BanService checks per-user feature ban states stored in the users database.
// It applies a three-level hierarchy: global ban → module ban → feature ban.
type BanService struct {
	db *usersdb.Client
}

// NewBanService creates a new BanService backed by the given users DB client.
func NewBanService(db *usersdb.Client) *BanService {
	if db == nil {
		return nil
	}
	return &BanService{db: db}
}

// CheckBan returns a non-nil error (containing the ban reason) if the user
// identified by (platform, userID) is banned for the given module.
// Returns nil if the user is allowed or has no record.
func (s *BanService) CheckBan(ctx context.Context, platform, userID string, module parser.TargetModule) error {
	if s == nil || s.db == nil {
		return nil
	}

	u, err := s.db.User.Query().
		Where(user.Platform(platform), user.UserID(userID)).
		Only(ctx)
	if err != nil {
		if usersdb.IsNotFound(err) {
			return nil
		}
		return nil // DB error: fail open (don't block users)
	}

	if u.BanState {
		return banError("所有功能", u.BanReason)
	}

	if isPJSKModule(module) {
		if u.PjskBanState {
			return banError("PJSK 功能", u.PjskBanReason)
		}
		switch featureBanFor(module) {
		case featureMain:
			if u.PjskMainBanState {
				return banError("PJSK 主要功能", u.PjskMainBanReason)
			}
		case featureRanking:
			if u.PjskRankingBanState {
				return banError("PJSK 排名功能", u.PjskRankingBanReason)
			}
		case featureAlias:
			if u.PjskAliasBanState {
				return banError("PJSK 别名功能", u.PjskAliasBanReason)
			}
		case featureMysekai:
			if u.PjskMysekaiBanState {
				return banError("PJSK MySekai 功能", u.PjskMysekaiBanReason)
			}
		}
	}

	return nil
}

type featureCategory int

const (
	featureNone featureCategory = iota
	featureMain
	featureRanking
	featureAlias
	featureMysekai
)

func isPJSKModule(m parser.TargetModule) bool {
	switch m {
	case parser.ModuleCard, parser.ModuleGacha, parser.ModuleMusic, parser.ModuleEvent,
		parser.ModuleDeck, parser.ModuleSK, parser.ModuleMysekai, parser.ModuleProfile,
		parser.ModuleHelp, parser.ModuleEducation, parser.ModuleScore, parser.ModuleStamp,
		parser.ModuleMisc, parser.ModuleArrest, parser.ModuleRegTime, parser.ModuleCheckData:
		return true
	case parser.ModuleAlias:
		return true
	}
	return false
}

func featureBanFor(m parser.TargetModule) featureCategory {
	switch m {
	case parser.ModuleSK:
		return featureRanking
	case parser.ModuleAlias:
		return featureAlias
	case parser.ModuleMysekai:
		return featureMysekai
	default:
		return featureMain
	}
}

func banError(featureName, reason string) error {
	if reason == "" {
		return fmt.Errorf("您已被禁止使用%s", featureName)
	}
	return fmt.Errorf("您已被禁止使用%s，原因：%s", featureName, reason)
}
