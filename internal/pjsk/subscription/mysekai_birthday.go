package subscription

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/config"
	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/mysekaibirthdaysubscription"
	"haruki-cloud/database/pjsk/mysekaibirthdaysubscriptionevent"
	"haruki-cloud/internal/pjsk/accountdata"
	renderregion "haruki-cloud/internal/pjsk/region"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"

	"entgo.io/ent/dialect/sql"
)

const (
	DefaultBirthdayMonitorMinutes = 90
	MaxBirthdayMonitorMinutes     = 120
	EmptyBirthdayMonitorMessage   = "本次生日材料更新未发现你订阅的材料。"
)

type BirthdayMaterial struct {
	Name        string
	ResourceID  int
	ResourceKey string
	Aliases     []string
}

var BirthdayMaterials = []BirthdayMaterial{
	{
		Name:        "diamond",
		ResourceID:  12,
		ResourceKey: "mysekai_material_12",
		Aliases:     []string{"钻石", "ダイヤモンド", "diamond"},
	},
	{
		Name:        "yuugiri",
		ResourceID:  5,
		ResourceKey: "mysekai_material_5",
		Aliases:     []string{"夕桐", "yuugiri", "yugiri"},
	},
	{
		Name:        "clover",
		ResourceID:  20,
		ResourceKey: "mysekai_material_20",
		Aliases:     []string{"四叶草", "四葉草", "四葉のクローバー", "clover"},
	},
}

var materialByName = func() map[string]BirthdayMaterial {
	result := make(map[string]BirthdayMaterial, len(BirthdayMaterials))
	for _, material := range BirthdayMaterials {
		result[material.Name] = material
	}
	return result
}()

var hmesCloseHTTPClient = &http.Client{Timeout: 5 * time.Second}

type BirthdayMonitorAction struct {
	Type           string `json:"type" msgpack:"type"`
	SubscriptionID string `json:"subscription_id" msgpack:"subscription_id"`
	Endpoint       string `json:"endpoint" msgpack:"endpoint"`
	Token          string `json:"token" msgpack:"token"`
	ExpiresAt      int64  `json:"expires_at" msgpack:"expires_at"`
}

type BirthdayMonitorResult struct {
	Subscription        *pjskdb.MysekaiBirthdaySubscription
	Duration            time.Duration
	Materials           []string
	Token               string
	SubscriptionVersion string
	NotifyEmpty         bool
}

type ActiveUploadSubscription struct {
	Active         bool     `json:"active"`
	SubscriptionID string   `json:"subscription_id,omitempty"`
	Materials      []string `json:"materials,omitempty"`
	MaterialIDs    []int    `json:"material_ids,omitempty"`
	NotifyEmpty    bool     `json:"notify_empty"`
}

type StoredBirthdayEvent struct {
	EventID        string
	SubscriptionID string
	EmptyResult    bool
}

type BirthdayEventPayload struct {
	SubscriptionID     string
	Region             string
	UID                string
	UploadTime         int64
	MatchedMaterialIDs []int
	EmptyResult        bool
	FilteredPayload    []byte
}

type PendingBirthdayEvent struct {
	EventID        string `json:"event_id"`
	SubscriptionID string `json:"subscription_id"`
	EmptyResult    bool   `json:"empty_result"`
}

type TokenValidationResult struct {
	Valid               bool
	Subscription        *pjskdb.MysekaiBirthdaySubscription
	SubscriptionVersion string
	PendingEvents       []PendingBirthdayEvent
}

type ClientBirthdayEvent struct {
	EventID         string
	SubscriptionID  string
	Region          string
	UID             string
	PlatformUserID  string
	EmptyResult     bool
	FilteredPayload []byte
}

type BirthdayMonitorCommand struct {
	Cancel          bool
	Region          string
	RegionExplicit  bool
	Selector        string
	DurationMinutes int
	Materials       []string
}

type Service struct {
	db       *pjskdb.Client
	bindings *accountdata.BindingService
	toolbox  *sekaiapi.HarukiToolboxClient
}

func NewService(db *pjskdb.Client, bindings *accountdata.BindingService) *Service {
	return &Service{db: db, bindings: bindings}
}

func NewServiceWithToolbox(db *pjskdb.Client, bindings *accountdata.BindingService, toolbox *sekaiapi.HarukiToolboxClient) *Service {
	return &Service{db: db, bindings: bindings, toolbox: toolbox}
}

func (s *Service) Ready() bool {
	return s != nil && s.db != nil && s.bindings != nil && s.bindings.IsReady()
}

func (s *Service) CreateOrUpdate(
	ctx context.Context,
	platform string,
	platformUserID string,
	platformGroupID string,
	cloudBotID string,
	selfID string,
	server string,
	regionExplicit bool,
	message string,
	notifyEmpty bool,
) (*BirthdayMonitorResult, error) {
	if err := s.requireReady(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(platformGroupID) == "" {
		return nil, fmt.Errorf("生日材料监听只支持群聊")
	}
	if strings.TrimSpace(selfID) == "" {
		return nil, fmt.Errorf("缺少 OneBot self_id，请更新 Client")
	}

	command, err := ParseBirthdayMonitorCommand(message)
	if err != nil {
		return nil, err
	}
	if command.Cancel {
		return nil, fmt.Errorf("请使用取消监听接口")
	}
	if command.RegionExplicit {
		server = command.Region
		regionExplicit = true
	}

	binding, err := s.resolveBinding(ctx, platform, platformUserID, server, regionExplicit, command.Selector)
	if err != nil {
		return nil, err
	}
	if binding == nil {
		return nil, fmt.Errorf("未找到可监听的绑定账号")
	}
	if !binding.Verified {
		return nil, fmt.Errorf("该账号尚未验证，请先在工具箱验证账号再发送/pjsk验证后才可使用")
	}

	version, token, err := randomVersionedToken()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	duration := time.Duration(command.DurationMinutes) * time.Minute
	expiresAt := now.Add(duration)
	region := normalizeRegion(binding.Server)
	uid := strings.TrimSpace(binding.PJSKUserID)

	existing, err := s.db.MysekaiBirthdaySubscription.Query().
		Where(
			mysekaibirthdaysubscription.Region(region),
			mysekaibirthdaysubscription.UID(uid),
		).
		Only(ctx)
	switch {
	case err == nil:
		if err := s.ackPendingEvents(ctx, existing.ID, now); err != nil {
			return nil, err
		}
		if err := s.syncBirthdayMonitor(ctx, existing.ID, version, region, uid, command.Materials, expiresAt, notifyEmpty); err != nil {
			return nil, err
		}
		updated, err := s.db.MysekaiBirthdaySubscription.UpdateOneID(existing.ID).
			SetPlatform(strings.TrimSpace(platform)).
			SetPlatformUserID(strings.TrimSpace(platformUserID)).
			SetPlatformGroupID(strings.TrimSpace(platformGroupID)).
			SetCloudBotID(strings.TrimSpace(cloudBotID)).
			SetSelfID(strings.TrimSpace(selfID)).
			SetMaterials(command.Materials).
			SetToken(token).
			SetActive(true).
			SetExpiresAt(expiresAt).
			ClearCancelledAt().
			Save(ctx)
		if err != nil {
			return nil, err
		}
		return &BirthdayMonitorResult{
			Subscription:        updated,
			Duration:            duration,
			Materials:           command.Materials,
			Token:               token,
			SubscriptionVersion: version,
			NotifyEmpty:         notifyEmpty,
		}, nil
	case pjskdb.IsNotFound(err):
		created, err := s.db.MysekaiBirthdaySubscription.Create().
			SetRegion(region).
			SetUID(uid).
			SetPlatform(strings.TrimSpace(platform)).
			SetPlatformUserID(strings.TrimSpace(platformUserID)).
			SetPlatformGroupID(strings.TrimSpace(platformGroupID)).
			SetCloudBotID(strings.TrimSpace(cloudBotID)).
			SetSelfID(strings.TrimSpace(selfID)).
			SetMaterials(command.Materials).
			SetToken(token).
			SetActive(false).
			SetExpiresAt(expiresAt).
			Save(ctx)
		if err != nil {
			return nil, err
		}
		if err := s.syncBirthdayMonitor(ctx, created.ID, version, region, uid, command.Materials, expiresAt, notifyEmpty); err != nil {
			_, _ = s.db.MysekaiBirthdaySubscription.UpdateOneID(created.ID).
				SetActive(false).
				SetCancelledAt(now).
				Save(ctx)
			return nil, err
		}
		activated, err := s.db.MysekaiBirthdaySubscription.UpdateOneID(created.ID).
			SetActive(true).
			ClearCancelledAt().
			Save(ctx)
		if err != nil {
			_ = s.deleteBirthdayMonitor(ctx, created.ID, version)
			return nil, err
		}
		return &BirthdayMonitorResult{
			Subscription:        activated,
			Duration:            duration,
			Materials:           command.Materials,
			Token:               token,
			SubscriptionVersion: version,
			NotifyEmpty:         notifyEmpty,
		}, nil
	default:
		return nil, err
	}
}

func (s *Service) Cancel(
	ctx context.Context,
	platform string,
	platformUserID string,
	platformGroupID string,
	cloudBotID string,
	selfID string,
	server string,
	regionExplicit bool,
	message string,
) (*pjskdb.MysekaiBirthdaySubscription, error) {
	if err := s.requireReady(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(platformGroupID) == "" {
		return nil, fmt.Errorf("生日材料监听只支持群聊")
	}
	command, err := ParseBirthdayMonitorCommand(message)
	if err != nil {
		return nil, err
	}
	if !command.Cancel {
		return nil, fmt.Errorf("请使用监听接口")
	}
	if command.RegionExplicit {
		server = command.Region
		regionExplicit = true
	}
	binding, err := s.resolveBinding(ctx, platform, platformUserID, server, regionExplicit, command.Selector)
	if err != nil {
		return nil, err
	}
	region := normalizeRegion(binding.Server)
	uid := strings.TrimSpace(binding.PJSKUserID)
	now := time.Now()

	sub, err := s.db.MysekaiBirthdaySubscription.Query().
		Where(
			mysekaibirthdaysubscription.Region(region),
			mysekaibirthdaysubscription.UID(uid),
			mysekaibirthdaysubscription.Active(true),
			mysekaibirthdaysubscription.ExpiresAtGT(now),
		).
		Only(ctx)
	if pjskdb.IsNotFound(err) {
		return nil, fmt.Errorf("当前账号没有活跃的生日材料监听")
	}
	if err != nil {
		return nil, err
	}
	if sub.Platform != strings.TrimSpace(platform) ||
		sub.PlatformUserID != strings.TrimSpace(platformUserID) ||
		sub.PlatformGroupID != strings.TrimSpace(platformGroupID) ||
		sub.CloudBotID != strings.TrimSpace(cloudBotID) ||
		sub.SelfID != strings.TrimSpace(selfID) {
		return nil, fmt.Errorf("当前账号没有由你创建的活跃生日材料监听")
	}
	if err := s.deleteBirthdayMonitor(ctx, sub.ID, tokenVersion(sub.Token)); err != nil {
		return nil, err
	}
	if err := s.ackPendingEvents(ctx, sub.ID, now); err != nil {
		return nil, err
	}

	cancelled, err := s.db.MysekaiBirthdaySubscription.UpdateOneID(sub.ID).
		SetActive(false).
		SetCancelledAt(now).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	_ = s.closeBirthdayMonitorConnection(ctx, cancelled.ID, tokenVersion(sub.Token))
	return cancelled, nil
}

func (s *Service) ActiveForUpload(ctx context.Context, region string, uid string) (*ActiveUploadSubscription, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	sub, err := s.activeSubscriptionForUID(ctx, normalizeRegion(region), strings.TrimSpace(uid))
	if pjskdb.IsNotFound(err) {
		return &ActiveUploadSubscription{Active: false}, nil
	}
	if err != nil {
		return nil, err
	}
	return &ActiveUploadSubscription{
		Active:         true,
		SubscriptionID: strconv.Itoa(sub.ID),
		Materials:      slices.Clone(sub.Materials),
		MaterialIDs:    MaterialIDs(sub.Materials),
		NotifyEmpty:    true,
	}, nil
}

func (s *Service) syncBirthdayMonitor(ctx context.Context, subscriptionID int, version string, region string, uid string, materials []string, expiresAt time.Time, notifyEmpty bool) error {
	if s == nil || s.toolbox == nil {
		return fmt.Errorf("toolbox 监听同步服务未配置，订阅失败，请稍后重试")
	}
	if err := s.toolbox.UpsertMysekaiBirthdayMonitor(ctx, sekaiapi.MysekaiBirthdayMonitorUpsertRequest{
		SubscriptionID:      strconv.Itoa(subscriptionID),
		SubscriptionVersion: version,
		Region:              region,
		UID:                 uid,
		Materials:           slices.Clone(materials),
		MaterialIDs:         MaterialIDs(materials),
		ExpiresAt:           expiresAt.Unix(),
		NotifyEmpty:         notifyEmpty,
	}); err != nil {
		return fmt.Errorf("同步 Toolbox 监听失败，订阅未生效，请稍后重试: %w", err)
	}
	return nil
}

func (s *Service) deleteBirthdayMonitor(ctx context.Context, subscriptionID int, version string) error {
	if s == nil || s.toolbox == nil {
		return nil
	}
	if err := s.toolbox.DeleteMysekaiBirthdayMonitor(ctx, strconv.Itoa(subscriptionID), version); err != nil {
		return fmt.Errorf("清理 Toolbox 监听失败，请稍后重试: %w", err)
	}
	return nil
}

func (s *Service) closeBirthdayMonitorConnection(ctx context.Context, subscriptionID int, version string) error {
	base := strings.TrimRight(strings.TrimSpace(config.Cfg.HMES.InternalBaseURL), "/")
	version = strings.TrimSpace(version)
	if base == "" || subscriptionID <= 0 || version == "" {
		return nil
	}
	u, err := url.Parse(fmt.Sprintf("%s/internal/subscriptions/%d/close", base, subscriptionID))
	if err != nil {
		return err
	}
	query := u.Query()
	query.Set("subscription_version", version)
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
	if err != nil {
		return err
	}
	if token := strings.TrimSpace(config.Cfg.HMES.InternalToken); token != "" {
		req.Header.Set("Authorization", token)
	}
	if userAgent := strings.TrimSpace(config.Cfg.HMES.UserAgent); userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	} else {
		req.Header.Set("User-Agent", "Haruki-Cloud")
	}
	resp, err := hmesCloseHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("hmes close returned status %d", resp.StatusCode)
	}
	return nil
}

func (s *Service) StoreEvent(ctx context.Context, payload BirthdayEventPayload) (*StoredBirthdayEvent, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	subscriptionID, err := strconv.Atoi(strings.TrimSpace(payload.SubscriptionID))
	if err != nil || subscriptionID <= 0 {
		return nil, fmt.Errorf("invalid subscription_id")
	}
	sub, err := s.db.MysekaiBirthdaySubscription.Query().
		Where(
			mysekaibirthdaysubscription.ID(subscriptionID),
			mysekaibirthdaysubscription.Active(true),
			mysekaibirthdaysubscription.ExpiresAtGT(time.Now()),
		).
		Only(ctx)
	if err != nil {
		return nil, err
	}
	if sub.Region != normalizeRegion(payload.Region) || sub.UID != strings.TrimSpace(payload.UID) {
		return nil, fmt.Errorf("event target does not match subscription")
	}

	event, err := s.db.MysekaiBirthdaySubscriptionEvent.Create().
		SetSubscriptionID(sub.ID).
		SetRegion(sub.Region).
		SetUID(sub.UID).
		SetPlatform(sub.Platform).
		SetPlatformUserID(sub.PlatformUserID).
		SetPlatformGroupID(sub.PlatformGroupID).
		SetCloudBotID(sub.CloudBotID).
		SetSelfID(sub.SelfID).
		SetMatchedMaterialIds(normalizeMaterialIDs(payload.MatchedMaterialIDs)).
		SetEmptyResult(payload.EmptyResult).
		SetFilteredPayload(slices.Clone(payload.FilteredPayload)).
		SetUploadTime(normalizeUploadTime(payload.UploadTime)).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	return &StoredBirthdayEvent{
		EventID:        strconv.Itoa(event.ID),
		SubscriptionID: strconv.Itoa(sub.ID),
		EmptyResult:    event.EmptyResult,
	}, nil
}

func (s *Service) ValidateToken(ctx context.Context, subscriptionID string, subscriptionVersion string, token string) (*TokenValidationResult, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	subID, err := strconv.Atoi(strings.TrimSpace(subscriptionID))
	if err != nil || subID <= 0 || strings.TrimSpace(token) == "" {
		return &TokenValidationResult{Valid: false}, nil
	}
	sub, err := s.db.MysekaiBirthdaySubscription.Query().
		Where(
			mysekaibirthdaysubscription.ID(subID),
			mysekaibirthdaysubscription.Token(strings.TrimSpace(token)),
			mysekaibirthdaysubscription.Active(true),
			mysekaibirthdaysubscription.ExpiresAtGT(time.Now()),
		).
		Only(ctx)
	if pjskdb.IsNotFound(err) {
		return &TokenValidationResult{Valid: false}, nil
	}
	if err != nil {
		return nil, err
	}
	version := tokenVersion(sub.Token)
	if wanted := strings.TrimSpace(subscriptionVersion); wanted != "" && wanted != version {
		return &TokenValidationResult{Valid: false}, nil
	}
	pending, err := s.pendingEventsForSubscription(ctx, sub)
	if err != nil {
		return nil, err
	}
	return &TokenValidationResult{Valid: true, Subscription: sub, SubscriptionVersion: version, PendingEvents: pending}, nil
}

func (s *Service) PendingEvents(ctx context.Context, subscriptionID int) ([]PendingBirthdayEvent, error) {
	events, err := s.db.MysekaiBirthdaySubscriptionEvent.Query().
		Where(
			mysekaibirthdaysubscriptionevent.SubscriptionID(subscriptionID),
			mysekaibirthdaysubscriptionevent.AcknowledgedAtIsNil(),
		).
		Order(mysekaibirthdaysubscriptionevent.ByCreatedAt(sql.OrderAsc())).
		All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]PendingBirthdayEvent, 0, len(events))
	for _, event := range events {
		result = append(result, PendingBirthdayEvent{
			EventID:        strconv.Itoa(event.ID),
			SubscriptionID: strconv.Itoa(event.SubscriptionID),
			EmptyResult:    event.EmptyResult,
		})
	}
	return result, nil
}

func (s *Service) EventForClient(ctx context.Context, eventID string, subscriptionID string, subscriptionVersion string, token string, cloudBotID string, platformGroupID string, platformUserID string, selfID string) (*ClientBirthdayEvent, error) {
	validation, err := s.ValidateToken(ctx, subscriptionID, subscriptionVersion, token)
	if err != nil {
		return nil, err
	}
	if validation == nil || !validation.Valid || validation.Subscription == nil {
		return nil, fmt.Errorf("invalid subscription token")
	}
	sub := validation.Subscription
	if sub.CloudBotID != strings.TrimSpace(cloudBotID) ||
		sub.PlatformGroupID != strings.TrimSpace(platformGroupID) ||
		sub.PlatformUserID != strings.TrimSpace(platformUserID) ||
		sub.SelfID != strings.TrimSpace(selfID) {
		return nil, fmt.Errorf("subscription context mismatch")
	}
	if strings.TrimSpace(subscriptionVersion) != "" && s.toolbox != nil {
		event, err := s.toolbox.GetMysekaiBirthdayEvent(ctx, sekaiapi.MysekaiBirthdayEventLookupRequest{
			EventID:             strings.TrimSpace(eventID),
			SubscriptionID:      strconv.Itoa(sub.ID),
			SubscriptionVersion: validation.SubscriptionVersion,
		})
		if err != nil {
			return nil, err
		}
		return &ClientBirthdayEvent{
			EventID:         event.EventID,
			SubscriptionID:  event.SubscriptionID,
			Region:          event.Region,
			UID:             event.UID,
			PlatformUserID:  sub.PlatformUserID,
			EmptyResult:     event.EmptyResult,
			FilteredPayload: slices.Clone(event.FilteredPayload),
		}, nil
	}
	id, err := strconv.Atoi(strings.TrimSpace(eventID))
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("invalid event_id")
	}
	event, err := s.db.MysekaiBirthdaySubscriptionEvent.Query().
		Where(
			mysekaibirthdaysubscriptionevent.ID(id),
			mysekaibirthdaysubscriptionevent.SubscriptionID(sub.ID),
		).
		Only(ctx)
	if err != nil {
		return nil, err
	}
	if event.CloudBotID != strings.TrimSpace(cloudBotID) ||
		event.PlatformGroupID != strings.TrimSpace(platformGroupID) ||
		event.PlatformUserID != strings.TrimSpace(platformUserID) ||
		event.SelfID != strings.TrimSpace(selfID) {
		return nil, fmt.Errorf("event context mismatch")
	}
	return &ClientBirthdayEvent{
		EventID:         strconv.Itoa(event.ID),
		SubscriptionID:  strconv.Itoa(event.SubscriptionID),
		Region:          event.Region,
		UID:             event.UID,
		PlatformUserID:  event.PlatformUserID,
		EmptyResult:     event.EmptyResult,
		FilteredPayload: slices.Clone(event.FilteredPayload),
	}, nil
}

func (s *Service) AckEvent(ctx context.Context, eventID string, subscriptionID string, subscriptionVersion string, token string, cloudBotID string, platformGroupID string, platformUserID string, selfID string) error {
	event, err := s.EventForClient(ctx, eventID, subscriptionID, subscriptionVersion, token, cloudBotID, platformGroupID, platformUserID, selfID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(subscriptionVersion) != "" && s.toolbox != nil {
		return s.toolbox.AckMysekaiBirthdayEvent(ctx, sekaiapi.MysekaiBirthdayEventLookupRequest{
			EventID:             event.EventID,
			SubscriptionID:      event.SubscriptionID,
			SubscriptionVersion: strings.TrimSpace(subscriptionVersion),
		})
	}
	id, err := strconv.Atoi(event.EventID)
	if err != nil || id <= 0 {
		return fmt.Errorf("invalid event_id")
	}
	return s.db.MysekaiBirthdaySubscriptionEvent.UpdateOneID(id).
		SetAcknowledgedAt(time.Now()).
		Exec(ctx)
}

func (s *Service) activeSubscriptionForUID(ctx context.Context, region string, uid string) (*pjskdb.MysekaiBirthdaySubscription, error) {
	return s.db.MysekaiBirthdaySubscription.Query().
		Where(
			mysekaibirthdaysubscription.Region(region),
			mysekaibirthdaysubscription.UID(uid),
			mysekaibirthdaysubscription.Active(true),
			mysekaibirthdaysubscription.ExpiresAtGT(time.Now()),
		).
		Only(ctx)
}

func (s *Service) pendingEventsForSubscription(ctx context.Context, sub *pjskdb.MysekaiBirthdaySubscription) ([]PendingBirthdayEvent, error) {
	if sub == nil {
		return nil, nil
	}
	events, err := s.db.MysekaiBirthdaySubscriptionEvent.Query().
		Where(
			mysekaibirthdaysubscriptionevent.SubscriptionID(sub.ID),
			mysekaibirthdaysubscriptionevent.Platform(sub.Platform),
			mysekaibirthdaysubscriptionevent.PlatformUserID(sub.PlatformUserID),
			mysekaibirthdaysubscriptionevent.PlatformGroupID(sub.PlatformGroupID),
			mysekaibirthdaysubscriptionevent.CloudBotID(sub.CloudBotID),
			mysekaibirthdaysubscriptionevent.SelfID(sub.SelfID),
			mysekaibirthdaysubscriptionevent.AcknowledgedAtIsNil(),
		).
		Order(mysekaibirthdaysubscriptionevent.ByCreatedAt(sql.OrderAsc())).
		All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]PendingBirthdayEvent, 0, len(events))
	for _, event := range events {
		result = append(result, PendingBirthdayEvent{
			EventID:        strconv.Itoa(event.ID),
			SubscriptionID: strconv.Itoa(event.SubscriptionID),
			EmptyResult:    event.EmptyResult,
		})
	}
	return result, nil
}

func (s *Service) ackPendingEvents(ctx context.Context, subscriptionID int, acknowledgedAt time.Time) error {
	if subscriptionID <= 0 {
		return nil
	}
	return s.db.MysekaiBirthdaySubscriptionEvent.Update().
		Where(
			mysekaibirthdaysubscriptionevent.SubscriptionID(subscriptionID),
			mysekaibirthdaysubscriptionevent.AcknowledgedAtIsNil(),
		).
		SetAcknowledgedAt(acknowledgedAt).
		Exec(ctx)
}

func (s *Service) resolveBinding(ctx context.Context, platform string, platformUserID string, server string, regionExplicit bool, selector string) (*accountdata.ResolvedBinding, error) {
	platform = strings.TrimSpace(platform)
	platformUserID = strings.TrimSpace(platformUserID)
	selector = strings.TrimSpace(selector)
	region := normalizeRegion(server)
	var binding *accountdata.ResolvedBinding
	var err error
	if selector != "" {
		_, binding, err = s.bindings.ResolveUserBindingBySelector(ctx, platform, platformUserID, selectorBindingServer(region, regionExplicit), selector)
	} else if !regionExplicit {
		_, binding, err = s.bindings.ResolveUserBinding(ctx, platform, platformUserID, accountdata.GlobalDefaultBindingScope)
		if err != nil {
			_, binding, err = s.bindings.ResolveUserBinding(ctx, platform, platformUserID, region)
		}
	} else {
		_, binding, err = s.bindings.ResolveUserBinding(ctx, platform, platformUserID, region)
	}
	if err != nil {
		if errors.Is(err, accountdata.ErrNoBinding) {
			return nil, fmt.Errorf("未找到可监听的绑定账号，请先绑定并验证账号")
		}
		return nil, err
	}
	return binding, nil
}

func (s *Service) requireReady() error {
	if !s.Ready() {
		return fmt.Errorf("生日材料监听服务未就绪")
	}
	return nil
}

func (s *Service) requireDB() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("生日材料监听数据库未就绪")
	}
	return nil
}

func ParseBirthdayMonitorCommand(message string) (BirthdayMonitorCommand, error) {
	trimmed := strings.TrimSpace(message)
	region, regionExplicit, normalizedCommand := stripBirthdayMonitorRegionPrefix(trimmed)
	commandText, cancel, ok := stripBirthdayMonitorCommand(normalizedCommand)
	if !ok {
		return BirthdayMonitorCommand{}, fmt.Errorf("未识别的生日材料监听命令")
	}
	fields := strings.Fields(commandText)
	result := BirthdayMonitorCommand{
		Cancel:          cancel,
		Region:          region,
		RegionExplicit:  regionExplicit,
		DurationMinutes: DefaultBirthdayMonitorMinutes,
	}
	enabled := map[string]bool{}
	sawMaterial := false
	for _, field := range fields {
		token := strings.TrimSpace(field)
		if token == "" {
			continue
		}
		lower := strings.ToLower(token)
		if isSelectorToken(lower) {
			result.Selector = lower
			continue
		}
		if minutes, ok := parseDurationToken(token); ok {
			if cancel {
				continue
			}
			if minutes <= 0 {
				return BirthdayMonitorCommand{}, fmt.Errorf("监听时长必须大于 0 分钟")
			}
			if minutes > MaxBirthdayMonitorMinutes {
				return BirthdayMonitorCommand{}, fmt.Errorf("监听时长不能超过 %d 分钟", MaxBirthdayMonitorMinutes)
			}
			result.DurationMinutes = minutes
			continue
		}
		if name, value, ok := parseMaterialToken(token); ok {
			if cancel {
				continue
			}
			sawMaterial = true
			enabled[name] = value
			continue
		}
		return BirthdayMonitorCommand{}, fmt.Errorf("无法识别参数：%s", token)
	}

	if cancel {
		return result, nil
	}
	if !sawMaterial {
		enabled["diamond"] = true
	}
	for _, material := range BirthdayMaterials {
		if enabled[material.Name] {
			result.Materials = append(result.Materials, material.Name)
		}
	}
	if len(result.Materials) == 0 {
		return BirthdayMonitorCommand{}, fmt.Errorf("至少需要开启一种监听材料")
	}
	return result, nil
}

func MaterialIDs(materials []string) []int {
	result := make([]int, 0, len(materials))
	seen := make(map[int]struct{}, len(materials))
	for _, name := range materials {
		material, ok := materialByName[strings.TrimSpace(name)]
		if !ok {
			continue
		}
		if _, exists := seen[material.ResourceID]; exists {
			continue
		}
		seen[material.ResourceID] = struct{}{}
		result = append(result, material.ResourceID)
	}
	return result
}

func MaterialNamesFromIDs(ids []int) []string {
	result := make([]string, 0, len(ids))
	for _, material := range BirthdayMaterials {
		if slices.Contains(ids, material.ResourceID) {
			result = append(result, material.Name)
		}
	}
	return result
}

func stripBirthdayMonitorCommand(message string) (string, bool, bool) {
	aliases := []struct {
		command string
		cancel  bool
	}{
		{"/mysekai birthday unmonitor", true},
		{"/烤森生日取消监听", true},
		{"/ms生日取消监听", true},
		{"/mysekai birthday monitor", false},
		{"/烤森生日监听", false},
		{"/ms生日监听", false},
	}
	lowerMessage := strings.ToLower(message)
	for _, alias := range aliases {
		lowerAlias := strings.ToLower(alias.command)
		if lowerMessage == lowerAlias {
			return "", alias.cancel, true
		}
		if strings.HasPrefix(lowerMessage, lowerAlias) {
			rest := strings.TrimSpace(message[len(alias.command):])
			return rest, alias.cancel, true
		}
	}
	return "", false, false
}

func stripBirthdayMonitorRegionPrefix(message string) (string, bool, string) {
	trimmed := strings.TrimSpace(message)
	if !strings.HasPrefix(trimmed, "/") {
		return "", false, trimmed
	}
	afterSlash := trimmed[1:]
	for _, region := range []renderregion.Value{
		renderregion.JP,
		renderregion.TW,
		renderregion.KR,
		renderregion.EN,
		renderregion.CN,
	} {
		prefix := region.String()
		if len(afterSlash) < len(prefix) || !strings.EqualFold(afterSlash[:len(prefix)], prefix) {
			continue
		}
		rest := afterSlash[len(prefix):]
		rest = strings.TrimLeftFunc(rest, func(r rune) bool {
			return r == '/' || r == ' ' || r == '\t'
		})
		if rest == "" {
			return prefix, true, "/"
		}
		return prefix, true, "/" + rest
	}
	return "", false, trimmed
}

func parseDurationToken(token string) (int, bool) {
	token = strings.TrimSuffix(strings.TrimSpace(token), "分钟")
	if token == "" {
		return 0, false
	}
	for _, r := range token {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	minutes, err := strconv.Atoi(token)
	return minutes, err == nil
}

func parseMaterialToken(token string) (string, bool, bool) {
	raw := strings.TrimSpace(token)
	value := true
	for _, suffix := range []string{"开启", "打开", "启用", "on"} {
		if strings.HasSuffix(strings.ToLower(raw), strings.ToLower(suffix)) {
			raw = strings.TrimSpace(raw[:len(raw)-len(suffix)])
			value = true
			break
		}
	}
	for _, suffix := range []string{"关闭", "关", "禁用", "off"} {
		if strings.HasSuffix(strings.ToLower(raw), strings.ToLower(suffix)) {
			raw = strings.TrimSpace(raw[:len(raw)-len(suffix)])
			value = false
			break
		}
	}
	for _, material := range BirthdayMaterials {
		for _, alias := range material.Aliases {
			if strings.EqualFold(raw, alias) {
				return material.Name, value, true
			}
		}
	}
	return "", false, false
}

func isSelectorToken(token string) bool {
	if !strings.HasPrefix(token, "u") || len(token) < 2 {
		return false
	}
	for _, r := range token[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func normalizeRegion(region string) string {
	normalized := renderregion.Normalize(strings.TrimSpace(region))
	if normalized.IsZero() {
		return renderregion.JP.String()
	}
	return normalized.String()
}

func selectorBindingServer(region string, regionExplicit bool) string {
	if !regionExplicit {
		return ""
	}
	return strings.TrimSpace(region)
}

func randomToken() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf[:]), nil
}

func randomVersionedToken() (string, string, error) {
	versionBytes := make([]byte, 12)
	if _, err := rand.Read(versionBytes); err != nil {
		return "", "", err
	}
	secret, err := randomToken()
	if err != nil {
		return "", "", err
	}
	version := base64.RawURLEncoding.EncodeToString(versionBytes)
	return version, version + "." + secret, nil
}

func tokenVersion(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if version, _, ok := strings.Cut(token, "."); ok {
		return strings.TrimSpace(version)
	}
	return ""
}

func normalizeUploadTime(value int64) time.Time {
	if value <= 0 {
		return time.Now()
	}
	if value > 1_000_000_000_000 {
		return time.UnixMilli(value)
	}
	return time.Unix(value, 0)
}

func normalizeMaterialIDs(values []int) []int {
	result := make([]int, 0, len(values))
	seen := make(map[int]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}
