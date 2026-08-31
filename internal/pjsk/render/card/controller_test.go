package card

import (
	"errors"
	json "haruki-cloud/internal/jsonutil"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/releasecheck"
)

func TestBuildCardListRequestResolvesIDsFromQuery(t *testing.T) {
	source := &lookupTestSource{
		cards: []*masterdata.Card{
			{ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card A", AssetBundleName: "card_a"},
			{ID: 1002, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cool", Prefix: "Card B", AssetBundleName: "card_b"},
		},
	}
	controller := NewController(source, nil, nil, nil)
	req, err := controller.BuildCardListRequest(ListRequest{Query: "mnr 4星", Region: "jp"})
	if err != nil {
		t.Fatalf("BuildCardListRequest() error = %v", err)
	}
	if len(req.Cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(req.Cards))
	}
	if req.Cards[0].CardID != 1001 || req.Cards[1].CardID != 1002 {
		t.Fatalf("unexpected cards: %+v", req.Cards)
	}
}

func TestBuildCardListRequestRejectsUnreleasedCardQuery(t *testing.T) {
	now := time.Now().UnixMilli()
	source := &lookupTestSource{
		cards: []*masterdata.Card{
			{ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Future Card", AssetBundleName: "card_a", ReleaseAt: now + 60_000},
		},
	}
	controller := NewController(source, nil, nil, nil)
	_, err := controller.BuildCardListRequest(ListRequest{Query: "1001", Region: "jp"})
	if err == nil {
		t.Fatal("expected unreleased card query to fail")
	}
	var unreleased *releasecheck.UnreleasedError
	if !errors.As(err, &unreleased) {
		t.Fatalf("expected unreleased error, got %T (%v)", err, err)
	}
	if unreleased.Kind != releasecheck.KindCard || unreleased.ID != 1001 {
		t.Fatalf("unexpected unreleased error: %+v", unreleased)
	}
}

func TestBuildCardListRequestFiltersUnreleasedCardsFromQuery(t *testing.T) {
	now := time.Now().UnixMilli()
	source := &lookupTestSource{
		cards: []*masterdata.Card{
			{ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Released Card", AssetBundleName: "card_a", ReleaseAt: now - 60_000},
			{ID: 1002, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cool", Prefix: "Future Card", AssetBundleName: "card_b", ReleaseAt: now + 60_000},
		},
	}
	controller := NewController(source, nil, nil, nil)
	req, err := controller.BuildCardListRequest(ListRequest{Query: "mnr 4星", Region: "jp"})
	if err != nil {
		t.Fatalf("BuildCardListRequest() error = %v", err)
	}
	if len(req.Cards) != 1 || req.Cards[0].CardID != 1001 {
		t.Fatalf("unexpected released card list: %+v", req.Cards)
	}
}

func TestBuildCardListRequestAllowsAndMarksUnreleasedCardsForNonJPLookup(t *testing.T) {
	now := time.Now().UnixMilli()
	source := &lookupTestSource{
		region: renderregion.CN,
		cards: []*masterdata.Card{
			{ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Released Card", AssetBundleName: "card_a", ReleaseAt: now - 60_000},
			{ID: 1002, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cool", Prefix: "Future Card", AssetBundleName: "card_b", ReleaseAt: now + 60_000},
		},
	}
	controller := NewController(source, nil, nil, nil)
	req, err := controller.BuildCardListRequest(ListRequest{Query: "mnr 4星", Region: "cn", AllowUnreleased: true})
	if err != nil {
		t.Fatalf("BuildCardListRequest() error = %v", err)
	}
	if len(req.Cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(req.Cards))
	}
	if req.Cards[1].CardID != 1002 {
		t.Fatalf("expected future card to stay in list, got %+v", req.Cards)
	}
	if len(req.Cards[1].ThumbnailInfo) == 0 || req.Cards[1].ThumbnailInfo[0].CustomText == nil || *req.Cards[1].ThumbnailInfo[0].CustomText != "未上线" {
		t.Fatalf("expected unreleased label on future card, got %+v", req.Cards[1].ThumbnailInfo)
	}
}

func TestBuildCardListRequestResolvesAdvancedFiltersFromQuery(t *testing.T) {
	cardInfo := &masterdata.Card{ID: 1003, CharacterID: 21, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card C", AssetBundleName: "card_c", SupportUnit: "idol"}
	source := &lookupTestSource{
		cards:      []*masterdata.Card{cardInfo},
		characters: map[int]*masterdata.Character{21: {ID: 21, FirstName: "初音", GivenName: "未来", Unit: "piapro"}},
		filterFunc: func(info *PjskCardQueryInfo) ([]*masterdata.Card, error) {
			if info == nil {
				t.Fatal("expected query info")
			}
			if info.EventID != 123 {
				t.Fatalf("unexpected event id: %+v", info)
			}
			if info.MainUnit != "piapro" || info.SupportUnit != "idol" {
				t.Fatalf("unexpected unit filter: %+v", info)
			}
			if info.SupplyType != SupplyFes {
				t.Fatalf("unexpected supply filter: %+v", info)
			}
			if info.Year != 2025 {
				t.Fatalf("unexpected year filter: %+v", info)
			}
			return []*masterdata.Card{cardInfo}, nil
		},
	}
	controller := NewController(source, nil, nil, nil)
	req, err := controller.BuildCardListRequest(ListRequest{Query: "event123 mmjv fes 25年", Region: "jp"})
	if err != nil {
		t.Fatalf("BuildCardListRequest() error = %v", err)
	}
	if len(req.Cards) != 1 || req.Cards[0].CardID != 1003 {
		t.Fatalf("unexpected cards: %+v", req.Cards)
	}
}

func TestBuildCardListRequestIncludesLimitedIconPaths(t *testing.T) {
	source := &lookupTestSource{
		cards: []*masterdata.Card{
			{ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card A", AssetBundleName: "card_a"},
		},
		supplyByCard: map[int]string{
			1001: "term_limited",
		},
	}
	controller := NewController(source, nil, nil, nil)
	req, err := controller.BuildCardListRequest(ListRequest{Query: "1001", Region: "jp"})
	if err != nil {
		t.Fatalf("BuildCardListRequest() error = %v", err)
	}
	if req.TermLimitedIconPath == nil || !strings.Contains(*req.TermLimitedIconPath, "term_limited.png") {
		t.Fatalf("unexpected term limited icon path: %+v", req.TermLimitedIconPath)
	}
	if req.FesLimitedIconPath == nil || !strings.Contains(*req.FesLimitedIconPath, "fes_limited.png") {
		t.Fatalf("unexpected fes limited icon path: %+v", req.FesLimitedIconPath)
	}
}

func TestBuildCardBoxRequestMarksOwnedCardsFromDetailedProfile(t *testing.T) {
	source := &lookupTestSource{}
	builder := NewBuilder(source, nil, nil, nil)
	req, err := builder.BuildCardBoxRequest([]*masterdata.Card{
		{ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card A", AssetBundleName: "card_a"},
		{ID: 1002, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cool", Prefix: "Card B", AssetBundleName: "card_b"},
	}, "jp", &drawing.DetailedProfileCardRequest{UserCards: []any{map[string]any{"cardId": 1002}}}, false, false, false, true, "")
	if err != nil {
		t.Fatalf("BuildCardBoxRequest() error = %v", err)
	}
	if len(req.Cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(req.Cards))
	}
	if req.Cards[0].Card.CardID != 1001 || req.Cards[0].HasCard {
		t.Fatalf("unexpected first card state: %+v", req.Cards[0])
	}
	if req.Cards[1].Card.CardID != 1002 || !req.Cards[1].HasCard {
		t.Fatalf("unexpected second card state: %+v", req.Cards[1])
	}
}

func TestBuildCardBoxRequestSkipsUnusedSkillsAndDeduplicatesCharacterMetadata(t *testing.T) {
	source := &lookupTestSource{}
	builder := NewBuilder(source, nil, nil, nil)
	req, err := builder.BuildCardBoxRequest([]*masterdata.Card{
		{ID: 1001, CharacterID: 5, SkillID: 101, CardRarityType: "rarity_4", Attr: "cute", AssetBundleName: "card_a"},
		{ID: 1002, CharacterID: 5, SkillID: 102, CardRarityType: "rarity_4", Attr: "cool", AssetBundleName: "card_b"},
	}, "jp", nil, false, false, false, true, "")
	if err != nil {
		t.Fatalf("BuildCardBoxRequest() error = %v", err)
	}
	if source.skillLookups != 0 {
		t.Fatalf("card box performed %d unused skill lookups", source.skillLookups)
	}
	if source.colorLookups != 1 {
		t.Fatalf("character color lookups = %d, want 1", source.colorLookups)
	}
	for _, item := range req.Cards {
		if item.Card.Skill != nil || item.Card.SpecialSkillInfo != nil {
			t.Fatalf("card box unexpectedly includes unused skill payload: %+v", item.Card)
		}
		if len(item.Card.ThumbnailInfo) != 1 {
			t.Fatalf("card box should build exactly one final thumbnail: %+v", item.Card.ThumbnailInfo)
		}
	}
}

func TestBuildCardBoxRequestIncludesDistributionStats(t *testing.T) {
	source := &lookupTestSource{}
	builder := NewBuilder(source, nil, nil, nil)
	req, err := builder.BuildCardBoxRequest([]*masterdata.Card{
		{ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card A", AssetBundleName: "card_a"},
		{ID: 1002, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cool", Prefix: "Card B", AssetBundleName: "card_b"},
		{ID: 1003, CharacterID: 6, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card C", AssetBundleName: "card_c"},
	}, "jp", &drawing.DetailedProfileCardRequest{UserCards: []any{
		map[string]any{"cardId": 1001},
		map[string]any{"cardId": 1003},
	}}, false, false, false, true, CardBoxGroupByAttr)
	if err != nil {
		t.Fatalf("BuildCardBoxRequest() error = %v", err)
	}
	if req.GroupBy != CardBoxGroupByAttr {
		t.Fatalf("unexpected group_by: %q", req.GroupBy)
	}
	if req.Distribution == nil {
		t.Fatal("expected card box distribution")
	}
	if !req.Distribution.OwnedData || req.Distribution.TotalCount != 3 || req.Distribution.OwnedCount != 2 {
		t.Fatalf("unexpected distribution totals: %+v", req.Distribution)
	}
	if req.Distribution.MaxCharacterBarCount != 1 || req.Distribution.MaxAttributeBarCount != 2 {
		t.Fatalf("unexpected distribution max counts: %+v", req.Distribution)
	}

	character5 := cardDistributionCharacterStat(req.Distribution.CharacterStats, 5)
	if character5.Count != 2 || character5.OwnedCount != 1 || character5.BarCount != 1 {
		t.Fatalf("unexpected character 5 stat: %+v", character5)
	}

	cuteStat := cardDistributionAttributeStat(req.Distribution.AttributeStats, "cute")
	coolStat := cardDistributionAttributeStat(req.Distribution.AttributeStats, "cool")
	if cuteStat.Count != 2 || cuteStat.OwnedCount != 2 || cuteStat.BarCount != 2 || len(cuteStat.CharacterStats) != 2 {
		t.Fatalf("unexpected cute stat: %+v", cuteStat)
	}
	if coolStat.Count != 1 || coolStat.OwnedCount != 0 || coolStat.BarCount != 0 {
		t.Fatalf("unexpected cool stat: %+v", coolStat)
	}
}

func cardDistributionCharacterStat(stats []drawing.CardDistributionCharacterStat, characterID int) drawing.CardDistributionCharacterStat {
	for _, stat := range stats {
		if stat.CharacterID == characterID {
			return stat
		}
	}
	return drawing.CardDistributionCharacterStat{}
}

func cardDistributionAttributeStat(stats []drawing.CardDistributionAttributeStat, attr string) drawing.CardDistributionAttributeStat {
	for _, stat := range stats {
		if stat.Attr == attr {
			return stat
		}
	}
	return drawing.CardDistributionAttributeStat{}
}

func TestBuildCardBoxRequestAppliesDisplayFlagsAndBeforeSetting(t *testing.T) {
	source := &lookupTestSource{cards: []*masterdata.Card{{ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card A", AssetBundleName: "card_a"}}}
	controller := NewController(source, nil, nil, nil)
	req, err := controller.BuildCardBoxRequest([]Query{{Query: "1001", Region: "jp", ShowID: true, UseAfterTraining: new(false)}})
	if err != nil {
		t.Fatalf("BuildCardBoxRequest() error = %v", err)
	}
	if !req.ShowID || req.ShowBox {
		t.Fatalf("unexpected request flags: %+v", req)
	}
	if len(req.Cards) != 1 || req.Cards[0].Card.IsAfterTraining == nil || *req.Cards[0].Card.IsAfterTraining {
		t.Fatalf("unexpected after-training state: %+v", req.Cards)
	}
}

func TestBuildCardBoxRequestUsesOwnedCardDefaultImageEvenWhenBeforeIsSet(t *testing.T) {
	source := &lookupTestSource{cards: []*masterdata.Card{{ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card A", AssetBundleName: "card_a"}}}
	controller := NewController(source, nil, nil, nil)
	profile := &drawing.DetailedProfileCardRequest{
		ID:        "123456789",
		UserCards: []any{map[string]any{"cardId": 1001, "defaultImage": "special_training", "specialTrainingStatus": "done"}},
	}
	req, err := controller.BuildCardBoxRequest([]Query{{Query: "1001", Region: "jp", UseAfterTraining: new(false), DetailedProfile: profile}})
	if err != nil {
		t.Fatalf("BuildCardBoxRequest() error = %v", err)
	}
	if req.UserInfo == nil || req.UserInfo == profile || req.UserInfo.ID != profile.ID {
		t.Fatalf("expected explicit card box request to keep cloned display info, got %+v", req.UserInfo)
	}
	if len(req.UserInfo.UserCards) != 0 {
		t.Fatalf("card box user info must omit duplicated owned cards, got %d", len(req.UserInfo.UserCards))
	}
	if len(req.Cards) != 1 || req.Cards[0].Card.IsAfterTraining == nil || !*req.Cards[0].Card.IsAfterTraining {
		t.Fatalf("expected owned card to keep special_training default image: %+v", req.Cards)
	}
}

func TestBuildCardBoxRequestRejectsShowBoxWithoutOwnedCardData(t *testing.T) {
	source := &lookupTestSource{cards: []*masterdata.Card{{ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card A", AssetBundleName: "card_a"}}}
	controller := NewController(source, nil, nil, nil)
	_, err := controller.BuildCardBoxRequest([]Query{{Query: "1001", Region: "jp", ShowBox: true, UseAfterTraining: new(true)}})
	if err == nil {
		t.Fatal("expected show_box without owned-card data to fail")
	}
	if !strings.Contains(err.Error(), "box") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildCardBoxRequestPassesUnownedOnlyWithOwnedCardData(t *testing.T) {
	source := &lookupTestSource{cards: []*masterdata.Card{{ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card A", AssetBundleName: "card_a"}}}
	controller := NewController(source, nil, nil, nil)
	profile := &drawing.DetailedProfileCardRequest{UserCards: []any{map[string]any{"cardId": 1001}}}
	req, err := controller.BuildCardBoxRequest([]Query{{Query: "1001", Region: "jp", UnownedOnly: true, UseAfterTraining: new(true), DetailedProfile: profile}})
	if err != nil {
		t.Fatalf("BuildCardBoxRequest() error = %v", err)
	}
	if !req.UnownedOnly {
		t.Fatalf("expected unowned_only flag, got %+v", req)
	}

	_, err = controller.BuildCardBoxRequest([]Query{{Query: "1001", Region: "jp", UnownedOnly: true, UseAfterTraining: new(true)}})
	if err == nil {
		t.Fatal("expected unowned_only without owned-card data to fail")
	}
}

func TestBuildCardBoxRequestRejectsExplicitRegionWithoutSource(t *testing.T) {
	source := &lookupTestSource{cards: []*masterdata.Card{{ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card A", AssetBundleName: "card_a"}}}
	controller := NewController(source, nil, nil, nil)

	_, err := controller.BuildCardBoxRequest([]Query{{Query: "1001", Region: "cn"}})
	if err == nil {
		t.Fatal("expected explicit cn lookup without a cn source to fail")
	}
	if !strings.Contains(err.Error(), "region cn") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildCardBoxRequestStrictFilterTreats25AsSchoolRefusalUnit(t *testing.T) {
	source := &lookupTestSource{
		cards: []*masterdata.Card{
			{ID: 1001, CharacterID: 17, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Kana", AssetBundleName: "card_a"},
		},
		filterFunc: func(info *PjskCardQueryInfo) ([]*masterdata.Card, error) {
			if info == nil {
				t.Fatal("expected query info")
			}
			if info.Unit != "school_refusal" {
				t.Fatalf("unexpected unit filter: %+v", info)
			}
			if info.CharacterID != 0 {
				t.Fatalf("did not expect character id from bare 25: %+v", info)
			}
			return []*masterdata.Card{
				{ID: 1001, CharacterID: 17, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Kana", AssetBundleName: "card_a"},
			}, nil
		},
	}
	controller := NewController(source, nil, nil, nil)
	req, err := controller.BuildCardBoxRequest([]Query{{Query: "25", Region: "jp", StrictFilterOnly: true}})
	if err != nil {
		t.Fatalf("BuildCardBoxRequest() error = %v", err)
	}
	if len(req.Cards) != 1 || req.Cards[0].Card.CardID != 1001 {
		t.Fatalf("unexpected filtered cards: %+v", req.Cards)
	}
}

func TestBuildCardBoxRequestUsesRequestedRegionSourceForFullList(t *testing.T) {
	jpSource := &lookupTestSource{
		region: renderregion.JP,
		cards: []*masterdata.Card{
			{ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "JP Card", AssetBundleName: "card_jp", ReleaseAt: 1700000000000},
		},
		allowEmptyFilter: true,
	}
	cnSource := &lookupTestSource{
		region: renderregion.CN,
		cards: []*masterdata.Card{
			{ID: 2001, CharacterID: 6, CardRarityType: "rarity_4", Attr: "cool", Prefix: "CN Card", AssetBundleName: "card_cn", ReleaseAt: 1700000000000},
		},
		allowEmptyFilter: true,
	}

	controller := NewController(jpSource, nil, nil, nil)
	controller.RegisterSource(cnSource)

	req, err := controller.BuildCardBoxRequest([]Query{{Region: "cn"}})
	if err != nil {
		t.Fatalf("BuildCardBoxRequest() error = %v", err)
	}
	if req.Region != "cn" {
		t.Fatalf("expected cn region, got %q", req.Region)
	}
	if len(req.Cards) != 1 || req.Cards[0].Card.CardID != 2001 {
		t.Fatalf("unexpected cards: %+v", req.Cards)
	}
}

func TestBuildCardBoxRequestUsesOwnedCardVisualStateFromDetailedProfile(t *testing.T) {
	source := &lookupTestSource{}
	builder := NewBuilder(source, nil, nil, nil)
	req, err := builder.BuildCardBoxRequest([]*masterdata.Card{
		{ID: 190, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card A", AssetBundleName: "card_a", InitialSpecialTrainingStatus: "not_done"},
		{ID: 191, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card B", AssetBundleName: "card_b", InitialSpecialTrainingStatus: "not_done"},
	}, "jp", &drawing.DetailedProfileCardRequest{UserCards: []any{
		map[string]any{"cardId": 190, "level": 60, "masterRank": 5, "specialTrainingStatus": "done", "defaultImage": "normal"},
		map[string]any{"cardId": 191, "level": 50, "masterRank": 2, "specialTrainingStatus": "done", "defaultImage": "special_training"},
	}}, false, false, false, true, "")
	if err != nil {
		t.Fatalf("BuildCardBoxRequest() error = %v", err)
	}
	if len(req.Cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(req.Cards))
	}
	first := req.Cards[0].Card
	if len(first.ThumbnailInfo) != 1 {
		t.Fatalf("expected 1 thumbnail for first card, got %+v", first.ThumbnailInfo)
	}
	if first.IsAfterTraining == nil || *first.IsAfterTraining {
		t.Fatalf("expected first card display to stay normal, got %+v", first.IsAfterTraining)
	}
	if !first.ThumbnailInfo[0].IsPcard {
		t.Fatalf("expected first card thumbnail to be pcard")
	}
	if first.ThumbnailInfo[0].Level == nil || *first.ThumbnailInfo[0].Level != 60 {
		t.Fatalf("unexpected first card level: %+v", first.ThumbnailInfo[0].Level)
	}
	if first.ThumbnailInfo[0].TrainRank == nil || *first.ThumbnailInfo[0].TrainRank != 5 {
		t.Fatalf("unexpected first card train rank: %+v", first.ThumbnailInfo[0].TrainRank)
	}
	if first.ThumbnailInfo[0].RareImgPath != "static_images/card/rare_star_after_training.png" {
		t.Fatalf("expected first card to use after-training stars, got %q", first.ThumbnailInfo[0].RareImgPath)
	}
	second := req.Cards[1].Card
	if len(second.ThumbnailInfo) != 1 {
		t.Fatalf("expected 1 thumbnail for second card, got %+v", second.ThumbnailInfo)
	}
	if second.IsAfterTraining == nil || !*second.IsAfterTraining {
		t.Fatalf("expected second card display to use after training, got %+v", second.IsAfterTraining)
	}
	if second.ThumbnailInfo[0].RareImgPath != "static_images/card/rare_star_after_training.png" {
		t.Fatalf("expected second card to use after-training stars, got %q", second.ThumbnailInfo[0].RareImgPath)
	}
}

func TestRenderCardListAutoSwitchesToCardBoxWhenTooManyCards(t *testing.T) {
	source := &lookupTestSource{}
	for i := 1; i <= cardListAutoBoxThreshold; i++ {
		source.cards = append(source.cards, &masterdata.Card{ID: 2000 + i, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card", AssetBundleName: "card_a"})
	}
	controller := NewController(source, nil, nil, nil)
	cardIDs := make([]int, 0, len(source.cards))
	for _, card := range source.cards {
		cardIDs = append(cardIDs, card.ID)
	}
	reqAny, autoBox, err := controller.buildCardListRenderRequest(ListRequest{
		CardIDs: cardIDs,
		Region:  "jp",
	})
	if err != nil {
		t.Fatalf("buildCardListRenderRequest() error = %v", err)
	}
	if !autoBox {
		t.Fatal("expected card list auto fallback to card box request")
	}
	req, ok := reqAny.(*drawing.CardBoxRequest)
	if !ok {
		t.Fatalf("expected card box request, got %T", reqAny)
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal card box request: %v", err)
	}
	var captured drawing.CardBoxRequest
	if err := json.Unmarshal(raw, &captured); err != nil {
		t.Fatalf("unmarshal card box request: %v", err)
	}
	if captured.UserInfo != nil {
		t.Fatalf("expected auto-switched card box request to omit user info, got %+v", captured.UserInfo)
	}
	if len(captured.Cards) != cardListAutoBoxThreshold {
		t.Fatalf("expected %d cards in card box request, got %d", cardListAutoBoxThreshold, len(captured.Cards))
	}
	for _, item := range captured.Cards {
		if item.HasCard {
			t.Fatalf("expected auto-switched card box request to mark all cards as unowned, got %+v", item)
		}
	}
}

func TestRenderCardListAutoSwitchesToCardBoxOmitsUserInfo(t *testing.T) {
	source := &lookupTestSource{}
	for i := 1; i <= cardListAutoBoxThreshold; i++ {
		source.cards = append(source.cards, &masterdata.Card{ID: 3000 + i, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card", AssetBundleName: "card_a"})
	}
	controller := NewController(source, nil, nil, nil)
	cardIDs := make([]int, 0, len(source.cards))
	for _, card := range source.cards {
		cardIDs = append(cardIDs, card.ID)
	}
	profile := &drawing.DetailedProfileCardRequest{
		ID:              "123456789",
		Region:          "jp",
		Nickname:        "tester",
		Source:          "suite",
		UpdateTime:      1710000000000,
		LeaderImagePath: "leader.png",
		UserCards:       []any{map[string]any{"cardId": 3001}},
	}

	reqAny, autoBox, err := controller.buildCardListRenderRequest(ListRequest{CardIDs: cardIDs, Region: "jp", DetailedProfile: profile})
	if err != nil {
		t.Fatalf("buildCardListRenderRequest() error = %v", err)
	}
	if !autoBox {
		t.Fatal("expected card list auto fallback to card box request")
	}
	req, ok := reqAny.(*drawing.CardBoxRequest)
	if !ok {
		t.Fatalf("expected card box request, got %T", reqAny)
	}
	if req.UserInfo != nil {
		t.Fatalf("expected auto-switched card box request to omit user info, got %+v", req.UserInfo)
	}
	for _, item := range req.Cards {
		if item.HasCard {
			t.Fatalf("expected auto-switched card box request to mark all cards as unowned, got %+v", item)
		}
	}
}
