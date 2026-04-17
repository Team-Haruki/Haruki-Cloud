package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/config"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/education"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderprofile "haruki-cloud/internal/pjsk/render/profile"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/utils/imagecache"
)

type bridgeEducationRegionValidator struct{}

func (bridgeEducationRegionValidator) GetUserProfile(server, userID string) (*sekaiapi.GetAnotherProfileResponse, error) {
	if server == string(renderregion.CN) {
		return &sekaiapi.GetAnotherProfileResponse{
			User: sekaiapi.AnotherUser{
				UserID: 12345678901234,
				Name:   "CN User",
			},
		}, nil
	}
	return nil, sekaiapi.ErrUserNotFound
}

type handlerTestEducationSource struct {
	region    renderregion.Value
	maxLevel  int
	assetName string
	boxes     map[int]*education.ResourceBox
	shopItems map[int]*education.ShopItem
}

func newHandlerTestEducationSource(region renderregion.Value, maxLevel int, assetName string) *handlerTestEducationSource {
	source := &handlerTestEducationSource{
		region:    region,
		maxLevel:  maxLevel,
		assetName: assetName,
		boxes:     make(map[int]*education.ResourceBox),
		shopItems: make(map[int]*education.ShopItem),
	}
	for level := 2; level <= maxLevel; level++ {
		boxID := 1000 + level
		source.boxes[boxID] = &education.ResourceBox{
			ID: boxID,
			Details: []education.ResourceBoxDetail{{
				ResourceType:  "area_item",
				ResourceID:    1,
				ResourceLevel: level,
			}},
		}
		source.shopItems[boxID] = &education.ShopItem{
			ID:            2000 + level,
			ResourceBoxID: boxID,
			StartAt:       1,
			Costs: []education.ShopItemCost{{
				ResourceType: "material",
				ResourceID:   1,
				Quantity:     1,
			}},
		}
	}
	return source
}

func (s *handlerTestEducationSource) DefaultRegion() renderregion.Value { return s.region }
func (s *handlerTestEducationSource) GetChallengeRewardsByCharacter(int) []*education.ChallengeReward {
	return nil
}
func (s *handlerTestEducationSource) GetResourceBoxByPurpose(purpose string, id int) *education.ResourceBox {
	if purpose != "shop_item" {
		return nil
	}
	return s.boxes[id]
}
func (s *handlerTestEducationSource) GetResourceBoxesByPurpose(purpose string) []*education.ResourceBox {
	if purpose != "shop_item" {
		return nil
	}
	out := make([]*education.ResourceBox, 0, len(s.boxes))
	for _, box := range s.boxes {
		out = append(out, box)
	}
	return out
}
func (s *handlerTestEducationSource) GetAreaItems() []*education.AreaItem {
	return []*education.AreaItem{{
		ID:              1,
		AreaID:          5,
		Name:            "item",
		AssetbundleName: s.assetName,
	}}
}
func (s *handlerTestEducationSource) GetAreaItem(id int) *education.AreaItem {
	if id != 1 {
		return nil
	}
	return &education.AreaItem{
		ID:              1,
		AreaID:          5,
		Name:            "item",
		AssetbundleName: s.assetName,
	}
}
func (s *handlerTestEducationSource) GetAreaItemLevels(areaItemID int) []*education.AreaItemLevel {
	if areaItemID != 1 {
		return nil
	}
	out := make([]*education.AreaItemLevel, 0, s.maxLevel)
	for level := 1; level <= s.maxLevel; level++ {
		out = append(out, &education.AreaItemLevel{
			AreaItemID:      1,
			Level:           level,
			TargetUnit:      "light_sound",
			Power1BonusRate: float64(level),
		})
	}
	return out
}
func (s *handlerTestEducationSource) GetAreaItemLevel(areaItemID, level int) *education.AreaItemLevel {
	if areaItemID != 1 || level <= 0 || level > s.maxLevel {
		return nil
	}
	return &education.AreaItemLevel{
		AreaItemID:      1,
		Level:           level,
		TargetUnit:      "light_sound",
		Power1BonusRate: float64(level),
	}
}
func (s *handlerTestEducationSource) GetCharacterRank(characterID, rank int) *education.CharacterRank {
	return nil
}
func (s *handlerTestEducationSource) GetBonds() []*education.Bond { return nil }
func (s *handlerTestEducationSource) GetBondLevels() []*education.BondLevel {
	return nil
}
func (s *handlerTestEducationSource) GetGameCharacterStyle(gameID int) *education.GameCharacterStyle {
	return nil
}
func (s *handlerTestEducationSource) GetLeaderMissionRequirements() ([]education.LeaderMissionRequirement, int) {
	return nil, 0
}
func (s *handlerTestEducationSource) GetMysekaiGateLevel(gateID, level int) *education.MysekaiGateLevel {
	return nil
}
func (s *handlerTestEducationSource) GetShopItemByResourceBoxID(resourceBoxID int) *education.ShopItem {
	return s.shopItems[resourceBoxID]
}
func (s *handlerTestEducationSource) GetShopItems() []*education.ShopItem {
	out := make([]*education.ShopItem, 0, len(s.shopItems))
	for _, item := range s.shopItems {
		out = append(out, item)
	}
	return out
}

func TestExecuteEducationAreaUsesResolvedRequestContextRegion(t *testing.T) {
	ctx := context.Background()
	service := newBridgeTestBindingServiceWithValidator(t, bridgeEducationRegionValidator{})
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	params, err := json.Marshal(education.AreaItemQuery{Unit: "light_sound"})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	if strings.Contains(string(params), "\"region\"") {
		t.Fatalf("expected area item params to omit empty region, got %s", params)
	}

	snapshot := mustBridgeEducationSnapshot(t)

	var captured drawing.AreaItemUpgradeMaterialsRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/education/area-item" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	controller := education.NewController(drawing.NewHarukiDrawingClient(server.URL), nil, nil, renderregion.JP)
	controller.RegisterSource(newHandlerTestEducationSource(renderregion.JP, 20, "jp_item"))
	controller.RegisterSource(newHandlerTestEducationSource(renderregion.CN, 15, "cn_item"))

	app := &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Edu:        controller,
		Bindings:   service,
		Snapshots:  rendersnapshot.NewStaticSnapshotProvider(snapshot),
		ImageCache: imagecache.New("https://example.com", cacheDir),
	}

	rc := &RequestContext{
		Ctx: ctx,
		Cmd: &parser.ResolvedCommand{
			Module:            parser.ModuleEducation,
			Mode:              "education-area",
			Region:            "",
			Params:            params,
			RequesterPlatform: "qq",
			RequesterUserID:   "42",
		},
		App:            app,
		Region:         renderregion.CN,
		RegionStr:      "cn",
		Platform:       "qq",
		PlatformUserID: "42",
	}

	message, err := executeEducation(rc)
	if err != nil {
		t.Fatalf("executeEducation() error = %v", err)
	}
	if len(message) != 1 || message[0].Type != onebot11.TypeImage {
		t.Fatalf("unexpected message: %+v", message)
	}
	if len(captured.AreaItems) != 1 {
		t.Fatalf("expected 1 area item, got %d", len(captured.AreaItems))
	}
	item := captured.AreaItems[0]
	if item.CurrentLevel != 15 {
		t.Fatalf("expected current level 15, got %d", item.CurrentLevel)
	}
	if len(item.Levels) != 0 {
		t.Fatalf("expected no future levels for cn source, got %+v", item.Levels)
	}
	if item.ItemIconPath != "asset/cn-assets/startapp/areaitem/cn_item/cn_item.png" {
		t.Fatalf("unexpected item icon path: %s", item.ItemIconPath)
	}
}

func TestExecuteEducationAreaRequiresSuiteSnapshotWhenBindingVisible(t *testing.T) {
	ctx := context.Background()
	service := newBridgeTestBindingServiceWithValidator(t, bridgeEducationRegionValidator{})
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	params, err := json.Marshal(education.AreaItemQuery{Unit: "light_sound"})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	_, err = executeEducation(&RequestContext{
		Ctx: ctx,
		Cmd: &parser.ResolvedCommand{
			Module:            parser.ModuleEducation,
			Mode:              "education-area",
			Region:            "jp",
			Params:            params,
			RequesterPlatform: "qq",
			RequesterUserID:   "42",
		},
		App: &renderapp.App{
			Edu:      education.NewController(nil, nil, nil, renderregion.JP),
			Bindings: service,
		},
		Region:         renderregion.JP,
		RegionStr:      "jp",
		Platform:       "qq",
		PlatformUserID: "42",
	})
	if err == nil {
		t.Fatal("expected missing suite snapshot to fail")
	}
	if err.Error() != ErrMsgSuiteDataNotFound {
		t.Fatalf("unexpected error: %v", err)
	}
}

type bridgeEducationSnapshotProviderStub struct {
	basicSnapshot rendersnapshot.Snapshot
	fullErr       error
}

func (p *bridgeEducationSnapshotProviderStub) Resolve(_ context.Context, _ rendersnapshot.Selector, opts rendersnapshot.ResolveOptions) (rendersnapshot.Snapshot, error) {
	if opts.NeedMySekai {
		return nil, p.fullErr
	}
	return p.basicSnapshot, nil
}

func TestExecuteEducationPowerFallsBackToSuiteSnapshotWhenMySekaiSnapshotUnavailable(t *testing.T) {
	ctx := context.Background()
	service := newBridgeTestBindingServiceWithValidator(t, bridgeEducationRegionValidator{})
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	snapshot := mustBridgeEducationSnapshot(t)
	provider := &bridgeEducationSnapshotProviderStub{
		basicSnapshot: snapshot,
		fullErr:       rendersnapshot.ErrSnapshotUnavailable,
	}

	var captured drawing.PowerBonusDetailRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/education/power-bonus" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	controller := education.NewController(drawing.NewHarukiDrawingClient(server.URL), nil, nil, renderregion.CN)
	controller.RegisterSource(newHandlerTestEducationSource(renderregion.CN, 15, "cn_item"))

	app := &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{Provider: "toolbox"},
		},
		Edu:        controller,
		Bindings:   service,
		ImageCache: imagecache.New("https://example.com", cacheDir),
	}

	originalFactory := snapshotProviderFactory
	snapshotProviderFactory = func(app *renderapp.App) rendersnapshot.HarukiSnapshotProvider {
		return provider
	}
	defer func() {
		snapshotProviderFactory = originalFactory
	}()

	rc := &RequestContext{
		Ctx: ctx,
		Cmd: &parser.ResolvedCommand{
			Module:            parser.ModuleEducation,
			Mode:              "education-power",
			Region:            "cn",
			RequesterPlatform: "qq",
			RequesterUserID:   "42",
		},
		App:            app,
		Region:         renderregion.CN,
		RegionStr:      "cn",
		Platform:       "qq",
		PlatformUserID: "42",
	}

	message, err := executeEducation(rc)
	if err != nil {
		t.Fatalf("executeEducation() error = %v", err)
	}
	if len(message) != 1 || message[0].Type != onebot11.TypeImage {
		t.Fatalf("unexpected message: %+v", message)
	}
	if len(captured.CharaBonuses) == 0 || len(captured.UnitBonuses) == 0 || len(captured.AttrBonuses) == 0 {
		t.Fatalf("expected power bonus request to fall back to suite snapshot, got %+v", captured)
	}
}

func TestExecuteEducationAreaPrefersAPIProfileOverSuiteSnapshotProfile(t *testing.T) {
	ctx := context.Background()
	service := newBridgeTestBindingServiceWithValidator(t, bridgeEducationRegionValidator{})
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/cn/12345678901234/profile" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(sekaiapi.GetAnotherProfileResponse{
			User: sekaiapi.AnotherUser{
				UserID: 12345678901234,
				Name:   "CN API User",
			},
			UserDeck: sekaiapi.UserDeck{
				DeckID: 1,
				Leader: 1001,
			},
			UserCards: []sekaiapi.AnotherUserCard{
				{
					CardID:       1001,
					DefaultImage: "special_training",
				},
			},
		})
	}))
	defer server.Close()

	oldBaseURL := config.Cfg.SekaiAPI.BaseURL
	oldToken := config.Cfg.SekaiAPI.Token
	config.Cfg.SekaiAPI.BaseURL = server.URL
	config.Cfg.SekaiAPI.Token = "test-token"
	defer func() {
		config.Cfg.SekaiAPI.BaseURL = oldBaseURL
		config.Cfg.SekaiAPI.Token = oldToken
	}()

	snapshot := mustBridgeEducationSnapshot(t)

	var captured drawing.AreaItemUpgradeMaterialsRequest
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/education/area-item" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
	}))
	defer drawingServer.Close()

	cacheDir := t.TempDir()
	controller := education.NewController(drawing.NewHarukiDrawingClient(drawingServer.URL), nil, nil, renderregion.CN)
	controller.RegisterSource(newHandlerTestEducationSource(renderregion.CN, 15, "cn_item"))
	profileController := renderprofile.NewController(runtimeProfileDataSourceStub{
		region: renderregion.CN,
		cards: map[int]*masterdata.Card{
			1001: {
				ID:              1001,
				AssetBundleName: "card_test",
			},
		},
	}, nil, nil, nil)

	app := &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Edu:        controller,
		Profiles:   profileController,
		Bindings:   service,
		SekaiAPI:   sekaiapi.NewSekaiAPIClient(&config.Cfg.SekaiAPI),
		Snapshots:  rendersnapshot.NewStaticSnapshotProvider(snapshot),
		ImageCache: imagecache.New("https://example.com", cacheDir),
	}

	params, err := json.Marshal(education.AreaItemQuery{Unit: "light_sound"})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	rc := &RequestContext{
		Ctx: ctx,
		Cmd: &parser.ResolvedCommand{
			Module:            parser.ModuleEducation,
			Mode:              "education-area",
			Region:            "cn",
			Params:            params,
			RequesterPlatform: "qq",
			RequesterUserID:   "42",
		},
		App:            app,
		Region:         renderregion.CN,
		RegionStr:      "cn",
		Platform:       "qq",
		PlatformUserID: "42",
	}

	message, err := executeEducation(rc)
	if err != nil {
		t.Fatalf("executeEducation() error = %v", err)
	}
	if len(message) != 1 || message[0].Type != onebot11.TypeImage {
		t.Fatalf("unexpected message: %+v", message)
	}
	if captured.Profile == nil {
		t.Fatal("expected profile in area-item request")
	}
	if got := captured.Profile.Nickname; got != "CN API User" {
		t.Fatalf("expected API profile nickname, got %q", got)
	}
	if got := captured.Profile.LeaderImagePath; got == "" || strings.Contains(got, "unknown") {
		t.Fatalf("expected resolved API leader image path, got %q", got)
	}
	if got := filepath.ToSlash(captured.Profile.LeaderImagePath); got == "asset/user/snapshot.png" {
		t.Fatalf("expected API leader image path, got snapshot fallback %q", got)
	}
}

func mustBridgeEducationSnapshot(t *testing.T) rendersnapshot.Snapshot {
	t.Helper()

	payload := map[string]any{
		"now": 1713000000000,
		"userGamedata": map[string]any{
			"userId": 1001,
			"name":   "tester",
			"deck":   1,
			"coin":   1000,
		},
		"userProfile": map[string]any{
			"profileImageType": "normal",
		},
		"userDecks": []map[string]any{
			{"deckId": 1, "leader": 1},
		},
		"userCards": []map[string]any{
			{"cardId": 1, "level": 1},
		},
		"userAreas": []map[string]any{
			{"areaItems": []map[string]any{
				{"areaItemId": 1, "level": 15},
			}},
		},
		"userMaterials": []map[string]any{
			{"materialId": 1, "quantity": 999},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	snapshot, err := rendersnapshot.NewFromBytes(nil, nil, renderregion.CN, data, nil, nil)
	if err != nil {
		t.Fatalf("NewFromBytes() error = %v", err)
	}
	return snapshot
}
