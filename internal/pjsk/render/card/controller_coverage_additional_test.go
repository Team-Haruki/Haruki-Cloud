package card

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

type controllerCoverageContextKey struct{}

type controllerCoverageContextSource struct {
	*lookupTestSource
	bound context.Context
}

func (s *controllerCoverageContextSource) WithContext(ctx context.Context) DataSource {
	clone := *s
	clone.bound = ctx
	return &clone
}

func TestControllerMergeNicknamesBranches(t *testing.T) {
	var nilController *Controller
	nilController.MergeNicknames(map[string]int{"new": 1})

	controller := NewController(&lookupTestSource{}, nil, nil, nil)
	controller.MergeNicknames(nil)
	controller.nicknames = nil
	controller.MergeNicknames(map[string]int{
		"  coverage alias  ": 5,
		"":                   6,
		"invalid":            0,
	})
	key := normalizeNicknameQuery("coverage alias")
	if controller.nicknames[key] != 5 {
		t.Fatalf("merged nicknames = %+v", controller.nicknames)
	}
	controller.MergeNicknames(map[string]int{"coverage alias": 6})
	if controller.nicknames[key] != 5 {
		t.Fatalf("conflicting nickname overwrote existing value: %+v", controller.nicknames)
	}
	controller.MergeNicknames(map[string]int{"coverage alias": 5})
}

func TestControllerWithContextClonesContextualAndPlainSources(t *testing.T) {
	var nilController *Controller
	if nilController.WithContext(context.Background()) != nil {
		t.Fatal("nil controller produced a clone")
	}

	plain := NewController(&lookupTestSource{}, nil, nil, nil)
	plainClone := plain.WithContext(context.Background())
	if plainClone == nil || plainClone == plain {
		t.Fatalf("plain clone = %#v", plainClone)
	}

	source := &controllerCoverageContextSource{lookupTestSource: &lookupTestSource{}}
	controller := NewController(source, nil, nil, nil)
	ctx := context.WithValue(context.Background(), controllerCoverageContextKey{}, "bound")
	clone := controller.WithContext(ctx)
	if clone == nil || clone == controller || clone.ctx != ctx {
		t.Fatalf("contextual clone = %#v", clone)
	}
	resolved, ok := clone.sources.SourceForRegion(renderregion.JP)
	if !ok {
		t.Fatal("contextual source was not registered")
	}
	contextual, ok := resolved.(*controllerCoverageContextSource)
	if !ok || contextual.bound != ctx || contextual == source {
		t.Fatalf("resolved contextual source = %#v", resolved)
	}
}

func TestControllerRenderEntryPointsSuccessAndErrors(t *testing.T) {
	source := &lookupTestSource{
		card: &masterdata.Card{
			ID: 1, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute",
			Prefix: "coverage", AssetBundleName: "card_coverage", ReleaseAt: time.Now().Add(-time.Hour).UnixMilli(),
		},
		characters: map[int]*masterdata.Character{5: {ID: 5, FirstName: "花里", GivenName: "实乃理", Unit: "idol"}},
		unitByCard: map[int]string{1: "idol"},
	}
	withoutDrawing := NewController(source, nil, nil, nil)
	if _, err := withoutDrawing.RenderCardDetail(Query{Query: "1"}); err == nil {
		t.Fatal("detail render without drawing client succeeded")
	}
	if _, err := withoutDrawing.RenderCardList(ListRequest{CardIDs: []int{1}}); err == nil {
		t.Fatal("list render without drawing client succeeded")
	}
	if _, err := withoutDrawing.RenderCardBox([]Query{{Query: "1"}}); err == nil {
		t.Fatal("box render without drawing client succeeded")
	}

	var mu sync.Mutex
	seen := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.URL.Path]++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PNG:" + r.URL.Path))
	}))
	t.Cleanup(server.Close)
	client := drawing.NewHarukiDrawingClient(server.URL, drawing.WithRetryCount(0), drawing.WithTimeout(time.Second))
	controller := NewController(source, nil, client, nil).WithContext(context.Background())

	tests := []struct {
		name string
		path string
		run  func() ([]byte, error)
	}{
		{name: "detail", path: "/api/pjsk/card/detail", run: func() ([]byte, error) {
			return controller.RenderCardDetail(Query{Query: "1", Region: "jp"})
		}},
		{name: "list", path: "/api/pjsk/card/list", run: func() ([]byte, error) {
			return controller.RenderCardList(ListRequest{CardIDs: []int{1}, Region: "jp"})
		}},
		{name: "box", path: "/api/pjsk/card/box", run: func() ([]byte, error) {
			return controller.RenderCardBox([]Query{{Query: "1", Region: "jp"}})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := tt.run()
			if err != nil {
				t.Fatalf("render error: %v", err)
			}
			if string(body) != "PNG:"+tt.path {
				t.Fatalf("render body = %q", body)
			}
		})
	}
	mu.Lock()
	gotSeen := map[string]int{
		"/api/pjsk/card/detail": seen["/api/pjsk/card/detail"],
		"/api/pjsk/card/list":   seen["/api/pjsk/card/list"],
		"/api/pjsk/card/box":    seen["/api/pjsk/card/box"],
	}
	mu.Unlock()
	if want := map[string]int{"/api/pjsk/card/detail": 1, "/api/pjsk/card/list": 1, "/api/pjsk/card/box": 1}; !reflect.DeepEqual(gotSeen, want) {
		t.Fatalf("drawing endpoints = %+v, want %+v", gotSeen, want)
	}

	if _, err := controller.RenderCardDetail(Query{Query: "not a card"}); err == nil {
		t.Fatal("invalid detail query rendered")
	}
	if _, err := controller.RenderCardList(ListRequest{}); err == nil {
		t.Fatal("empty list query rendered")
	}
	if _, err := controller.RenderCardBox(nil); err == nil {
		t.Fatal("empty box query rendered")
	}
}

func TestControllerExplicitCardIDsSkipInvalidDuplicateMissingAndFuture(t *testing.T) {
	now := time.Now().UnixMilli()
	source := &lookupTestSource{cards: []*masterdata.Card{
		{ID: 1, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", AssetBundleName: "card_1", ReleaseAt: now - 1000},
		{ID: 2, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", AssetBundleName: "card_2", ReleaseAt: now + 60_000},
	}}
	controller := NewController(source, nil, nil, nil)
	title := "coverage title"
	profile := &drawing.DetailedProfileCardRequest{ID: "user"}
	req, err := controller.BuildCardListRequest(ListRequest{
		CardIDs:         []int{0, -1, 1, 1, 999, 2},
		Region:          "jp",
		Title:           &title,
		DetailedProfile: profile,
	})
	if err != nil {
		t.Fatalf("BuildCardListRequest() error = %v", err)
	}
	if len(req.Cards) != 1 || req.Cards[0].CardID != 1 || req.Title != &title || req.UserInfo != profile {
		t.Fatalf("filtered explicit list = %+v", req)
	}
	if _, err := controller.BuildCardListRequest(ListRequest{CardIDs: []int{0, 999, 2}, Region: "jp"}); err == nil {
		t.Fatal("explicit list without visible cards succeeded")
	}
	if _, _, err := controller.buildCardListRenderRequest(ListRequest{CardIDs: []int{999}, Region: "jp"}); err == nil {
		t.Fatal("invalid render-list request succeeded")
	}
}

func TestControllerCardBoxFullListFailuresAndTitle(t *testing.T) {
	controller := NewController(&lookupTestSource{}, nil, nil, nil)
	if _, err := controller.BuildCardBoxRequest(nil); err == nil {
		t.Fatal("empty box query succeeded")
	}

	errSource := &lookupTestSource{allowEmptyFilter: true}
	errSource.filterFunc = func(*PjskCardQueryInfo) ([]*masterdata.Card, error) {
		return nil, fmt.Errorf("filter unavailable")
	}
	if _, err := NewController(errSource, nil, nil, nil).BuildCardBoxRequest([]Query{{Region: "jp"}}); err == nil {
		t.Fatal("failed full-list filter succeeded")
	}

	futureSource := &lookupTestSource{
		cards:            []*masterdata.Card{{ID: 2, CharacterID: 5, CardRarityType: "rarity_4", ReleaseAt: time.Now().Add(time.Hour).UnixMilli()}},
		allowEmptyFilter: true,
	}
	if _, err := NewController(futureSource, nil, nil, nil).BuildCardBoxRequest([]Query{{Region: "jp"}}); err == nil {
		t.Fatal("future-only full list succeeded")
	}

	releasedSource := &lookupTestSource{
		cards: []*masterdata.Card{
			nil,
			{ID: 3, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", AssetBundleName: "card_3", ReleaseAt: time.Now().Add(-time.Hour).UnixMilli()},
		},
		allowEmptyFilter: true,
	}
	title := "box title"
	req, err := NewController(releasedSource, nil, nil, nil).BuildCardBoxRequest([]Query{{Region: "jp", Title: &title}})
	if err != nil {
		t.Fatalf("released full list error = %v", err)
	}
	if req.Title != &title || len(req.Cards) != 1 || req.Cards[0].Card.CardID != 3 {
		t.Fatalf("released full list = %+v", req)
	}
}
