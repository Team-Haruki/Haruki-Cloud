package provider

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func writeLocalCoverageFile(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func newLocalCoverageProvider(t *testing.T) *LocalProvider {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"gameCharacters.json": `[
			{"id":1,"firstName":"Hoshino","givenName":"Ichika","firstNameEnglish":"Hoshino","givenNameEnglish":"Ichika","unit":"light_sound","gender":"female"},
			{"id":21,"firstName":"Hatsune","givenName":"Miku","firstNameEnglish":"Hatsune","givenNameEnglish":"Miku","unit":"piapro","gender":"female"}
		]`,
		"gameCharacterUnits.json": `[
			{"ID":10,"GameCharacterID":1,"Unit":"light_sound","ColorCode":"#112233"},
			{"ID":21,"GameCharacterID":21,"Unit":"piapro","ColorCode":"#33ccbb"},
			{"ID":22,"GameCharacterID":0,"Unit":"idol","ColorCode":""}
		]`,
		"skills.json": `[
			{"id":1,"shortDescription":"score","description":"{{1;v}}","descriptionSpriteName":"score_up","skillEffects":[{"id":1,"activateEffectValue":100}]},
			{"id":2,"shortDescription":"heal","description":"{{2;c}}","descriptionSpriteName":"life_recovery","skillEffects":[]}
		]`,
		"cards.json": `[
			{"ID":1,"CharacterID":1,"CardRarityType":"rarity_3","Attr":"happy","Prefix":"First","ReleaseAt":1704067200000,"SkillID":2,"CardSupplyID":1},
			{"ID":2,"CharacterID":21,"CardRarityType":"rarity_4","Attr":"cool","Prefix":"Second","ReleaseAt":1735689600000,"SkillID":1,"CardSupplyID":2,"SupportUnit":"idol"},
			{"ID":3,"CharacterID":21,"CardRarityType":"rarity_birthday","Attr":"cute","Prefix":"Birthday","ReleaseAt":1767225600000,"SkillID":1,"CardSupplyID":0}
		]`,
		"cardSupplies.json": `[
			{"id":1,"cardSupplyType":"normal"},
			{"id":2,"cardSupplyType":"term_limited"}
		]`,
		"events.json": `[
			{"id":10,"eventType":"world_bloom","unit":"none","name":"World Link","startAt":2000,"eventRankingRewardRanges":[{"eventRankingRewardDetails":[{"resourceType":"honor","resourceId":100}]}]},
			{"id":20,"eventType":"marathon","unit":"light_sound","name":"Earlier","startAt":1000},
			{"id":21,"eventType":"marathon","unit":"light_sound","name":"Later","startAt":3000}
		]`,
		"eventCards.json": `[
			{"cardId":2,"eventId":10},
			{"cardId":1,"eventId":20}
		]`,
		"gachas.json": `[
			{"id":50,"name":"Old","startAt":100,"gachaPickups":[{"id":1,"gachaId":50,"cardId":1}]},
			{"id":51,"name":"New","startAt":200,"gachaCeilItemId":7,"gachaPickups":[{"id":2,"gachaId":51,"cardId":2}]},
			{"id":52,"name":"Same time","startAt":200}
		]`,
		"gachaCeilItems.json": `[
			{"id":7,"assetbundleName":"ceil_7"},
			{"id":0,"assetbundleName":"ignored"},
			{"id":8,"assetbundleName":""}
		]`,
		"costume3ds.json": `[
			{"id":1001,"seq":1,"costume3dGroupId":100,"costume3dType":"normal","characterId":1,"name":"Blue Star","partType":"body","colorId":1,"colorName":"Blue","howToObtain":"gacha","assetbundleName":"costume_1","designer":"A","archivePublishedAt":100,"publishedAt":200},
			{"id":1002,"seq":2,"costume3dGroupId":100,"costume3dType":"normal","characterId":1,"name":"Red Star","partType":"body","colorId":2,"colorName":"Red","howToObtain":"shop","assetbundleName":"costume_2","designer":"B","archivePublishedAt":300},
			{"id":1003,"seq":3,"costume3dGroupId":101,"costume3dType":"special","characterId":21,"name":"Miku Hair","partType":"hair","colorId":1,"colorName":"Green","howToObtain":"event","assetbundleName":"costume_3","designer":"C","publishedAt":200},
			{"id":0,"name":"ignored"}
		]`,
		"cardCostume3ds.json": `[
			{"cardId":2,"costume3dId":1001},
			{"cardId":1,"costume3dId":1002},
			{"cardId":0,"costume3dId":1003},
			{"cardId":3,"costume3dId":0}
		]`,
		"cardEpisodes.json": `[
			{"id":3,"seq":2,"cardId":2,"cardEpisodePartType":"after_training"},
			{"id":2,"seq":1,"cardId":2,"cardEpisodePartType":"first_part"},
			{"id":1,"seq":1,"cardId":2,"cardEpisodePartType":"first_part_alt"}
		]`,
		"musics.json": `[
			{"id":1,"seq":1,"title":"Alpha Song","pronunciation":"Arufa","assetbundleName":"music_1","publishedAt":100,"releasedAt":110},
			{"id":2,"seq":2,"title":"Beta Song","pronunciation":"Beta Song","assetbundleName":"music_2","publishedAt":100,"releasedAt":120}
		]`,
		"musicDifficulties.json": `[
			{"id":1,"musicId":1,"musicDifficulty":"expert","playLevel":28,"totalNoteCount":1000},
			{"id":2,"musicId":1,"musicDifficulty":"master","playLevel":31,"totalNoteCount":1200}
		]`,
		"musicVocals.json": `[
			{"id":11,"musicId":1,"musicVocalType":"sekai","caption":"Sekai","characters":[{"characterType":"game_character","characterId":1},{"characterType":"outside_character","characterId":"bad"}],"assetbundleName":"vocal_1"}
		]`,
		"musicTags.json": `[
			{"musicId":1,"musicTag":" mv "},
			{"musicId":1,"musicTag":""}
		]`,
		"outsideCharacters.json": `[
			{"id":5,"name":" Guest "}
		]`,
		"eventMusics.json": `[
			{"eventId":21,"musicId":1,"seq":2},
			{"eventId":20,"musicId":1,"seq":1},
			{"eventId":20,"musicId":2,"seq":2}
		]`,
		"limitedTimeMusics.json": `[
			{"ID":1,"MusicID":1,"StartAt":100,"EndAt":200}
		]`,
		"honors.json": `[
			{"ID":100,"GroupID":200,"HonorType":"rank_match","Name":"Champion","Levels":[{"Level":1,"Description":"level"}]}
		]`,
		"honorGroups.json": `[
			{"ID":200,"HonorType":"birthday","Name":"Ichika Birthday"},
			{"ID":201,"HonorType":"normal","Name":"Normal"}
		]`,
		"bondsHonors.json": `[
			{"ID":300,"GameCharacterUnitID1":10,"GameCharacterUnitID2":21,"Name":"Bond"}
		]`,
		"bondsHonorWords.json": `[
			{"ID":301,"Seq":1,"BondsGroupID":9,"Name":"Together"}
		]`,
		"stamps.json": `[
			{"ID":1,"AssetBundleName":"stamp_1","CharacterID":1,"CharacterID2":21}
		]`,
		"virtualLives.json": `[
			{"id":2,"name":"Later","assetbundleName":"live_2","startAt":200,"endAt":300,"virtualLiveSchedules":[{"startAt":210,"endAt":220},{"startAt":0,"endAt":1}],"virtualLiveRewards":[{"virtualLiveType":"normal","resourceBoxId":50},{"resourceBoxId":0}],"virtualLiveCharacters":[{"gameCharacterUnitId":10,"virtualLivePerformanceType":"normal"},{"gameCharacterUnitId":0}]},
			{"id":1,"name":"Earlier","assetbundleName":"live_1","startAt":100,"endAt":150}
		]`,
		"playerFrames.json": `[
			{"ID":1,"Seq":1,"PlayerFrameGroupID":2,"Description":"frame","GameCharacterID":1}
		]`,
		"playerFrameGroups.json": `[
			{"ID":2,"Seq":1,"Name":"group","AssetBundleName":"frame_group"}
		]`,
		"areaItems.json": `[
			{"ID":1,"AreaID":2,"Name":"Plant","AssetbundleName":"area_1"}
		]`,
		"areaItemLevels.json": `[
			{"AreaItemID":1,"Level":1,"TargetUnit":"light_sound","Power1BonusRate":0.1},
			{"AreaItemID":1,"Level":2,"TargetUnit":"light_sound","Power1BonusRate":0.2}
		]`,
		"characterRanks.json": `[
			{"characterId":1,"characterRank":5,"power1BonusRate":0.05}
		]`,
		"bonds.json": `[
			{"groupId":1,"characterId1":1,"characterId2":21}
		]`,
		"levels.json": `[
			{"levelType":"bonds","level":2,"totalExp":200},
			{"levelType":"bonds","level":1,"totalExp":100},
			{"levelType":"character","level":1,"totalExp":300},
			{"levelType":"unknown","level":1,"totalExp":0},
			{"levelType":"bonds","level":0,"totalExp":0}
		]`,
		"characterMissionV2s.json": `[
			{"id":1,"characterId":1,"characterMissionType":"leader","parameterGroupId":10,"isAchievementMission":true}
		]`,
		"characterMissionV2ParameterGroups.json": `[
			{"id":10,"gameId":1,"seq":2,"requirement":20,"exp":3,"quantity":1},
			{"id":10,"gameId":1,"seq":1,"requirement":10,"exp":2,"quantity":1},
			{"id":11,"gameId":101,"seq":2,"requirement":200},
			{"id":11,"gameId":101,"seq":1,"requirement":100}
		]`,
		"mysekaiGateLevels.json": `[
			{"mysekaiGateId":1,"level":2,"powerBonusRate":0.2}
		]`,
		"shopItems.json": `[
			{"id":1,"shopId":2,"seq":1,"resourceBoxId":50,"releaseConditionId":3,"startAt":100,"costs":[{"cost":{"resourceType":"coin","resourceId":1,"quantity":10}}]}
		]`,
		"mysekaiFixture.json": `[
			{"id":1,"large":9007199254740993},
			{"id":"2","name":"second"},
			{"name":"no id"}
		]`,
		"mysekaiObject.json": `{"id":3,"name":"object"}`,
	}
	for name, content := range files {
		writeLocalCoverageFile(t, root, name, content)
	}
	return NewLocalProvider(root, renderregion.JP)
}

func TestLocalCardMusicCostumeAndGachaProviders(t *testing.T) {
	p := newLocalCoverageProvider(t)
	ctx := context.Background()
	testLocalCardLookupBranches(t, p, ctx)
	testLocalCardFilterBranches(t, p, ctx)
	testLocalCardMetadataBranches(t, p, ctx)
	testLocalMusicLookupBranches(t, p, ctx)
	testLocalMusicDetailBranches(t, p, ctx)
	testLocalCostumeBranches(t, p, ctx)
	testLocalGachaBranches(t, p, ctx)
}

func testLocalCardLookupBranches(t *testing.T, p *LocalProvider, ctx context.Context) {
	t.Helper()
	if _, err := p.cards.GetByID(ctx, 0); err == nil {
		t.Fatal("zero card id was accepted")
	}
	if card, err := p.cards.GetByID(ctx, 2); err != nil || card.Prefix != "Second" {
		t.Fatalf("GetByID(2) = %+v, %v", card, err)
	}
	if _, err := p.cards.GetByID(ctx, 999); err == nil {
		t.Fatal("missing card was accepted")
	}
	if _, err := p.cards.GetByCharacterAndSeq(ctx, 0, 1); err == nil {
		t.Fatal("missing card character was accepted")
	}
	if card, err := p.cards.GetByCharacterAndSeq(ctx, 21, -1); err != nil || card.ID != 3 {
		t.Fatalf("latest card = %+v, %v", card, err)
	}
	if card, err := p.cards.GetByCharacterAndSeq(ctx, 21, 1); err != nil || card.ID != 2 {
		t.Fatalf("first character card = %+v, %v", card, err)
	}
	for _, seq := range []int{-9, 9} {
		if _, err := p.cards.GetByCharacterAndSeq(ctx, 21, seq); err == nil {
			t.Fatalf("out-of-range sequence %d was accepted", seq)
		}
	}
	if _, err := p.cards.GetByCharacterAndSeq(ctx, 99, 1); err == nil {
		t.Fatal("character without cards was accepted")
	}
	if _, err := p.cards.Filter(ctx, nil); err == nil {
		t.Fatal("nil card filter was accepted")
	}
}

func testLocalCardFilterBranches(t *testing.T, p *LocalProvider, ctx context.Context) {
	t.Helper()
	filtered, err := p.cards.Filter(ctx, &CardFilter{
		CharacterID: 21,
		Unit:        "idol",
		Rarity:      "rarity_4",
		Attr:        "cool",
		SkillType:   "score_up",
		SkillIDs:    []int{1},
		SupplyType:  "limited",
		Year:        2025,
		EventID:     10,
		Limit:       1,
	})
	if err != nil || len(filtered) != 1 || filtered[0].ID != 2 {
		t.Fatalf("full card filter = %+v, %v", filtered, err)
	}
	if filtered, err = p.cards.Filter(ctx, &CardFilter{EventID: 999}); err != nil || filtered != nil {
		t.Fatalf("missing event filter = %+v, %v", filtered, err)
	}
	for _, filter := range []*CardFilter{
		{CharacterID: 999}, {Rarity: "missing"}, {Attr: "missing"}, {SkillIDs: []int{999}},
		{Year: 1999}, {Unit: "missing"}, {SkillType: "missing"}, {SupplyType: "festival"},
	} {
		if got, err := p.cards.Filter(ctx, filter); err != nil || len(got) != 0 {
			t.Fatalf("nonmatching card filter %+v = %+v, %v", filter, got, err)
		}
	}
}

func testLocalCardMetadataBranches(t *testing.T, p *LocalProvider, ctx context.Context) {
	t.Helper()
	testLocalCardSupplyBranches(t, p, ctx)
	testLocalCardSourceBranches(t, p, ctx)
}

func testLocalCardSupplyBranches(t *testing.T, p *LocalProvider, ctx context.Context) {
	t.Helper()
	if got := p.cards.GetSupplyType(ctx, nil); got != "normal" {
		t.Fatalf("nil card supply type = %q", got)
	}
	if got := p.cards.GetSupplyType(ctx, &masterdata.Card{CardRarityType: "rarity_birthday"}); got != "birthday" {
		t.Fatalf("birthday supply = %q", got)
	}
	if got := p.cards.GetSupplyType(ctx, &masterdata.Card{}); got != "normal" {
		t.Fatalf("default supply = %q", got)
	}
	if got := p.cards.GetSupplyType(ctx, &masterdata.Card{ID: 2, CardSupplyID: 2}); got != "unit_event_limited" {
		t.Fatalf("world-link supply = %q", got)
	}
	if p.cards.isWorldLink3Card(0) {
		t.Fatal("zero card reported as world-link")
	}
	if gacha, err := p.cards.GetGachaByCardID(ctx, 2); err != nil || gacha.ID != 51 {
		t.Fatalf("card gacha = %+v, %v", gacha, err)
	}
	if _, err := p.cards.GetGachaByCardID(ctx, 0); err == nil {
		t.Fatal("zero gacha card was accepted")
	}
	if _, err := p.cards.GetGachaByCardID(ctx, 999); err == nil {
		t.Fatal("missing card gacha was accepted")
	}
}

func testLocalCardSourceBranches(t *testing.T, p *LocalProvider, ctx context.Context) {
	t.Helper()
	if costumes, err := p.cards.GetCostume3dsByCardID(ctx, 2); err != nil || len(costumes) != 1 || costumes[0].ID != 1001 {
		t.Fatalf("card costumes = %+v, %v", costumes, err)
	}
	if costumes, err := p.cards.GetCostume3dsByCardID(ctx, 0); err != nil || costumes != nil {
		t.Fatalf("zero card costumes = %+v, %v", costumes, err)
	}
	if unit, err := p.cards.GetUnitByCardID(ctx, 1); err != nil || unit != "light_sound" {
		t.Fatalf("card unit = %q, %v", unit, err)
	}
	if unit, err := p.cards.GetUnitByCardID(ctx, 2); err != nil || unit != "idol" {
		t.Fatalf("virtual singer card unit = %q, %v", unit, err)
	}
	if episodes, err := p.cards.GetEpisodesByCardID(ctx, 2); err != nil || len(episodes) != 3 || episodes[0].ID != 1 {
		t.Fatalf("card episodes = %+v, %v", episodes, err)
	}
	if episodes, err := p.cards.GetEpisodesByCardID(ctx, 999); err != nil || episodes != nil {
		t.Fatalf("missing card episodes = %+v, %v", episodes, err)
	}
}

func testLocalMusicLookupBranches(t *testing.T, p *LocalProvider, ctx context.Context) {
	t.Helper()
	testLocalMusicSearchBranches(t, p, ctx)
	testLocalMusicListBranches(t, p, ctx)
}

func testLocalMusicSearchBranches(t *testing.T, p *LocalProvider, ctx context.Context) {
	t.Helper()
	if _, err := p.musics.Search(ctx, " "); err == nil {
		t.Fatal("empty music search was accepted")
	}
	for query, wantID := range map[string]int{"1": 1, "alpha": 1, "arufa": 1} {
		music, err := p.musics.Search(ctx, query)
		if err != nil || music.ID != wantID {
			t.Fatalf("music search %q = %+v, %v", query, music, err)
		}
	}
	if _, err := p.musics.Search(ctx, "missing"); err == nil {
		t.Fatal("missing music search succeeded")
	}
	if _, err := p.musics.GetByID(ctx, 0); err == nil {
		t.Fatal("zero music id was accepted")
	}
	if _, err := p.musics.GetByID(ctx, 999); err == nil {
		t.Fatal("missing music was accepted")
	}
	if music, err := p.musics.GetByEventID(ctx, 20); err != nil || music.ID != 1 {
		t.Fatalf("event music = %+v, %v", music, err)
	}
	if _, err := p.musics.GetByEventID(ctx, 999); err == nil {
		t.Fatal("missing event music succeeded")
	}
}

func testLocalMusicListBranches(t *testing.T, p *LocalProvider, ctx context.Context) {
	t.Helper()
	if all := p.musics.GetAll(ctx); len(all) != 2 || all[0].ID != 1 {
		t.Fatalf("all musics = %+v", all)
	}
	if titles, err := p.musics.GetLocalizedTitles(ctx, 1); err != nil || len(titles) != 2 {
		t.Fatalf("localized titles = %+v, %v", titles, err)
	}
	if titles, err := p.musics.GetLocalizedTitles(ctx, 2); err != nil || len(titles) != 1 {
		t.Fatalf("deduplicated titles = %+v, %v", titles, err)
	}
	if _, err := p.musics.GetLocalizedTitles(ctx, 0); err == nil {
		t.Fatal("zero localized-title id was accepted")
	}
}

func testLocalMusicDetailBranches(t *testing.T, p *LocalProvider, ctx context.Context) {
	t.Helper()
	testLocalMusicArrangementBranches(t, p, ctx)
	testLocalMusicAssociationBranches(t, p, ctx)
}

func testLocalMusicArrangementBranches(t *testing.T, p *LocalProvider, ctx context.Context) {
	t.Helper()
	if diffs, err := p.musics.GetDifficulties(ctx, 1); err != nil || len(diffs) != 2 {
		t.Fatalf("music difficulties = %+v, %v", diffs, err)
	}
	if _, err := p.musics.GetDifficulties(ctx, 999); err == nil {
		t.Fatal("missing difficulties succeeded")
	}
	if vocals, err := p.musics.GetVocals(ctx, 1); err != nil || len(vocals) != 1 || len(vocals[0].Characters) != 1 {
		t.Fatalf("music vocals = %+v, %v", vocals, err)
	}
	if _, err := p.musics.GetVocals(ctx, 999); err == nil {
		t.Fatal("missing vocals succeeded")
	}
	if tags, err := p.musics.GetTags(ctx, 1); err != nil || len(tags) != 1 || tags[0] != "mv" {
		t.Fatalf("music tags = %+v, %v", tags, err)
	}
}

func testLocalMusicAssociationBranches(t *testing.T, p *LocalProvider, ctx context.Context) {
	t.Helper()
	if name, err := p.musics.GetOutsideCharacterByID(ctx, 5); err != nil || name != "Guest" {
		t.Fatalf("outside character = %q, %v", name, err)
	}
	if _, err := p.musics.GetOutsideCharacterByID(ctx, 0); err == nil {
		t.Fatal("zero outside character was accepted")
	}
	if _, err := p.musics.GetOutsideCharacterByID(ctx, 999); err == nil {
		t.Fatal("missing outside character was accepted")
	}
	if event, err := p.musics.GetPrimaryEventByMusicID(ctx, 1); err != nil || event.ID != 20 {
		t.Fatalf("primary music event = %+v, %v", event, err)
	}
	if _, err := p.musics.GetPrimaryEventByMusicID(ctx, 999); err == nil {
		t.Fatal("missing primary music event succeeded")
	}
	if limited := p.musics.GetLimitedTimeMusics(ctx, 1); len(limited) != 1 {
		t.Fatalf("limited musics = %+v", limited)
	}
	if limited := p.musics.GetLimitedTimeMusics(ctx, 999); limited != nil {
		t.Fatalf("missing limited musics = %+v", limited)
	}
}

func testLocalCostumeBranches(t *testing.T, p *LocalProvider, ctx context.Context) {
	t.Helper()
	testLocalCostumeFilterBranches(t, p, ctx)
	testLocalCostumeMetadataBranches(t, p, ctx)
}

func testLocalCostumeFilterBranches(t *testing.T, p *LocalProvider, ctx context.Context) {
	t.Helper()
	if _, err := p.costumes.GetByID(ctx, 0); err == nil {
		t.Fatal("zero costume id was accepted")
	}
	if costume, err := p.costumes.GetByID(ctx, 1001); err != nil || costume.AssetBundleName != "costume_1" {
		t.Fatalf("costume = %+v, %v", costume, err)
	}
	if _, err := p.costumes.GetByID(ctx, 999); err == nil {
		t.Fatal("missing costume was accepted")
	}
	if items, err := p.costumes.Filter(ctx, nil); err != nil || len(items) != 3 {
		t.Fatalf("all costumes = %+v, %v", items, err)
	}
	filters := []*CostumeFilter{
		{PartType: "body"}, {CostumeType: "special"}, {CharacterID: 21}, {CharacterIDs: []int{21}},
		{ColorID: 2}, {Keyword: "designer"}, {Keyword: "red"}, {Offset: -1, Limit: 1}, {Offset: 99},
	}
	for _, filter := range filters {
		if _, err := p.costumes.Filter(ctx, filter); err != nil {
			t.Fatalf("costume filter %+v: %v", filter, err)
		}
	}
}

func testLocalCostumeMetadataBranches(t *testing.T, p *LocalProvider, ctx context.Context) {
	t.Helper()
	if variants, err := p.costumes.GetVariants(ctx, 100, "body", 1); err != nil || len(variants) != 2 || variants[0].ColorID != 1 {
		t.Fatalf("costume variants = %+v, %v", variants, err)
	}
	if _, err := p.costumes.GetVariants(ctx, 0, "", 0); err == nil {
		t.Fatal("zero costume group was accepted")
	}
	if sources, err := p.costumes.GetSourceCardIDs(ctx, []int{1001, 1002, 0}); err != nil || len(sources[1001]) != 1 || len(sources[1002]) != 1 {
		t.Fatalf("costume source cards = %+v, %v", sources, err)
	}
	if localCostumeMatchesFilter(nil, &CostumeFilter{}) || costumeListTime(nil) != 0 {
		t.Fatal("nil costume helpers returned a match/time")
	}
	if localCostumeContainsKeyword(&masterdata.Costume3d{Name: "plain"}, "missing") {
		t.Fatal("nonmatching costume keyword matched")
	}
}

func testLocalGachaBranches(t *testing.T, p *LocalProvider, ctx context.Context) {
	t.Helper()
	if _, err := p.gachas.GetByID(ctx, 0); err == nil {
		t.Fatal("zero gacha id was accepted")
	}
	if gacha, err := p.gachas.GetByID(ctx, 51); err != nil || gacha.Name != "New" {
		t.Fatalf("gacha = %+v, %v", gacha, err)
	}
	if _, err := p.gachas.GetByID(ctx, 999); err == nil {
		t.Fatal("missing gacha was accepted")
	}
	if all := p.gachas.GetAll(ctx); len(all) != 3 || all[0].ID != 52 {
		t.Fatalf("all gachas = %+v", all)
	}
	if card, err := p.gachas.GetCardByID(ctx, 1); err != nil || card.ID != 1 {
		t.Fatalf("gacha card = %+v, %v", card, err)
	}
	if _, err := p.gachas.GetCardByID(ctx, 0); err == nil {
		t.Fatal("zero gacha card id was accepted")
	}
	if name, err := p.gachas.GetCeilItemAssetbundleName(ctx, 7); err != nil || name != "ceil_7" {
		t.Fatalf("ceil asset = %q, %v", name, err)
	}
	if _, err := p.gachas.GetCeilItemAssetbundleName(ctx, 0); err == nil {
		t.Fatal("zero ceil id was accepted")
	}
	if _, err := p.gachas.GetCeilItemAssetbundleName(ctx, 999); err == nil {
		t.Fatal("missing ceil id was accepted")
	}
}

func TestLocalHonorCharacterSkillAndAuxiliaryProviders(t *testing.T) {
	p := newLocalCoverageProvider(t)
	ctx := context.Background()
	testLocalCharacterBranches(t, p, ctx)
	testLocalSkillBranches(t, p, ctx)
	testLocalHonorGroupBranches(t, p, ctx)
	testLocalBondsHonorBranches(t, p, ctx)
	testLocalAuxiliaryProviderBranches(t, p, ctx)
}

func testLocalCharacterBranches(t *testing.T, p *LocalProvider, ctx context.Context) {
	t.Helper()
	if _, err := p.characters.GetByID(ctx, 0); err == nil {
		t.Fatal("zero character id was accepted")
	}
	if character, err := p.characters.GetByID(ctx, 1); err != nil || character.GivenName != "Ichika" {
		t.Fatalf("character = %+v, %v", character, err)
	}
	if _, err := p.characters.GetByID(ctx, 999); err == nil {
		t.Fatal("missing character was accepted")
	}
	if color, ok := p.characters.GetColorCode(ctx, 1); !ok || color != "#112233" {
		t.Fatalf("character color = %q, %t", color, ok)
	}
	if _, ok := p.characters.GetColorCode(ctx, 0); ok {
		t.Fatal("zero character color succeeded")
	}
	if unit, err := p.characters.GetGameCharacterUnit(ctx, 10); err != nil || unit.GameCharacterID != 1 {
		t.Fatalf("character unit = %+v, %v", unit, err)
	}
	if _, err := p.characters.GetGameCharacterUnit(ctx, 0); err == nil {
		t.Fatal("zero character unit id was accepted")
	}
	if _, err := p.characters.GetGameCharacterUnit(ctx, 999); err == nil {
		t.Fatal("missing character unit was accepted")
	}
}

func testLocalSkillBranches(t *testing.T, p *LocalProvider, ctx context.Context) {
	t.Helper()
	if skill, err := p.skills.GetByID(ctx, 1); err != nil || skill.ID != 1 {
		t.Fatalf("skill = %+v, %v", skill, err)
	}
	if skill, err := p.skills.GetByID(ctx, 0); err != nil || skill != nil {
		t.Fatalf("zero skill = %+v, %v", skill, err)
	}
	if _, err := p.skills.GetByID(ctx, 999); err == nil {
		t.Fatal("missing skill was accepted")
	}
	if got := p.skills.FormatDescription(ctx, nil, 1); got != "" {
		t.Fatalf("nil skill description = %q", got)
	}
	characterSkill := &masterdata.Skill{Description: "{{2;c}}"}
	if got := p.skills.FormatDescription(ctx, characterSkill, 1); !strings.Contains(got, "Ichika") {
		t.Fatalf("character skill description = %q", got)
	}
}

func testLocalHonorGroupBranches(t *testing.T, p *LocalProvider, ctx context.Context) {
	t.Helper()
	if _, err := p.honors.GetByID(ctx, 0); err == nil {
		t.Fatal("zero honor id was accepted")
	}
	if honor, err := p.honors.GetByID(ctx, 100); err != nil || honor.Name != "Champion" {
		t.Fatalf("honor = %+v, %v", honor, err)
	}
	if _, err := p.honors.GetByID(ctx, 999); err == nil {
		t.Fatal("missing honor was accepted")
	}
	group, err := p.honors.GetGroupByID(ctx, 200)
	if err != nil || group.BackgroundAssetBundleName == nil || group.FrameName == nil {
		t.Fatalf("birthday honor group = %+v, %v", group, err)
	}
	if cached, ok := p.honors.deriveBirthdayAssetsForGroup(200, "ignored"); !ok || cached.background == "" {
		t.Fatalf("cached birthday assets = %+v, %t", cached, ok)
	}
	if _, err := p.honors.GetGroupByID(ctx, 0); err == nil {
		t.Fatal("zero honor group id was accepted")
	}
	if _, err := p.honors.GetGroupByID(ctx, 999); err == nil {
		t.Fatal("missing honor group was accepted")
	}
	if _, ok := p.honors.deriveBirthdayAssetsForGroup(999, "Nobody"); ok {
		t.Fatal("unknown birthday group derived assets")
	}
	if localBirthdayGroupMatchesCharacter("", &localGameCharacterJSON{ID: 1}) || localBirthdayGroupMatchesCharacter("Ichika", nil) {
		t.Fatal("invalid birthday match succeeded")
	}
}

func testLocalBondsHonorBranches(t *testing.T, p *LocalProvider, ctx context.Context) {
	t.Helper()
	if bond, err := p.honors.GetBondsHonorByID(ctx, 300); err != nil || bond.Name != "Bond" {
		t.Fatalf("bonds honor = %+v, %v", bond, err)
	}
	if _, err := p.honors.GetBondsHonorByID(ctx, 0); err == nil {
		t.Fatal("zero bonds honor id was accepted")
	}
	if word, err := p.honors.GetBondsHonorWordByID(ctx, 301); err != nil || word.Name != "Together" {
		t.Fatalf("bonds word = %+v, %v", word, err)
	}
	if _, err := p.honors.GetBondsHonorWordByID(ctx, 0); err == nil {
		t.Fatal("zero bonds word id was accepted")
	}
	if unit, ok := p.honors.GetGameCharacterUnitByID(ctx, 10); !ok || unit.GameCharacterID != 1 {
		t.Fatalf("honor character unit = %+v, %t", unit, ok)
	}
	if _, ok := p.honors.GetGameCharacterUnitByID(ctx, 0); ok {
		t.Fatal("zero honor character unit id succeeded")
	}
	if eventID := p.honors.GetEventIDByHonorID(ctx, 100); eventID != 10 {
		t.Fatalf("honor event id = %d", eventID)
	}
	if eventID := p.honors.GetEventIDByHonorID(ctx, 0); eventID != 0 {
		t.Fatalf("zero honor event id = %d", eventID)
	}
}

func testLocalAuxiliaryProviderBranches(t *testing.T, p *LocalProvider, ctx context.Context) {
	t.Helper()
	if stamps, err := p.stamps.GetAll(ctx); err != nil || len(stamps) != 1 {
		t.Fatalf("stamps = %+v, %v", stamps, err)
	}
	if lives, err := p.vlives.GetLives(ctx, renderregion.JP); err != nil || len(lives) != 2 || len(lives[1].Schedules) != 1 || len(lives[1].Rewards) != 1 || len(lives[1].Characters) != 1 {
		t.Fatalf("virtual lives = %+v, %v", lives, err)
	}
	if frame, err := p.playerFrames.GetByID(ctx, 1); err != nil || frame.Description != "frame" {
		t.Fatalf("player frame = %+v, %v", frame, err)
	}
	if _, err := p.playerFrames.GetByID(ctx, 0); err == nil {
		t.Fatal("zero player frame id was accepted")
	}
	if group, err := p.playerFrames.GetGroupByID(ctx, 2); err != nil || group.Name != "group" {
		t.Fatalf("player frame group = %+v, %v", group, err)
	}
	if _, err := p.playerFrames.GetGroupByID(ctx, 0); err == nil {
		t.Fatal("zero player frame group id was accepted")
	}
}

func TestLocalEducationAndMySekaiProviders(t *testing.T) {
	p := newLocalCoverageProvider(t)
	ctx := context.Background()
	education := p.education
	testLocalEducationAreaBranches(t, education, ctx)
	testLocalEducationMissionBranches(t, education, ctx)
	testLocalEducationShopBranches(t, education, ctx)
	testLocalMySekaiBranches(t, p)
}

func testLocalEducationAreaBranches(t *testing.T, education *localEducationProvider, ctx context.Context) {
	t.Helper()
	if items := education.GetAreaItems(ctx); len(items) != 1 {
		t.Fatalf("area items = %+v", items)
	}
	if item := education.GetAreaItem(ctx, 1); item == nil || item.Name != "Plant" {
		t.Fatalf("area item = %+v", item)
	}
	if education.GetAreaItem(ctx, 0) != nil || education.GetAreaItem(ctx, 999) != nil {
		t.Fatal("invalid area item resolved")
	}
	if levels := education.GetAreaItemLevels(ctx, 1); len(levels) != 2 {
		t.Fatalf("area levels = %+v", levels)
	}
	if education.GetAreaItemLevels(ctx, 0) != nil {
		t.Fatal("zero area item levels resolved")
	}
	if level := education.GetAreaItemLevel(ctx, 1, 2); level == nil || level.Power1BonusRate != 0.2 {
		t.Fatalf("area level = %+v", level)
	}
	if education.GetAreaItemLevel(ctx, 0, 0) != nil || education.GetAreaItemLevel(ctx, 999, 1) != nil {
		t.Fatal("invalid area level resolved")
	}
	if rank := education.GetCharacterRank(ctx, 1, 5); rank == nil || rank.Power1BonusRate != 0.05 {
		t.Fatalf("character rank = %+v", rank)
	}
	if education.GetCharacterRank(ctx, 0, 0) != nil || education.GetCharacterRank(ctx, 999, 1) != nil {
		t.Fatal("invalid character rank resolved")
	}
}

func testLocalEducationMissionBranches(t *testing.T, education *localEducationProvider, ctx context.Context) {
	t.Helper()
	testLocalEducationBondsAndStyleBranches(t, education, ctx)
	testLocalEducationCharacterMissionBranches(t, education, ctx)
}

func testLocalEducationBondsAndStyleBranches(t *testing.T, education *localEducationProvider, ctx context.Context) {
	t.Helper()
	if bonds := education.GetBonds(ctx); len(bonds) != 1 {
		t.Fatalf("bonds = %+v", bonds)
	}
	if levels := education.GetBondLevels(ctx); len(levels) != 2 || levels[0].Level != 1 {
		t.Fatalf("bond levels = %+v", levels)
	}
	if levels := education.GetCharacterLevels(ctx); len(levels) != 1 {
		t.Fatalf("character levels = %+v", levels)
	}
	if style := education.GetGameCharacterStyle(ctx, 10); style == nil || style.ColorCode != "#112233" {
		t.Fatalf("character style = %+v", style)
	}
	if education.GetGameCharacterStyle(ctx, 0) != nil {
		t.Fatal("zero character style resolved")
	}
}

func testLocalEducationCharacterMissionBranches(t *testing.T, education *localEducationProvider, ctx context.Context) {
	t.Helper()
	if missions := education.GetCharacterMissions(ctx, 1); len(missions) != 1 {
		t.Fatalf("character missions = %+v", missions)
	}
	if education.GetCharacterMissions(ctx, 0) != nil {
		t.Fatal("zero character missions resolved")
	}
	if groups := education.GetCharacterMissionParameterGroups(ctx, 10); len(groups) != 2 {
		t.Fatalf("mission groups = %+v", groups)
	}
	if education.GetCharacterMissionParameterGroups(ctx, 0) != nil {
		t.Fatal("zero mission group resolved")
	}
	if requirements, maxPlay := education.GetLeaderMissionRequirements(ctx); len(requirements) != 2 || requirements[0].Seq != 1 || maxPlay != 20 {
		t.Fatalf("leader requirements = %+v, max=%d", requirements, maxPlay)
	}
	if gate := education.GetMysekaiGateLevel(ctx, 1, 2); gate == nil || gate.PowerBonusRate != 0.2 {
		t.Fatalf("gate level = %+v", gate)
	}
	if education.GetMysekaiGateLevel(ctx, 0, 0) != nil || education.GetMysekaiGateLevel(ctx, 999, 1) != nil {
		t.Fatal("invalid gate level resolved")
	}
}

func testLocalEducationShopBranches(t *testing.T, education *localEducationProvider, ctx context.Context) {
	t.Helper()
	if shop := education.GetShopItemByResourceBoxID(ctx, 50); shop == nil || len(shop.Costs) != 1 || shop.Costs[0].Quantity != 10 {
		t.Fatalf("shop item = %+v", shop)
	}
	if education.GetShopItemByResourceBoxID(ctx, 0) != nil {
		t.Fatal("zero shop item resolved")
	}
	if shops := education.GetShopItems(ctx); len(shops) != 1 {
		t.Fatalf("shop items = %+v", shops)
	}
}

func testLocalMySekaiBranches(t *testing.T, p *LocalProvider) {
	t.Helper()
	if !p.mysekai.Configured() {
		t.Fatal("local mysekai provider is not configured")
	}
	if list := p.mysekai.LoadList("mysekaiFixture.json"); len(list) != 3 {
		t.Fatalf("mysekai list = %+v", list)
	}
	if byID := p.mysekai.LoadMapByID("mysekaiFixture.json"); len(byID) != 2 || byID[1] == nil || byID[2] == nil {
		t.Fatalf("mysekai map = %+v", byID)
	}
	if target := map[string]any{}; !p.mysekai.LoadObject("mysekaiObject.json", &target) || target["name"] != "object" {
		t.Fatalf("mysekai object = %+v", target)
	}
	if p.mysekai.LoadList("missing.json") != nil || p.mysekai.LoadMapByID("missing.json") != nil {
		t.Fatal("missing mysekai fixture loaded")
	}
	if p.mysekai.LoadObject("missing.json", &map[string]any{}) {
		t.Fatal("missing mysekai object loaded")
	}
}
