package card

import (
	"slices"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestControllerQuerySummaryEntryPoints(t *testing.T) {
	beforeTraining := false
	source := &lookupTestSource{region: renderregion.CN}
	controller := NewController(source, nil, nil, nil)

	tests := []struct {
		name string
		got  func() string
		want string
	}{
		{
			name: "detail trims and describes card ID",
			got: func() string {
				return controller.SummaryForDetail(Query{Region: " cn ", Query: " 1001 "})
			},
			want: "CN / 查卡 / 卡牌ID1001",
		},
		{
			name: "list describes explicit card IDs",
			got: func() string {
				return controller.SummaryForList(ListRequest{Region: "cn", CardIDs: []int{1001, 1002}})
			},
			want: "CN / 卡牌列表 / 卡牌ID 1001, 1002",
		},
		{
			name: "box includes presentation options",
			got: func() string {
				return controller.SummaryForBox(Query{
					Region:           "cn",
					Query:            " 4星 ",
					StrictFilterOnly: true,
					ShowID:           true,
					ShowBox:          true,
					UseAfterTraining: &beforeTraining,
				})
			},
			want: "CN / 卡牌一览 / 四星 / 显示ID / 显示持有 / 花前",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.got(); got != tt.want {
				t.Fatalf("summary = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestControllerShouldShowQuerySummary(t *testing.T) {
	controller := NewController(nil, nil, nil, nil)

	if controller.ShouldShowSummaryForDetail(Query{Query: "4星"}) {
		t.Fatal("detail view should never show a query summary")
	}

	listTests := []struct {
		name  string
		query ListRequest
		want  bool
	}{
		{name: "multiple explicit IDs", query: ListRequest{CardIDs: []int{1, 2}}, want: true},
		{name: "one explicit ID", query: ListRequest{CardIDs: []int{1}}, want: false},
		{name: "filter query", query: ListRequest{Query: " 4星 "}, want: true},
		{name: "single card query", query: ListRequest{Query: "1001"}, want: false},
		{name: "strict parser rejects ID", query: ListRequest{Query: "1001", StrictFilterOnly: true}, want: false},
	}
	for _, tt := range listTests {
		t.Run("list/"+tt.name, func(t *testing.T) {
			if got := controller.ShouldShowSummaryForList(tt.query); got != tt.want {
				t.Fatalf("ShouldShowSummaryForList() = %t, want %t", got, tt.want)
			}
		})
	}

	boxTests := []struct {
		name  string
		query Query
		want  bool
	}{
		{name: "empty query means all cards", query: Query{Query: "  "}, want: true},
		{name: "filter query", query: Query{Query: "4星"}, want: true},
		{name: "single card query", query: Query{Query: "1001"}, want: false},
		{name: "unparseable query", query: Query{Query: "not-a-card-query"}, want: false},
	}
	for _, tt := range boxTests {
		t.Run("box/"+tt.name, func(t *testing.T) {
			if got := controller.ShouldShowSummaryForBox(tt.query); got != tt.want {
				t.Fatalf("ShouldShowSummaryForBox() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestFormatQuerySummaryFallbacks(t *testing.T) {
	afterTraining := true
	controllerWithoutSource := NewController(nil, nil, nil, nil)
	var nilController *Controller

	tests := []struct {
		name       string
		controller *Controller
		region     string
		mode       string
		rawQuery   string
		after      *bool
		want       string
	}{
		{
			name:       "nil controller uses normalized region and default mode label",
			controller: nilController,
			region:     " jp ",
			mode:       "unsupported",
			want:       "JP / 查卡",
		},
		{
			name:       "missing source falls back to requested region",
			controller: controllerWithoutSource,
			region:     "tw",
			mode:       "list",
			rawQuery:   "not-a-card-query",
			want:       "TW / 卡牌列表 / not-a-card-query",
		},
		{
			name:       "unknown region preserves caller label",
			controller: nilController,
			region:     "unknown",
			mode:       "list",
			rawQuery:   "not-a-card-query",
			want:       "UNKNOWN / 卡牌列表 / not-a-card-query",
		},
		{
			name:       "blank region is omitted",
			controller: nilController,
			region:     " ",
			mode:       "list",
			rawQuery:   "not-a-card-query",
			want:       "卡牌列表 / not-a-card-query",
		},
		{
			name:       "after-training box omits before-training marker",
			controller: nilController,
			region:     "jp",
			mode:       "box",
			after:      &afterTraining,
			want:       "JP / 卡牌一览 / 全部已上线卡牌",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.controller.formatQuerySummary(tt.region, tt.mode, tt.rawQuery, false, false, false, tt.after, nil)
			if got != tt.want {
				t.Fatalf("formatQuerySummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestQueryUsesMultiCardSelection(t *testing.T) {
	customController := &Controller{nicknames: map[string]int{"hero": 99}}
	var nilController *Controller

	tests := []struct {
		name       string
		controller *Controller
		mode       string
		rawQuery   string
		strict     bool
		want       bool
	}{
		{name: "empty box", controller: nilController, mode: "box", rawQuery: " ", want: true},
		{name: "empty list", controller: nilController, mode: "list", rawQuery: " ", want: false},
		{name: "default parser filter", controller: nilController, mode: "list", rawQuery: "4星", want: true},
		{name: "custom nickname filter", controller: customController, mode: "list", rawQuery: "hero 4星", want: true},
		{name: "strict filter", controller: customController, mode: "list", rawQuery: "4星", strict: true, want: true},
		{name: "detail parser filter", controller: customController, mode: "detail", rawQuery: "4星", want: true},
		{name: "single ID", controller: customController, mode: "list", rawQuery: "1001", want: false},
		{name: "parse error", controller: customController, mode: "list", rawQuery: "not-a-card-query", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.controller.queryUsesMultiCardSelection(tt.mode, tt.rawQuery, tt.strict); got != tt.want {
				t.Fatalf("queryUsesMultiCardSelection() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestDescribeQueryParts(t *testing.T) {
	customController := &Controller{nicknames: map[string]int{"hero": 99}}
	var nilController *Controller

	tests := []struct {
		name       string
		controller *Controller
		mode       string
		rawQuery   string
		strict     bool
		cardIDs    []int
		want       []string
	}{
		{name: "explicit IDs", controller: nilController, mode: "list", cardIDs: []int{10, 20}, want: []string{"卡牌ID 10, 20"}},
		{name: "empty invalid IDs", controller: nilController, mode: "list", cardIDs: []int{0, -1}},
		{name: "empty box", controller: nilController, mode: "box", want: []string{"全部已上线卡牌"}},
		{name: "empty detail", controller: nilController, mode: "detail"},
		{name: "strict filter", controller: nilController, mode: "list", rawQuery: " 4星 ", strict: true, want: []string{"四星"}},
		{name: "detail ID", controller: nilController, mode: "detail", rawQuery: "1001", want: []string{"卡牌ID1001"}},
		{name: "preferred filter", controller: nilController, mode: "list", rawQuery: "4星", want: []string{"四星"}},
		{name: "custom nickname sequence", controller: customController, mode: "detail", rawQuery: "hero-2", want: []string{"hero最新第2张"}},
		{name: "strict parse failure preserves query", controller: customController, mode: "list", rawQuery: "1001", strict: true, want: []string{"1001"}},
		{name: "parse failure preserves query", controller: customController, mode: "list", rawQuery: "not-a-card-query", want: []string{"not-a-card-query"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.controller.describeQueryParts(tt.mode, tt.rawQuery, tt.strict, tt.cardIDs, nil)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("describeQueryParts() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDescribeExplicitCardIDs(t *testing.T) {
	tests := []struct {
		name string
		ids  []int
		want []string
	}{
		{name: "none"},
		{name: "only invalid", ids: []int{-1, 0}},
		{name: "single", ids: []int{-1, 42, 0}, want: []string{"卡牌ID42"}},
		{name: "up to five", ids: []int{1, 2, 3, 4, 5}, want: []string{"卡牌ID 1, 2, 3, 4, 5"}},
		{name: "more than five", ids: []int{1, 2, 3, 4, 5, 6}, want: []string{"6张指定卡牌"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := describeExplicitCardIDs(tt.ids); !slices.Equal(got, tt.want) {
				t.Fatalf("describeExplicitCardIDs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDescribeCardQueryInfo(t *testing.T) {
	source := &lookupTestSource{characters: map[int]*masterdata.Character{
		5: {ID: 5, FirstName: "花里", GivenName: "实乃理"},
		6: {ID: 6, FirstName: "桐谷", GivenName: "遥"},
	}}
	nicknames := map[string]int{"mnr": 5, "haruka": 6}

	tests := []struct {
		name string
		info *PjskCardQueryInfo
		want []string
	}{
		{name: "nil info"},
		{name: "ID", info: &PjskCardQueryInfo{Type: QueryTypeID, Value: 42}, want: []string{"卡牌ID42"}},
		{name: "invalid ID uses original", info: &PjskCardQueryInfo{Type: QueryTypeID, Original: " raw ID "}, want: []string{"raw ID"}},
		{name: "global latest", info: &PjskCardQueryInfo{Type: QueryTypeLatest, Sequence: -3}, want: []string{"全局最新第3张"}},
		{name: "invalid latest uses original", info: &PjskCardQueryInfo{Type: QueryTypeLatest, Sequence: 1, Original: "latest"}, want: []string{"latest"}},
		{name: "character latest", info: &PjskCardQueryInfo{Type: QueryTypeSeq, CharacterID: 5, Sequence: -2}, want: []string{"花里实乃理最新第2张"}},
		{name: "character ordinal", info: &PjskCardQueryInfo{Type: QueryTypeSeq, CharacterID: 6, Sequence: 2}, want: []string{"桐谷遥第2张"}},
		{name: "invalid character sequence uses original", info: &PjskCardQueryInfo{Type: QueryTypeSeq, CharacterID: 6, Original: "haruka"}, want: []string{"haruka"}},
		{
			name: "complete filter",
			info: &PjskCardQueryInfo{
				Type:        QueryTypeFilter,
				EventID:     7,
				BanCharID:   5,
				BanSeq:      2,
				CharacterID: 6,
				Attr:        "cute",
				SkillIDs:    []int{4},
				SkillType:   "score_up",
				MainUnit:    "piapro",
				SupportUnit: "idol",
				Rarity:      "rarity_4",
				SupplyType:  SupplyFes,
				Year:        2025,
			},
			want: []string{"event7", "花里实乃理2箱活", "桐谷遥", "粉", "大分", "分卡", "MMJV", "四星", "fes", "2025年"},
		},
		{name: "empty filter", info: &PjskCardQueryInfo{Type: QueryTypeFilter}},
		{name: "unknown type uses original", info: &PjskCardQueryInfo{Type: QueryTypeUnknown, Original: " raw "}, want: []string{"raw"}},
		{name: "unknown type without original", info: &PjskCardQueryInfo{Type: QueryTypeUnknown}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := describeCardQueryInfo(tt.info, source, nicknames); !slices.Equal(got, tt.want) {
				t.Fatalf("describeCardQueryInfo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSummaryCharacterLabel(t *testing.T) {
	source := &lookupTestSource{characters: map[int]*masterdata.Character{
		5: {ID: 5, FirstName: "花里", GivenName: "实乃理"},
		6: {ID: 6, FirstName: " ", GivenName: ""},
	}}

	tests := []struct {
		name      string
		source    DataSource
		nicknames map[string]int
		character int
		want      string
	}{
		{name: "invalid character", source: source, character: 0, want: ""},
		{name: "masterdata name", source: source, nicknames: map[string]int{"mnr": 5}, character: 5, want: "花里实乃理"},
		{name: "empty masterdata name falls back", source: source, nicknames: map[string]int{"len": 6}, character: 6, want: "len"},
		{
			name:   "best nickname is stable",
			source: nil,
			nicknames: map[string]int{
				"长昵称": 8,
				"zz":  8,
				"b":   8,
				"a":   8,
				" ":   8,
				"x":   9,
			},
			character: 8,
			want:      "a",
		},
		{name: "numeric fallback", source: nil, nicknames: map[string]int{"other": 1}, character: 99, want: "角色99"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := summaryCharacterLabel(tt.source, tt.nicknames, tt.character); got != tt.want {
				t.Fatalf("summaryCharacterLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBetterSummaryNickname(t *testing.T) {
	tests := []struct {
		name      string
		candidate string
		current   string
		want      bool
	}{
		{name: "ASCII beats non-ASCII", candidate: "long", current: "短", want: true},
		{name: "non-ASCII loses to ASCII", candidate: "短", current: "long", want: false},
		{name: "shorter wins", candidate: "a", current: "bb", want: true},
		{name: "longer loses", candidate: "aaa", current: "bb", want: false},
		{name: "lexically earlier wins", candidate: "aa", current: "ab", want: true},
		{name: "lexically later loses", candidate: "ac", current: "ab", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := betterSummaryNickname(tt.candidate, tt.current); got != tt.want {
				t.Fatalf("betterSummaryNickname() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestSummaryStringLabels(t *testing.T) {
	tests := []struct {
		name  string
		label func(string) string
		items map[string]string
	}{
		{
			name:  "mode",
			label: cardSummaryModeLabel,
			items: map[string]string{"detail": "查卡", "list": "卡牌列表", "box": "卡牌一览", "unknown": "查卡"},
		},
		{
			name:  "attribute",
			label: summaryAttributeLabel,
			items: map[string]string{" cute ": "粉", "cool": "蓝", "pure": "绿", "happy": "橙", "mysterious": "紫", "unknown": ""},
		},
		{
			name:  "skill type",
			label: summarySkillTypeLabel,
			items: map[string]string{" life_recovery ": "奶卡", "score_up": "分卡", "judgment_up": "判卡", "unknown": ""},
		},
		{
			name:  "attached virtual singer unit",
			label: summaryAttachedVSLabel,
			items: map[string]string{" light_sound ": "LNV", "idol": "MMJV", "street": "VBSV", "theme_park": "WSV", "school_refusal": "25HV", "unknown": ""},
		},
		{
			name:  "unit",
			label: summaryUnitLabel,
			items: map[string]string{" light_sound ": "L/N", "idol": "MMJ", "street": "VBS", "theme_park": "WS", "school_refusal": "25H", "piapro": "VS", "unknown": ""},
		},
		{
			name:  "rarity",
			label: summaryRarityLabel,
			items: map[string]string{" rarity_4 ": "四星", "rarity_3": "三星", "rarity_2": "二星", "rarity_1": "一星", "rarity_birthday": "生日", "unknown": ""},
		},
		{
			name:  "supply",
			label: summarySupplyLabel,
			items: map[string]string{
				" " + SupplyFes + " ": "fes",
				SupplyCFes:            "cfes",
				SupplyBFes:            "bfes",
				SupplyWL:              "wl限定",
				SupplyCollab:          "联动限定",
				SupplyLimited:         "限定",
				SupplyNormal:          "非限",
				SupplyBirthday:        "生日",
				"unknown":             "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for input, want := range tt.items {
				if got := tt.label(input); got != want {
					t.Errorf("label(%q) = %q, want %q", input, got, want)
				}
			}
		})
	}
}

func TestSummaryDetailedSkillLabel(t *testing.T) {
	tests := []struct {
		name string
		ids  []int
		want string
	}{
		{name: "none", want: ""},
		{name: "large score", ids: []int{4}, want: "大分"},
		{name: "perfect score", ids: []int{11}, want: "P分"},
		{name: "life score", ids: []int{12}, want: "血分"},
		{name: "judgment score", ids: []int{13}, want: "判分"},
		{name: "unit score", ids: []int{15, 16, 17, 18, 19}, want: "团分"},
		{name: "generic IDs skip invalid", ids: []int{0, 2, -1, 7}, want: "技能2,7"},
		{name: "only invalid IDs", ids: []int{0, -1}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := summaryDetailedSkillLabel(tt.ids); got != tt.want {
				t.Fatalf("summaryDetailedSkillLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSummaryUnitFilterLabel(t *testing.T) {
	tests := []struct {
		name string
		info *PjskCardQueryInfo
		want string
	}{
		{name: "nil", info: nil, want: ""},
		{name: "original virtual singer", info: &PjskCardQueryInfo{MainUnit: "piapro", SupportUnit: "none"}, want: "原V"},
		{name: "attached virtual singer", info: &PjskCardQueryInfo{MainUnit: "piapro", SupportUnit: "street"}, want: "VBSV"},
		{name: "unknown attachment falls back to unit", info: &PjskCardQueryInfo{MainUnit: "piapro", SupportUnit: "unknown", Unit: "idol"}, want: "MMJ"},
		{name: "pure original unit", info: &PjskCardQueryInfo{MainUnit: "theme_park", SupportUnit: "none"}, want: "纯WS"},
		{name: "unknown pure unit falls back", info: &PjskCardQueryInfo{MainUnit: "unknown", SupportUnit: "none", Unit: "piapro"}, want: "VS"},
		{name: "mixed main unit falls back", info: &PjskCardQueryInfo{MainUnit: "idol", SupportUnit: "street", Unit: "light_sound"}, want: "L/N"},
		{name: "ordinary unit", info: &PjskCardQueryInfo{Unit: "school_refusal"}, want: "25H"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := summaryUnitFilterLabel(tt.info); got != tt.want {
				t.Fatalf("summaryUnitFilterLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFilterNonEmptyStrings(t *testing.T) {
	got := filterNonEmptyStrings([]string{"  JP ", "", "  ", "查卡", " 卡牌ID1 "})
	want := []string{"JP", "查卡", "卡牌ID1"}
	if !slices.Equal(got, want) {
		t.Fatalf("filterNonEmptyStrings() = %v, want %v", got, want)
	}

	if got := filterNonEmptyStrings(nil); got == nil || len(got) != 0 {
		t.Fatalf("filterNonEmptyStrings(nil) = %#v, want non-nil empty slice", got)
	}
}
