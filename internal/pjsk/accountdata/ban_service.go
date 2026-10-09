package accountdata

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	usersdb "haruki-cloud/database/users"
	"haruki-cloud/database/users/user"
	"haruki-cloud/internal/cluster"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/identity"
	"haruki-cloud/internal/pjsk/displaytime"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/utils/usererror"
)

// BanReasonMaxRunes is the longest ban reason /kill accepts.
const BanReasonMaxRunes = 255

// BanService checks per-user feature ban states stored in the users database.
// It applies a three-level hierarchy: global ban → module ban → feature ban.
type BanService struct {
	db       *usersdb.Client
	identity *identity.Resolver
	readOnly bool
	admins   map[string]struct{}
}

// SetAdminQQIDs replaces the explicit roster authorized to run global
// moderation commands. Invalid or empty values are ignored.
func (s *BanService) SetAdminQQIDs(qqIDs []string) {
	if s == nil {
		return
	}
	admins := make(map[string]struct{}, len(qqIDs))
	for _, value := range qqIDs {
		qqID, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err == nil && qqID > 0 {
			admins[strconv.FormatInt(qqID, 10)] = struct{}{}
		}
	}
	s.admins = admins
}

// IsAdmin reports whether an identity is explicitly authorized for global
// moderation. Only QQ identities are supported by /kill and /back.
func (s *BanService) IsAdmin(platform, userID string) bool {
	if s == nil || strings.TrimSpace(platform) != "qq" {
		return false
	}
	_, ok := s.admins[strings.TrimSpace(userID)]
	return ok
}

// NewBanService creates a new BanService backed by the given users DB client.
func NewBanService(db *usersdb.Client) *BanService {
	if db == nil {
		return nil
	}
	return &BanService{db: db, identity: identity.NewResolver(db)}
}

// SetReadOnly disables moderation mutations while preserving ban checks.
func (s *BanService) SetReadOnly(readOnly bool) {
	if s == nil {
		return
	}
	s.readOnly = readOnly
	if s.identity != nil {
		s.identity.SetReadOnly(readOnly)
	}
}

// GlobalBanStatus is the effective global-ban state for an identity. An
// expired timed ban is reported as inactive even if its historical database
// fields have not yet been cleared.
type GlobalBanStatus struct {
	Active    bool
	Reason    string
	ExpiresAt *time.Time
}

// GlobalBanStatus returns the effective global ban for an identity.
func (s *BanService) GlobalBanStatus(ctx context.Context, platform, userID string) (GlobalBanStatus, error) {
	if s == nil || s.db == nil {
		return GlobalBanStatus{}, nil
	}
	u, err := s.db.User.Query().
		Where(user.Platform(platform), user.UserID(userID)).
		Only(ctx)
	if err != nil {
		if usersdb.IsNotFound(err) {
			return GlobalBanStatus{}, nil
		}
		return GlobalBanStatus{}, err
	}
	return globalBanStatusForUser(u), nil
}

// IsGloballyBanned is the compact checker used by Bot authentication paths.
func (s *BanService) IsGloballyBanned(ctx context.Context, platform, userID string) (bool, error) {
	status, err := s.GlobalBanStatus(ctx, platform, userID)
	return status.Active, err
}

// Kill globally bans a QQ identity. A nil expiresAt creates a permanent ban.
// The identity is created first when it has never used Haruki, so the ban also
// applies to future first use.
func (s *BanService) Kill(ctx context.Context, qqID, reason string, expiresAt *time.Time) (GlobalBanStatus, error) {
	if s == nil || s.db == nil || s.identity == nil {
		return GlobalBanStatus{}, usererror.Misconfigured(errBanServiceNotConfigured)
	}
	if err := cluster.EnsureWritable(s.readOnly); err != nil {
		return GlobalBanStatus{}, err
	}
	qqID = strings.TrimSpace(qqID)
	reason = strings.TrimSpace(reason)
	if qqID == "" || reason == "" {
		return GlobalBanStatus{}, usererror.Usage(i18n.M("moderation.kill.usage_reason"), "/kill")
	}
	if len([]rune(reason)) > BanReasonMaxRunes {
		return GlobalBanStatus{}, usererror.Invalid(i18n.M("moderation.kill.reason_too_long", i18n.Data{"Max": BanReasonMaxRunes}))
	}
	if expiresAt != nil && !expiresAt.After(time.Now()) {
		return GlobalBanStatus{}, usererror.Invalid(i18n.M("moderation.kill.expiry_past"))
	}
	if _, err := s.identity.ResolveOrCreate(ctx, "qq", qqID); err != nil {
		return GlobalBanStatus{}, err
	}
	update := s.db.User.Update().
		Where(user.PlatformEQ("qq"), user.UserIDEQ(qqID)).
		SetBanState(true).
		SetBanReason(reason)
	if expiresAt == nil {
		update = update.ClearBanExpiresAt()
	} else {
		update = update.SetBanExpiresAt(*expiresAt)
	}
	if _, err := update.Save(ctx); err != nil {
		return GlobalBanStatus{}, err
	}
	return GlobalBanStatus{Active: true, Reason: reason, ExpiresAt: expiresAt}, nil
}

const CNMySekaiAttemptThreshold = 3

// CNMySekaiAttempt is the outcome of recording a blocked CN MySekai request.
// Attempts is 0 when the service could not track the identity.
type CNMySekaiAttempt struct {
	Attempts  int
	Threshold int
	Silenced  bool
}

// RecordCNMySekaiAttempt grants three notices per identity, shared by all
// blocked CN MySekai commands. Later requests stay silent across restarts.
func (s *BanService) RecordCNMySekaiAttempt(ctx context.Context, platform, userID string) (CNMySekaiAttempt, error) {
	result := CNMySekaiAttempt{Threshold: CNMySekaiAttemptThreshold}
	if s == nil || s.db == nil || s.identity == nil {
		return result, nil
	}
	platform = strings.TrimSpace(platform)
	userID = strings.TrimSpace(userID)
	if platform == "" || userID == "" {
		return result, nil
	}
	id, err := s.identity.ResolveOrCreate(ctx, platform, userID)
	if err != nil {
		return result, err
	}
	for {
		u, err := s.db.User.Get(ctx, id)
		if err != nil {
			return result, err
		}
		if u.PjskCnMysekaiAttempts >= CNMySekaiAttemptThreshold {
			result.Attempts = CNMySekaiAttemptThreshold
			result.Silenced = true
			return result, nil
		}
		if err := cluster.EnsureWritable(s.readOnly); err != nil {
			return result, err
		}
		// Compare-and-swap prevents concurrent commands from receiving the
		// same warning number or exceeding the three-notice limit.
		updated, err := s.db.User.Update().
			Where(user.IDEQ(id), user.PjskCnMysekaiAttemptsEQ(u.PjskCnMysekaiAttempts)).
			AddPjskCnMysekaiAttempts(1).
			Save(ctx)
		if err != nil {
			return result, err
		}
		if updated > 0 {
			result.Attempts = u.PjskCnMysekaiAttempts + 1
			return result, nil
		}
	}
}

// Back removes a global ban and all of its metadata from a QQ identity.
func (s *BanService) Back(ctx context.Context, qqID string) error {
	if s == nil || s.db == nil {
		return usererror.Misconfigured(errBanServiceNotConfigured)
	}
	if err := cluster.EnsureWritable(s.readOnly); err != nil {
		return err
	}
	qqID = strings.TrimSpace(qqID)
	if qqID == "" {
		return usererror.Usage(i18n.M("moderation.back.usage_reason"), "/back")
	}
	count, err := s.db.User.Update().
		Where(user.PlatformEQ("qq"), user.UserIDEQ(qqID)).
		SetBanState(false).
		ClearBanReason().
		ClearBanExpiresAt().
		Save(ctx)
	if err != nil {
		return err
	}
	if count == 0 {
		return usererror.New(usererror.CodeNotFound, i18n.M("moderation.back.not_found", i18n.Data{"QQ": qqID}))
	}
	return nil
}

// CheckBan returns a typed usererror (CodeForbidden, with the ban reason and,
// for a timed global ban, the expiry in the requester's time zone) if the
// user identified by (platform, userID) is banned for the given module.
// Returns nil if the user is allowed or has no record.
func (s *BanService) CheckBan(ctx context.Context, platform, userID string, module parser.TargetModule) error {
	if s == nil || s.db == nil {
		return nil
	}

	u, err := s.db.User.Query().
		Where(user.Platform(platform), user.UserID(userID)).
		Only(ctx)
	if err != nil {
		return nil // Missing record or DB error: fail open (don't block users).
	}
	if status := globalBanStatusForUser(u); status.Active {
		return globalBanError(status, displaytime.RequestLocation(ctx))
	}

	if !isPJSKModule(module) {
		return nil
	}
	return pjskBanError(u, featureBanFor(module))
}

func pjskBanError(u *usersdb.User, feature featureCategory) error {
	if u.PjskBanState {
		return banError(i18n.M("moderation.feature.pjsk"), u.PjskBanReason)
	}
	switch feature {
	case featureMain:
		return activeFeatureBanError(u.PjskMainBanState, i18n.M("moderation.feature.main"), u.PjskMainBanReason)
	case featureRanking:
		return activeFeatureBanError(u.PjskRankingBanState, i18n.M("moderation.feature.ranking"), u.PjskRankingBanReason)
	case featureAlias:
		return activeFeatureBanError(u.PjskAliasBanState, i18n.M("moderation.feature.alias"), u.PjskAliasBanReason)
	case featureMysekai:
		return activeFeatureBanError(u.PjskMysekaiBanState, i18n.M("moderation.feature.mysekai"), u.PjskMysekaiBanReason)
	default:
		return nil
	}
}

func activeFeatureBanError(active bool, label i18n.Message, reason string) error {
	if !active {
		return nil
	}
	return banError(label, reason)
}

func globalBanStatusForUser(u *usersdb.User) GlobalBanStatus {
	if u == nil || !u.BanState || u.BanExpiresAt != nil && !u.BanExpiresAt.After(time.Now()) {
		return GlobalBanStatus{}
	}
	return GlobalBanStatus{
		Active:    true,
		Reason:    strings.TrimSpace(u.BanReason),
		ExpiresAt: u.BanExpiresAt,
	}
}

func globalBanError(status GlobalBanStatus, loc *time.Location) error {
	feature := i18n.M("moderation.feature.all")
	if status.ExpiresAt == nil {
		return banError(feature, status.Reason)
	}
	expiresAt := i18n.FormatUserTime(*status.ExpiresAt, loc)
	if status.Reason == "" {
		return usererror.Forbidden(i18n.M("moderation.banned_until", i18n.Data{"Feature": feature, "ExpiresAt": expiresAt}))
	}
	return usererror.Forbidden(i18n.M("moderation.banned_reason_until", i18n.Data{
		"Feature":   feature,
		"Reason":    status.Reason,
		"ExpiresAt": expiresAt,
	}))
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

var errBanServiceNotConfigured = errors.New("ban service: users database is not configured")

func banError(feature i18n.Message, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return usererror.Forbidden(i18n.M("moderation.banned", i18n.Data{"Feature": feature}))
	}
	return usererror.Forbidden(i18n.M("moderation.banned_reason", i18n.Data{"Feature": feature, "Reason": reason}))
}
