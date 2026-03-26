package userdata

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/userbinding"
	"haruki-cloud/database/pjsk/userdefaultbinding"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/drawing"
	sekaiapi "haruki-cloud/utils/sekai"
)

const GlobalDefaultBindingScope = "default"

var ErrBindingServiceUnavailable = errors.New("pjsk: binding service is not configured")

type IdentityResolver interface {
	ResolveOrCreate(ctx context.Context, platform, userID string) (int, error)
}

type ProfileValidator interface {
	GetUserProfile(server, userID string) (*sekaiapi.GetAnotherProfileResponse, error)
}

type FastVerificationProvider interface {
	GetToolboxUserFastVerificationGameAccountBindings(platform, platformUserID string) ([]sekaiapi.UserGameBinding, error)
}

type ProfileBGStorage interface {
	SaveProfileBackground(ctx context.Context, server string, bindingID int, imageURL string) (*drawing.ProfileBgSettings, error)
	DeleteProfileBackground(ctx context.Context, settings *drawing.ProfileBgSettings) error
}

type BindingService struct {
	pjskDB       *pjskdb.Client
	identity     IdentityResolver
	validator    ProfileValidator
	fastVerifier FastVerificationProvider
	bgStorage    ProfileBGStorage
}

type BindResult struct {
	Server              string
	UserID              string
	UserName            string
	AlreadyBound        bool
	SetGlobalDefault    bool
	SetServerDefault    bool
	MultipleServerMatch bool
}

type DefaultScope string

const (
	DefaultScopeGlobal DefaultScope = "global"
	DefaultScopeServer DefaultScope = "server"
)

type BindingListItem struct {
	Index           int
	BindingID       int
	Server          string
	UserID          string
	Visible         bool
	SuiteVisible    bool
	MySekaiVisible  bool
	Verified        bool
	Bg              *drawing.ProfileBgSettings
	IsGlobalDefault bool
	IsServerDefault bool
}

type UnbindResult struct {
	Removed          BindingListItem
	ReassignedGlobal *BindingListItem
	ReassignedServer *BindingListItem
}

type DefaultBindingResult struct {
	Scope   DefaultScope
	Server  string
	Binding BindingListItem
}

type profileProbe struct {
	Server   string
	UserID   string
	UserName string
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
			SetVisible(true).
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

func (s *BindingService) Unbind(ctx context.Context, platform, platformUserID, selector string) (*UnbindResult, error) {
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
	target, err := selectBinding(items, selector)
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

func (s *BindingService) SetDefault(ctx context.Context, platform, platformUserID, selector, serverScope string) (*DefaultBindingResult, error) {
	return s.updateDefault(ctx, platform, platformUserID, selector, serverScope, false)
}

func (s *BindingService) ClearDefault(ctx context.Context, platform, platformUserID, selector, serverScope string) (*DefaultBindingResult, error) {
	return s.updateDefault(ctx, platform, platformUserID, selector, serverScope, true)
}

func (s *BindingService) updateDefault(ctx context.Context, platform, platformUserID, selector, serverScope string, clear bool) (*DefaultBindingResult, error) {
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
	target, err := selectBinding(items, selector)
	if err != nil {
		return nil, err
	}

	scope, scopeLabel, err := normalizeDefaultScope(serverScope)
	if err != nil {
		return nil, err
	}
	if scope != GlobalDefaultBindingScope && target.Server != scope {
		return nil, fmt.Errorf("所选账号不属于%s服", strings.ToUpper(scope))
	}

	existing, err := s.pjskDB.UserDefaultBinding.Query().
		Where(
			userdefaultbinding.HarukiUserID(harukiUserID),
			userdefaultbinding.Server(scope),
		).
		Only(ctx)
	if err != nil && !pjskdb.IsNotFound(err) {
		return nil, err
	}

	if clear {
		if pjskdb.IsNotFound(err) {
			return nil, fmt.Errorf("你当前没有设置%s默认绑定", scopeLabel)
		}
		if existing.BindingID != target.BindingID {
			return nil, fmt.Errorf("所选账号不是你当前的%s默认绑定", scopeLabel)
		}
		if err := s.pjskDB.UserDefaultBinding.DeleteOneID(existing.ID).Exec(ctx); err != nil {
			return nil, err
		}
		return &DefaultBindingResult{
			Scope:   defaultScopeType(scope),
			Server:  scope,
			Binding: target,
		}, nil
	}

	tx, err := s.pjskDB.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err := upsertDefaultBindingTx(ctx, tx, harukiUserID, scope, target.BindingID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil

	if scope == GlobalDefaultBindingScope {
		target.IsGlobalDefault = true
	} else {
		target.IsServerDefault = true
	}
	return &DefaultBindingResult{
		Scope:   defaultScopeType(scope),
		Server:  scope,
		Binding: target,
	}, nil
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

func buildBindingList(bindings []*pjskdb.UserBinding, defaults []*pjskdb.UserDefaultBinding) []BindingListItem {
	if len(bindings) == 0 {
		return nil
	}

	globalDefaultID := 0
	serverDefaultByServer := make(map[string]int, len(defaults))
	for _, item := range defaults {
		if item.Server == GlobalDefaultBindingScope {
			globalDefaultID = item.BindingID
			continue
		}
		serverDefaultByServer[item.Server] = item.BindingID
	}

	list := make([]BindingListItem, 0, len(bindings))
	for _, binding := range bindings {
		list = append(list, BindingListItem{
			BindingID:       binding.ID,
			Server:          binding.Server,
			UserID:          binding.UserID,
			Visible:         binding.Visible,
			SuiteVisible:    binding.SuiteVisible,
			MySekaiVisible:  binding.MysekaiVisible,
			Verified:        binding.Verified,
			Bg:              cloneProfileBGSettings(binding.Bg),
			IsGlobalDefault: binding.ID == globalDefaultID,
			IsServerDefault: binding.ID == serverDefaultByServer[binding.Server],
		})
	}

	slices.SortFunc(list, func(a, b BindingListItem) int {
		if cmp := compareNumericString(a.UserID, b.UserID); cmp != 0 {
			return cmp
		}
		if a.Server < b.Server {
			return -1
		}
		if a.Server > b.Server {
			return 1
		}
		return a.BindingID - b.BindingID
	})

	for i := range list {
		list[i].Index = i + 1
	}
	return list
}

func selectBinding(items []BindingListItem, selector string) (BindingListItem, error) {
	if len(items) == 0 {
		return BindingListItem{}, fmt.Errorf("你还没有绑定任何PJSK账号")
	}
	selector = normalizeUID(selector)
	if selector == "" {
		return BindingListItem{}, fmt.Errorf("请提供账号ID或u序号")
	}

	lower := strings.ToLower(selector)
	if strings.HasPrefix(lower, "u") {
		index, err := strconv.Atoi(strings.TrimSpace(lower[1:]))
		if err != nil || index <= 0 {
			return BindingListItem{}, fmt.Errorf("请提供正确的u序号，例如 u1")
		}
		if index > len(items) {
			return BindingListItem{}, fmt.Errorf("指定的账号序号超出范围，目前仅绑定了%d个账号", len(items))
		}
		return items[index-1], nil
	}

	var matched []BindingListItem
	for _, item := range items {
		if item.UserID == selector {
			matched = append(matched, item)
		}
	}
	switch len(matched) {
	case 0:
		return BindingListItem{}, fmt.Errorf("未找到绑定的账号ID %s", selector)
	case 1:
		return matched[0], nil
	default:
		return BindingListItem{}, fmt.Errorf("账号ID %s 在多个区服都已绑定，请改用 u序号 操作", selector)
	}
}

func ensureDefaultBindingTx(ctx context.Context, tx *pjskdb.Tx, harukiUserID int, scope string, bindingID int) (bool, error) {
	_, err := tx.UserDefaultBinding.Query().
		Where(
			userdefaultbinding.HarukiUserID(harukiUserID),
			userdefaultbinding.Server(scope),
		).
		Only(ctx)
	if err == nil {
		return false, nil
	}
	if !pjskdb.IsNotFound(err) {
		return false, err
	}
	if _, err := tx.UserDefaultBinding.Create().
		SetHarukiUserID(harukiUserID).
		SetServer(scope).
		SetBindingID(bindingID).
		Save(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func upsertDefaultBindingTx(ctx context.Context, tx *pjskdb.Tx, harukiUserID int, scope string, bindingID int) (*pjskdb.UserDefaultBinding, error) {
	existing, err := tx.UserDefaultBinding.Query().
		Where(
			userdefaultbinding.HarukiUserID(harukiUserID),
			userdefaultbinding.Server(scope),
		).
		Only(ctx)
	if err == nil {
		return tx.UserDefaultBinding.UpdateOneID(existing.ID).
			SetBindingID(bindingID).
			Save(ctx)
	}
	if !pjskdb.IsNotFound(err) {
		return nil, err
	}
	return tx.UserDefaultBinding.Create().
		SetHarukiUserID(harukiUserID).
		SetServer(scope).
		SetBindingID(bindingID).
		Save(ctx)
}

func hasDefaultScope(items []*pjskdb.UserDefaultBinding, scope string) bool {
	for _, item := range items {
		if item.Server == scope {
			return true
		}
	}
	return false
}

func normalizeDefaultScope(scope string) (string, string, error) {
	scope = strings.TrimSpace(strings.ToLower(scope))
	if scope == "" || scope == GlobalDefaultBindingScope {
		return GlobalDefaultBindingScope, "全局", nil
	}
	normalized := renderregion.Normalize(scope)
	if normalized.IsZero() {
		return "", "", fmt.Errorf("不支持的区服: %s", scope)
	}
	return normalized.String(), strings.ToUpper(normalized.String()), nil
}

func defaultScopeType(scope string) DefaultScope {
	if scope == GlobalDefaultBindingScope {
		return DefaultScopeGlobal
	}
	return DefaultScopeServer
}

func normalizeUID(value string) string {
	return strings.TrimSpace(value)
}

func isNumericUID(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func compareNumericString(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")
	if a == "" {
		a = "0"
	}
	if b == "" {
		b = "0"
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func (s *BindingService) SetBindingVisible(ctx context.Context, platform, platformUserID, server string, visible bool) (*BindingListItem, error) {
	binding, err := s.currentBindingEntity(ctx, platform, platformUserID, server)
	if err != nil {
		return nil, err
	}
	if _, err := s.pjskDB.UserBinding.UpdateOneID(binding.ID).
		SetVisible(visible).
		Save(ctx); err != nil {
		return nil, err
	}
	return s.bindingListItemByID(ctx, platform, platformUserID, binding.ID)
}

func (s *BindingService) SetBindingSuiteVisible(ctx context.Context, platform, platformUserID, server string, suiteVisible bool) (*BindingListItem, error) {
	binding, err := s.currentBindingEntity(ctx, platform, platformUserID, server)
	if err != nil {
		return nil, err
	}
	if _, err := s.pjskDB.UserBinding.UpdateOneID(binding.ID).
		SetSuiteVisible(suiteVisible).
		Save(ctx); err != nil {
		return nil, err
	}
	return s.bindingListItemByID(ctx, platform, platformUserID, binding.ID)
}

func (s *BindingService) SetBindingMySekaiVisible(ctx context.Context, platform, platformUserID, server string, mySekaiVisible bool) (*BindingListItem, error) {
	binding, err := s.currentBindingEntity(ctx, platform, platformUserID, server)
	if err != nil {
		return nil, err
	}
	if _, err := s.pjskDB.UserBinding.UpdateOneID(binding.ID).
		SetMysekaiVisible(mySekaiVisible).
		Save(ctx); err != nil {
		return nil, err
	}
	return s.bindingListItemByID(ctx, platform, platformUserID, binding.ID)
}

func (s *BindingService) VerifyCurrentBinding(ctx context.Context, platform, platformUserID, server string) (*BindingListItem, bool, error) {
	if s == nil || s.fastVerifier == nil {
		return nil, false, fmt.Errorf("pjsk: fast verification provider is not configured")
	}
	binding, err := s.currentBindingEntity(ctx, platform, platformUserID, server)
	if err != nil {
		return nil, false, err
	}
	if binding.Verified {
		item, itemErr := s.bindingListItemByID(ctx, platform, platformUserID, binding.ID)
		return item, true, itemErr
	}

	records, err := s.fastVerifier.GetToolboxUserFastVerificationGameAccountBindings(platform, platformUserID)
	if err != nil {
		return nil, false, err
	}

	matched := false
	for _, record := range records {
		if strings.EqualFold(strings.TrimSpace(record.Server), binding.Server) &&
			strings.TrimSpace(record.GameUserID) == binding.UserID {
			matched = true
			break
		}
	}
	if !matched {
		return nil, false, fmt.Errorf("当前%s服绑定账号未出现在快速验证列表中", strings.ToUpper(binding.Server))
	}

	if _, err := s.pjskDB.UserBinding.UpdateOneID(binding.ID).
		SetVerified(true).
		Save(ctx); err != nil {
		return nil, false, err
	}
	item, err := s.bindingListItemByID(ctx, platform, platformUserID, binding.ID)
	return item, false, err
}

func (s *BindingService) ListVerifiedBindings(ctx context.Context, platform, platformUserID, server string) ([]BindingListItem, error) {
	if err := s.requireReady(platform, platformUserID); err != nil {
		return nil, err
	}
	server = strings.TrimSpace(strings.ToLower(server))
	if server == "" {
		return nil, fmt.Errorf("请提供区服")
	}
	items, err := s.List(ctx, platform, platformUserID)
	if err != nil {
		return nil, err
	}
	var verified []BindingListItem
	for _, item := range items {
		if !strings.EqualFold(item.Server, server) || !item.Verified {
			continue
		}
		verified = append(verified, item)
	}
	return verified, nil
}

func (s *BindingService) SetCurrentBindingProfileBG(ctx context.Context, platform, platformUserID, server, imageURL string) (*BindingListItem, error) {
	if s == nil || s.bgStorage == nil {
		return nil, fmt.Errorf("pjsk: profile background storage is not configured")
	}
	binding, err := s.currentBindingEntity(ctx, platform, platformUserID, server)
	if err != nil {
		return nil, err
	}
	if !binding.Verified {
		return nil, fmt.Errorf("当前%s服绑定账号尚未验证，无法设置个人信息背景", strings.ToUpper(binding.Server))
	}
	settings, err := s.bgStorage.SaveProfileBackground(ctx, binding.Server, binding.ID, imageURL)
	if err != nil {
		return nil, err
	}
	if _, err := s.pjskDB.UserBinding.UpdateOneID(binding.ID).
		SetBg(settings).
		Save(ctx); err != nil {
		return nil, err
	}
	return s.bindingListItemByID(ctx, platform, platformUserID, binding.ID)
}

func (s *BindingService) ClearCurrentBindingProfileBG(ctx context.Context, platform, platformUserID, server string) (*BindingListItem, error) {
	binding, err := s.currentBindingEntity(ctx, platform, platformUserID, server)
	if err != nil {
		return nil, err
	}
	if !binding.Verified {
		return nil, fmt.Errorf("当前%s服绑定账号尚未验证，无法清除个人信息背景", strings.ToUpper(binding.Server))
	}
	if s.bgStorage != nil {
		if err := s.bgStorage.DeleteProfileBackground(ctx, binding.Bg); err != nil {
			return nil, err
		}
	}
	if _, err := s.pjskDB.UserBinding.UpdateOneID(binding.ID).
		ClearBg().
		Save(ctx); err != nil {
		return nil, err
	}
	return s.bindingListItemByID(ctx, platform, platformUserID, binding.ID)
}

func (s *BindingService) AdjustCurrentBindingProfileBG(ctx context.Context, platform, platformUserID, server string, blur, alpha *int, vertical *bool) (*BindingListItem, error) {
	binding, err := s.currentBindingEntity(ctx, platform, platformUserID, server)
	if err != nil {
		return nil, err
	}
	if !binding.Verified {
		return nil, fmt.Errorf("当前%s服绑定账号尚未验证，无法调整个人信息背景", strings.ToUpper(binding.Server))
	}
	if binding.Bg == nil || binding.Bg.ImgPath == nil || strings.TrimSpace(*binding.Bg.ImgPath) == "" {
		return nil, fmt.Errorf("当前%s服还没有自定义个人信息背景", strings.ToUpper(binding.Server))
	}

	settings := cloneProfileBGSettings(binding.Bg)
	if blur != nil {
		settings.Blur = *blur
	}
	if alpha != nil {
		settings.Alpha = *alpha
	}
	if vertical != nil {
		settings.Vertical = *vertical
	}

	if _, err := s.pjskDB.UserBinding.UpdateOneID(binding.ID).
		SetBg(settings).
		Save(ctx); err != nil {
		return nil, err
	}
	return s.bindingListItemByID(ctx, platform, platformUserID, binding.ID)
}

func (s *BindingService) currentBindingEntity(ctx context.Context, platform, platformUserID, server string) (*pjskdb.UserBinding, error) {
	if err := s.requireReady(platform, platformUserID); err != nil {
		return nil, err
	}
	_, resolved, err := s.ResolveUserBinding(ctx, platform, platformUserID, server)
	if err != nil {
		return nil, err
	}
	return s.pjskDB.UserBinding.Get(ctx, resolved.BindingID)
}

func (s *BindingService) bindingListItemByID(ctx context.Context, platform, platformUserID string, bindingID int) (*BindingListItem, error) {
	items, err := s.List(ctx, platform, platformUserID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.BindingID == bindingID {
			cloned := item
			return &cloned, nil
		}
	}
	return nil, fmt.Errorf("未找到绑定记录 %d", bindingID)
}

var AllBindingServers = []renderregion.Value{
	renderregion.JP,
	renderregion.CN,
	renderregion.TW,
	renderregion.KR,
	renderregion.EN,
}

// ResolveUserBinding resolves a platform user's active PJSK binding for the given
// server by combining identity resolution and binding resolution.
// Returns the haruki user ID and the resolved binding.
func (s *BindingService) ResolveUserBinding(ctx context.Context, platform, platformUserID, server string) (int, *ResolvedBinding, error) {
	harukiUserID, err := s.identity.ResolveOrCreate(ctx, platform, platformUserID)
	if err != nil {
		return 0, nil, err
	}
	resolver := NewBindingResolver(s.pjskDB)
	binding, err := resolver.Resolve(ctx, harukiUserID, server)
	if err != nil {
		return harukiUserID, nil, err
	}
	return harukiUserID, binding, nil
}
