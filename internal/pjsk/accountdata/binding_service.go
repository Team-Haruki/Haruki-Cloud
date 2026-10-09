package accountdata

import (
	"context"
	"errors"
	"fmt"
	"strings"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/gameaccount"
	"haruki-cloud/database/pjsk/userbinding"
	"haruki-cloud/database/pjsk/userdefaultbinding"
	usersdb "haruki-cloud/database/users"
	"haruki-cloud/internal/cluster"
	"haruki-cloud/internal/i18n"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/utils/censor"
	"haruki-cloud/utils/usererror"
)

const (
	bannedGameAccountBindThreshold = 3
	// Stored in the users database as the ban reason and shown verbatim in
	// later ban replies; data, not catalog copy.
	bannedGameAccountBindBanReason = "多次尝试绑定被封禁游戏账号" //copylint:ignore stored ban reason
)

// BindingService manages user game account bindings.
type BindingService struct {
	pjskDB       *pjskdb.Client
	usersDB      *usersdb.Client
	identity     IdentityResolver
	validator    ProfileValidator
	fastVerifier FastVerificationProvider
	bgStorage    ProfileBGStorage
	censor       *censor.Service
	readOnly     bool
}

func NewBindingService(pjskClient *pjskdb.Client, identityResolver IdentityResolver, validator ProfileValidator) *BindingService {
	return &BindingService{
		pjskDB:    pjskClient,
		identity:  identityResolver,
		validator: validator,
	}
}

func (s *BindingService) SetUsersDB(db *usersdb.Client) {
	if s == nil {
		return
	}
	s.usersDB = db
}

func (s *BindingService) SetFastVerificationProvider(provider FastVerificationProvider) {
	if s == nil {
		return
	}
	s.fastVerifier = provider
}

func (s *BindingService) SetProfileBGStorage(store ProfileBGStorage) {
	if s == nil {
		return
	}
	s.bgStorage = store
}

func (s *BindingService) SetCensorService(svc *censor.Service) {
	if s == nil {
		return
	}
	s.censor = svc
}

func (s *BindingService) SetReadOnly(readOnly bool) {
	if s == nil {
		return
	}
	s.readOnly = readOnly
}

func (s *BindingService) IsReady() bool {
	return s != nil && s.pjskDB != nil && s.identity != nil && s.validator != nil
}

func (s *BindingService) Bind(ctx context.Context, platform, platformUserID, rawUID string) (*BindResult, error) {
	if err := s.requireReady(platform, platformUserID); err != nil {
		return nil, err
	}
	if err := s.requireWritable(); err != nil {
		return nil, err
	}
	uid := normalizeUID(rawUID)
	if uid == "" {
		return nil, usererror.Misuse(i18n.M("binding.bind.uid_required"))
	}
	if !isNumericUID(uid) {
		return nil, usererror.BadParam(uid, i18n.M("common.param.uid_digits"))
	}

	harukiUserID, err := s.identity.ResolveOrCreate(ctx, platform, platformUserID)
	if err != nil {
		return nil, err
	}

	matches, err := s.probeUID(ctx, uid)
	if err != nil {
		return nil, err
	}
	target := matches[0]

	tx, err := s.pjskDB.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	account, err := getOrCreateGameAccountTx(ctx, tx, target.Server, target.UserID)
	if err != nil {
		return nil, err
	}

	binding, err := tx.UserBinding.Query().
		Where(
			userbinding.HarukiUserID(harukiUserID),
			userbinding.GameAccountIDEQ(account.ID),
		).
		WithGameAccount().
		Only(ctx)

	alreadyBound := false
	switch {
	case err == nil:
		alreadyBound = true
	case pjskdb.IsNotFound(err):
		if account.IsBanned {
			banned, err := s.recordBannedGameAccountBindAttempt(ctx, harukiUserID)
			if err != nil {
				return nil, err
			}
			if banned {
				return nil, banError(i18n.M("moderation.feature.pjsk"), bannedGameAccountBindBanReason)
			}
			return nil, usererror.Forbidden(i18n.M("moderation.bind_banned_account_warning"))
		}
		displayOrder, orderErr := nextBindingDisplayOrderTx(ctx, tx, harukiUserID)
		if orderErr != nil {
			return nil, orderErr
		}
		binding, err = createBindingVisibility(tx.UserBinding.Create().
			SetHarukiUserID(harukiUserID).
			SetGameAccountID(account.ID).
			SetDisplayOrder(displayOrder), NewBindingVisibility).
			Save(ctx)
		if err != nil {
			return nil, err
		}
	default:
		return nil, err
	}

	setGlobalDefault, err := ensureDefaultBindingTx(ctx, tx, harukiUserID, GlobalDefaultBindingScope, binding.ID)
	if err != nil {
		return nil, err
	}
	setServerDefault, err := ensureDefaultBindingTx(ctx, tx, harukiUserID, target.Server, binding.ID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil

	return &BindResult{
		Server:              target.Server,
		UserID:              target.UserID,
		UserName:            target.UserName,
		AlreadyBound:        alreadyBound,
		SetGlobalDefault:    setGlobalDefault,
		SetServerDefault:    setServerDefault,
		MultipleServerMatch: len(matches) > 1,
	}, nil
}

func (s *BindingService) List(ctx context.Context, platform, platformUserID string) ([]BindingListItem, error) {
	if err := s.requireReady(platform, platformUserID); err != nil {
		return nil, err
	}
	harukiUserID, err := s.identity.ResolveOrCreate(ctx, platform, platformUserID)
	if err != nil {
		return nil, err
	}
	bindings, err := s.pjskDB.UserBinding.Query().
		Where(userbinding.HarukiUserID(harukiUserID)).
		WithGameAccount().
		All(ctx)
	if err != nil {
		return nil, err
	}
	defaults, err := s.pjskDB.UserDefaultBinding.Query().
		Where(userdefaultbinding.HarukiUserID(harukiUserID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return buildBindingListWithBackgrounds(bindings, defaults), nil
}

func (s *BindingService) Unbind(ctx context.Context, platform, platformUserID, selector, server string) (*UnbindResult, error) {
	if err := s.requireReady(platform, platformUserID); err != nil {
		return nil, err
	}
	if err := s.requireWritable(); err != nil {
		return nil, err
	}
	harukiUserID, err := s.identity.ResolveOrCreate(ctx, platform, platformUserID)
	if err != nil {
		return nil, err
	}

	bindings, err := s.pjskDB.UserBinding.Query().
		Where(userbinding.HarukiUserID(harukiUserID)).
		WithGameAccount().
		All(ctx)
	if err != nil {
		return nil, err
	}
	defaults, err := s.pjskDB.UserDefaultBinding.Query().
		Where(userdefaultbinding.HarukiUserID(harukiUserID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	items := buildBindingList(bindings, defaults)
	target, err := selectBinding(items, selector, server)
	if err != nil {
		return nil, err
	}

	hadGlobalDefault := target.IsGlobalDefault
	hadServerDefault := target.IsServerDefault

	tx, err := s.pjskDB.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.UserDefaultBinding.Delete().
		Where(userdefaultbinding.BindingID(target.BindingID)).
		Exec(ctx); err != nil {
		return nil, err
	}

	if err := tx.UserBinding.DeleteOneID(target.BindingID).Exec(ctx); err != nil {
		return nil, err
	}

	result := &UnbindResult{Removed: target}

	remainingBindings, err := tx.UserBinding.Query().
		Where(userbinding.HarukiUserID(harukiUserID)).
		WithGameAccount().
		All(ctx)
	if err != nil {
		return nil, err
	}
	remainingDefaults, err := tx.UserDefaultBinding.Query().
		Where(userdefaultbinding.HarukiUserID(harukiUserID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	remainingItems := buildBindingList(remainingBindings, remainingDefaults)

	if hadGlobalDefault && !hasDefaultScope(remainingDefaults, GlobalDefaultBindingScope) && len(remainingItems) > 0 {
		if _, err := ensureDefaultBindingTx(ctx, tx, harukiUserID, GlobalDefaultBindingScope, remainingItems[0].BindingID); err != nil {
			return nil, err
		}
		item := remainingItems[0]
		item.IsGlobalDefault = true
		result.ReassignedGlobal = &item
	}

	if hadServerDefault && !hasDefaultScope(remainingDefaults, target.Server) {
		for _, item := range remainingItems {
			if item.Server != target.Server {
				continue
			}
			if _, err := ensureDefaultBindingTx(ctx, tx, harukiUserID, target.Server, item.BindingID); err != nil {
				return nil, err
			}
			item.IsServerDefault = true
			result.ReassignedServer = &item
			break
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return result, nil
}

func (s *BindingService) requireReady(platform, platformUserID string) error {
	if !s.IsReady() {
		return ErrBindingServiceUnavailable
	}
	if strings.TrimSpace(platform) == "" || strings.TrimSpace(platformUserID) == "" {
		return fmt.Errorf("platform and platform_user_id are required for binding commands")
	}
	return nil
}

func (s *BindingService) requireWritable() error {
	if s == nil {
		return ErrBindingServiceUnavailable
	}
	return cluster.EnsureWritable(s.readOnly)
}

func (s *BindingService) recordBannedGameAccountBindAttempt(ctx context.Context, harukiUserID int) (bool, error) {
	if s == nil || s.usersDB == nil {
		return false, nil
	}

	u, err := s.usersDB.User.Get(ctx, harukiUserID)
	if err != nil {
		if usersdb.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	nextAttempts := u.PjskBannedGameAccountBindAttempts + 1
	update := s.usersDB.User.UpdateOneID(harukiUserID).
		SetPjskBannedGameAccountBindAttempts(nextAttempts)
	banned := nextAttempts >= bannedGameAccountBindThreshold
	if banned {
		update.SetPjskBanState(true).
			SetPjskBanReason(bannedGameAccountBindBanReason)
	}
	return banned, update.Exec(ctx)
}

func (s *BindingService) probeUID(ctx context.Context, uid string) ([]profileProbe, error) {
	results := make([]profileProbe, 0, len(AllBindingServers))
	failures := make([]i18n.Message, 0, len(AllBindingServers))
	var causes []error
	serviceFailed := false

	for _, server := range AllBindingServers {
		var resp *sekaiapi.GetAnotherProfileResponse
		var err error
		if validator, ok := s.validator.(contextProfileValidator); ok {
			resp, err = validator.GetUserProfileContext(ctx, server.String(), uid)
		} else {
			resp, err = s.validator.GetUserProfile(server.String(), uid)
		}
		if err == nil {
			name := strings.TrimSpace(resp.User.Name)
			if name == "" {
				name = uid
			}
			results = append(results, profileProbe{
				Server:   server.String(),
				UserID:   uid,
				UserName: name,
			})
			continue
		}

		region := i18n.RegionLabel(server.String())
		switch {
		case errors.Is(err, sekaiapi.ErrUserNotFound):
			failures = append(failures, i18n.M("binding.bind.failure.not_found", i18n.Data{"Region": region}))
		case errors.Is(err, sekaiapi.ErrServerMaintenance):
			failures = append(failures, i18n.M("binding.bind.failure.maintenance", i18n.Data{"Region": region}))
			serviceFailed = true
		default:
			failures = append(failures, i18n.M("binding.bind.failure.error", i18n.Data{"Region": region}))
			serviceFailed = true
			causes = append(causes, fmt.Errorf("%s: %w", server.String(), err))
		}
	}

	if len(results) == 0 {
		code := usererror.CodeNotFound
		if serviceFailed {
			code = usererror.CodeUnavailable
		}
		return nil, usererror.Wrap(code, i18n.M("binding.bind.failed_all", i18n.Data{"Lines": failures}), errors.Join(causes...))
	}
	return results, nil
}

func getOrCreateGameAccountTx(ctx context.Context, tx *pjskdb.Tx, server, userID string) (*pjskdb.GameAccount, error) {
	server = strings.TrimSpace(strings.ToLower(server))
	userID = strings.TrimSpace(userID)
	if server == "" || userID == "" {
		return nil, fmt.Errorf("invalid game account identity")
	}

	account, err := tx.GameAccount.Query().
		Where(
			gameaccount.ServerEQ(server),
			gameaccount.UserIDEQ(userID),
		).
		Only(ctx)
	switch {
	case err == nil:
		return account, nil
	case !pjskdb.IsNotFound(err):
		return nil, err
	}

	return tx.GameAccount.Create().
		SetServer(server).
		SetUserID(userID).
		Save(ctx)
}
