package deck

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"haruki-cloud/internal/i18n"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/testutil"
	"haruki-cloud/utils/usererror"
)

func TestValidateFixedCardCharacters(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})
	_, source, _ := controller.resolveCardSource(renderregion.JP)
	cards := source.(*testCardSource)
	cards.cards[1003].CharacterID = 1
	cards.cards[1004].CharacterID = 2
	cards.cards[1005].CharacterID = 1
	cards.cards[1006].CharacterID = 26
	cards.cards[1007].CharacterID = 26

	for _, tc := range []struct {
		name     string
		ids      []int
		recType  string
		liveType string
		want     []i18n.Message
	}{
		{name: "no fixed cards"},
		{name: "single card", ids: []int{1001}},
		{name: "different characters", ids: []int{1001, 1002}},
		{name: "same ID is not multiple cards", ids: []int{1001, 1001}},
		{name: "missing card retains upstream validation", ids: []int{1001, 9999}},
		{name: "challenge allows same character", ids: []int{1001, 1003}, recType: "challenge"},
		{name: "challenge auto allows same character", ids: []int{1001, 1003}, liveType: "challenge_auto"},
		{
			name: "all groups in input order", ids: []int{1002, 1001, 1003, 1004, 1005},
			want: []i18n.Message{
				i18n.M("deck.fixed.character_conflict", i18n.Data{"Name": controller.resolveCharacterName(renderregion.JP, 2), "CardIDs": "1002, 1004"}),
				i18n.M("deck.fixed.character_conflict", i18n.Data{"Name": controller.resolveCharacterName(renderregion.JP, 1), "CardIDs": "1001, 1003, 1005"}),
			},
		},
		{
			name: "missing name uses character ID", ids: []int{1006, 1007},
			want: []i18n.Message{
				i18n.M("deck.fixed.character_conflict", i18n.Data{"Name": i18n.M("deck.fixed.character_label", i18n.Data{"ID": 26}), "CardIDs": "1006, 1007"}),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := controller.validateFixedCardCharacters(renderregion.JP, tc.recType, map[string]any{
				"fixed_cards": tc.ids, "live_type": tc.liveType,
			})
			if tc.want == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			typed := testutil.RequireUserError(t, err, usererror.CodeInput, "deck.fixed.character_conflicts")
			if !reflect.DeepEqual(typed.Message.Data["Conflicts"], tc.want) {
				t.Fatalf("got %#v, want %#v", typed.Message.Data["Conflicts"], tc.want)
			}
		})
	}
}

func TestBuildAutoRecommendRejectsCardCharacterConflictBeforeServiceCall(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	controller := newTestDeckControllerWithMeta(t, RecommendConfig{
		Enabled: true, ServiceBaseURL: server.URL, MasterdataDir: t.TempDir(),
	}, &testMusicMetaSource{data: []byte(`{}`)})
	_, source, _ := controller.resolveCardSource(renderregion.JP)
	source.(*testCardSource).cards[1002].CharacterID = 1
	_, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region: "jp", RecommendType: "no_event", FixedCards: []int{1001, 1002},
	})
	testutil.RequireUserError(t, err, usererror.CodeInput, "deck.fixed.character_conflicts")
	if calls.Load() != 0 {
		t.Fatalf("invalid deck made %d service calls", calls.Load())
	}
}

func TestValidateFixedCardCharactersUsesRequestedRegion(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})
	controller.RegisterCardSource(&testCardSource{
		region: renderregion.CN,
		cards: map[int]*masterdata.Card{
			1001: {ID: 1001, CharacterID: 2, AssetBundleName: "cn_1001"},
			1002: {ID: 1002, CharacterID: 2, AssetBundleName: "cn_1002"},
		},
		characters: map[int]*masterdata.Character{
			2: {ID: 2, FirstName: "天马", GivenName: "咲希"},
		},
	})
	err := controller.validateFixedCardCharacters(renderregion.CN, "event", map[string]any{"fixed_cards": []int{1001, 1002}})
	typed := testutil.RequireUserError(t, err, usererror.CodeInput, "deck.fixed.character_conflicts")
	conflicts := typed.Message.Data["Conflicts"].([]i18n.Message)
	if len(conflicts) != 1 || conflicts[0].Data["Name"] != controller.resolveCharacterName(renderregion.CN, 2) {
		t.Fatalf("unexpected CN card conflict: %#v", conflicts)
	}
}
