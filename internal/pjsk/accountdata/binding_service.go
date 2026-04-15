package accountdata

import (
	"context"
	"errors"
	"fmt"
	"strings"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/userbinding"
	"haruki-cloud/database/pjsk/userdefaultbinding"
	"haruki-cloud/utils/censor"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

// BindingService manages user game account bindings.
type BindingService struct {
	pjskDB       *pjskdb.Client
	identity     IdentityResolver
	validator    ProfileValidator
	fastVerifier FastVerificationProvider
	bgStorage    ProfileBGStorage
	censor       *censor.Service
}

func NewBindingService(pjskClient *pjskdb.Client, identityResolver IdentityResolver, validator ProfileValidator) *BindingService {
	return &BindingService{
		pjskDB:    pjskClient,
		identity:  identityResolver,
		validator: validator,
	}
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

func (s *BindingService) IsReady() bool {
	return s != nil && s.pjskDB != nil && s.identity != nil && s.validator != nil
}

func (s *BindingService) Bind(ctx context.Context, platform, platformUserID, rawUID string) (*BindResult, error) {
	if err := s.requireReady(platform, platformUserID); err != nil {
		return nil, err
	}
	uid := normalizeUID(rawUID)
	if uid == "" {
		return nil, fmt.Errorf("请提供要绑定的游戏ID")
	}
	if !isNumericUID(uid) {
		return nil, fmt.Errorf("游戏ID必须为纯数字")
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

	binding, err := tx.UserBinding.Query().
		Where(
			userbinding.HarukiUserID(harukiUserID),
			userbinding.Server(target.Server),
			userbinding.UserID(target.UserID),
		).
		Only(ctx)

	alreadyBound := false
	switch {
	case err == nil:
		alreadyBound = true
	case pjskdb.IsNotFound(err):
		binding, err = tx.UserBinding.Create().
			SetHarukiUserID(harukiUserID).
			SetServer(target.Server).
			SetUserID(target.UserID).
			SetVisible(false).
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
	return buildBindingList(bindings, defaults), nil
}

func (s *BindingService) Unbind(ctx context.Context, platform, platformUserID, selector, server string) (*UnbindResult, error) {
	if err := s.requireReady(platform, platformUserID); err != nil {
		return nil, err
	}
	harukiUserID, err := s.identity.ResolveOrCreate(ctx, platform, platformUserID)
	if err != nil {
		return nil, err
	}

	bindings, err := s.pjskDB.UserBinding.Query().
		Where(userbinding.HarukiUserID(harukiUserID)).
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

func (s *BindingService) probeUID(ctx context.Context, uid string) ([]profileProbe, error) {
	results := make([]profileProbe, 0, len(AllBindingServers))
	failures := make([]string, 0, len(AllBindingServers))

	for _, server := range AllBindingServers {
		resp, err := s.validator.GetUserProfile(server.String(), uid)
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

		switch {
		case errors.Is(err, sekaiapi.ErrUserNotFound):
			failures = append(failures, fmt.Sprintf("%s: 用户不存在", strings.ToUpper(server.String())))
		case errors.Is(err, sekaiapi.ErrServerMaintenance):
			failures = append(failures, fmt.Sprintf("%s: 服务器维护中", strings.ToUpper(server.String())))
		default:
			failures = append(failures, fmt.Sprintf("%s: %v", strings.ToUpper(server.String()), err))
		}
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("所有支持的服务器尝试绑定失败，请检查ID是否正确\n%s", strings.Join(failures, "\n"))
	}
	return results, nil
}
