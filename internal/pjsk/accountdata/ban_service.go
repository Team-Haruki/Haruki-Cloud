package accountdata

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	usersdb "haruki-cloud/database/users"
	"haruki-cloud/database/users/user"
	"haruki-cloud/internal/cluster"
	"haruki-cloud/internal/identity"
	"haruki-cloud/internal/pjsk/parser"
)

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
		return GlobalBanStatus{}, fmt.Errorf("用户封禁服务未就绪，请稍后再试")
	}
	if err := cluster.EnsureWritable(s.readOnly); err != nil {
		return GlobalBanStatus{}, err
	}
	qqID = strings.TrimSpace(qqID)
	reason = strings.TrimSpace(reason)
	if qqID == "" || reason == "" {
		return GlobalBanStatus{}, fmt.Errorf("QQ号和封禁原因不能为空")
	}
	if len([]rune(reason)) > 255 {
		return GlobalBanStatus{}, fmt.Errorf("封禁原因不能超过255个字符")
	}
	if expiresAt != nil && !expiresAt.After(time.Now()) {
		return GlobalBanStatus{}, fmt.Errorf("封禁到期时间必须晚于当前时间")
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

// CN MySekai gate: every blocked request is counted per identity and the
// third one converts into a temporary global ban.
const (
	CNMySekaiAttemptThreshold = 3
	cnMySekaiBanReason        = "多次尝试使用国服未开启的 MySekai 功能"
	defaultCNMySekaiBanFor    = 10 * time.Minute
)

// CNMySekaiAttempt is the outcome of recording one blocked CN MySekai request.
// Attempts is 0 when the service could not track the identity.
type CNMySekaiAttempt struct {
	Attempts  int
	Threshold int
	Banned    bool
	ExpiresAt time.Time
}

// RecordCNMySekaiAttempt counts a blocked CN MySekai request for an identity.
// Reaching CNMySekaiAttemptThreshold sets a global ban that expires after
// banFor (ten minutes when banFor is not positive) and resets the counter so the
// next three attempts after the ban lapses warn again before banning.
func (s *BanService) RecordCNMySekaiAttempt(ctx context.Context, platform, userID string, banFor time.Duration) (CNMySekaiAttempt, error) {
	result := CNMySekaiAttempt{Threshold: CNMySekaiAttemptThreshold}
	if s == nil || s.db == nil || s.identity == nil {
		return result, nil
	}
	platform = strings.TrimSpace(platform)
	userID = strings.TrimSpace(userID)
	if platform == "" || userID == "" {
		return result, nil
	}
	if err := cluster.EnsureWritable(s.readOnly); err != nil {
		return result, err
	}
	if banFor <= 0 {
		banFor = defaultCNMySekaiBanFor
	}

	id, err := s.identity.ResolveOrCreate(ctx, platform, userID)
	if err != nil {
		return result, err
	}
	u, err := s.db.User.Get(ctx, id)
	if err != nil {
		return result, err
	}

	attempts := u.PjskCnMysekaiAttempts + 1
	update := s.db.User.UpdateOneID(id)
	if attempts >= CNMySekaiAttemptThreshold {
		expiresAt := time.Now().Add(banFor)
		update.SetPjskCnMysekaiAttempts(0).
			SetBanState(true).
			SetBanReason(cnMySekaiBanReason).
			SetBanExpiresAt(expiresAt)
		result.Banned = true
		result.ExpiresAt = expiresAt
	} else {
		update.SetPjskCnMysekaiAttempts(attempts)
	}
	if err := update.Exec(ctx); err != nil {
		return result, err
	}
	result.Attempts = attempts
	return result, nil
}

// Back removes a global ban and all of its metadata from a QQ identity.
func (s *BanService) Back(ctx context.Context, qqID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("用户封禁服务未就绪，请稍后再试")
	}
	if err := cluster.EnsureWritable(s.readOnly); err != nil {
		return err
	}
	qqID = strings.TrimSpace(qqID)
	if qqID == "" {
		return fmt.Errorf("QQ号不能为空")
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
		return fmt.Errorf("未找到QQ用户 %s", qqID)
	}
	return nil
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
		return nil // Missing record or DB error: fail open (don't block users).
	}
	if status := globalBanStatusForUser(u); status.Active {
		return globalBanError(status)
	}

	if !isPJSKModule(module) {
		return nil
	}
	return pjskBanError(u, featureBanFor(module))
}

func pjskBanError(u *usersdb.User, feature featureCategory) error {
	if u.PjskBanState {
		return banError("PJSK 功能", u.PjskBanReason)
	}
	switch feature {
	case featureMain:
		return activeFeatureBanError(u.PjskMainBanState, "PJSK 主要功能", u.PjskMainBanReason)
	case featureRanking:
		return activeFeatureBanError(u.PjskRankingBanState, "PJSK 排名功能", u.PjskRankingBanReason)
	case featureAlias:
		return activeFeatureBanError(u.PjskAliasBanState, "PJSK 别名功能", u.PjskAliasBanReason)
	case featureMysekai:
		return activeFeatureBanError(u.PjskMysekaiBanState, "PJSK MySekai 功能", u.PjskMysekaiBanReason)
	default:
		return nil
	}
}

func activeFeatureBanError(active bool, label, reason string) error {
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

func globalBanError(status GlobalBanStatus) error {
	err := banError("所有功能", status.Reason)
	if status.ExpiresAt == nil {
		return err
	}
	return fmt.Errorf("%s，封禁至：%s", err.Error(), status.ExpiresAt.Format("2006-01-02 15:04:05 MST"))
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
