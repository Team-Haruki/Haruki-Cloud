package requestbuilder

import (
	"context"
	"strings"
	"testing"
	"time"

	sekaidb "haruki-cloud/database/sekai"
	json "haruki-cloud/internal/jsonutil"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
)

func TestBirthdayCharacterMatchingHelpers(t *testing.T) {
	rows := []*sekaidb.Gamecharacter{
		nil,
		{GameID: 0, FirstName: "ignored"},
		{GameID: 2, FirstName: "天马", GivenName: "咲希", FirstNameEnglish: "Tenma", GivenNameEnglish: "Saki"},
		{GameID: 2, FirstName: "天马", GivenName: "咲希"},
		{GameID: 3, FirstName: "望月", GivenName: "穗波", FirstNameEnglish: "Mochizuki", GivenNameEnglish: "Honami"},
	}
	for _, query := range []string{"天马咲希", "天马 咲希", "TENMASAKI", " Saki "} {
		ids := matchBirthdayCharacterIDs(rows, query)
		if len(ids) != 1 || ids[0] != 2 {
			t.Fatalf("match %q = %v", query, ids)
		}
	}
	if got := matchBirthdayCharacterIDs(rows, "missing"); got != nil {
		t.Fatalf("missing match = %v", got)
	}
	if got := matchBirthdayCharacterIDs(rows, " "); got != nil {
		t.Fatalf("empty match = %v", got)
	}
	if birthdayCharacterMatches(rows[2], "not-a-name") {
		t.Fatal("unrelated character name matched")
	}
	names := birthdayCharacterNames(&sekaidb.Gamecharacter{FirstName: "Miku", GivenName: "Miku", FirstNameEnglish: "miku"})
	if len(names) != 3 {
		t.Fatalf("deduplicated character names = %#v", names)
	}
	values := []string{"Miku"}
	appendBirthdayCharacterName(&values, " miku ")
	appendBirthdayCharacterName(&values, "")
	if len(values) != 1 || normalizeBirthdayCharacterText("  Hatsune   Miku ") != "hatsunemiku" {
		t.Fatalf("normalized names = %#v", values)
	}
}

func TestBirthdayDateRegionAndPathHelpers(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	next := nextBirthdayTime(renderregion.JP, 8, 11, now)
	if next.Year() != 2027 || next.Location() != birthdayDisplayLocation {
		t.Fatalf("next birthday = %v", next)
	}
	if birthdayRegionLocation(renderregion.Unknown).String() != "UTC+9" {
		t.Fatalf("unknown region location = %v", birthdayRegionLocation(renderregion.Unknown))
	}
	if birthdayRegionName(renderregion.Unknown) != "日服" || birthdayRegionName(renderregion.Value("xx")) != "XX" || birthdayRegionName(renderregion.CN) != "国服" {
		t.Fatal("birthday region names are incorrect")
	}
	if !isBirthdayFifthAnniv(renderregion.JP) || isBirthdayFifthAnniv(renderregion.EN) {
		t.Fatal("fifth anniversary region classification failed")
	}
	if birthdayDaysUntil(now, now.Add(-time.Hour)) != 0 || birthdayDaysUntil(now, now.Add(49*time.Hour)) != 2 {
		t.Fatal("birthday day countdown failed")
	}
	event := buildBirthdayEventTime(now, now.Add(30*time.Second))
	if event.EndAt != now.Add(30*time.Second).UnixMilli() {
		t.Fatalf("short event end = %d", event.EndAt)
	}
	event = buildBirthdayEventTime(now, now.Add(2*time.Hour))
	if event.EndAt != now.Add(119*time.Minute).UnixMilli() {
		t.Fatalf("normal event end = %d", event.EndAt)
	}

	if birthdayRelativePath(nil, "/absolute/path") != "/absolute/path" || birthdayRelativePath(&renderapp.App{}, "") != "" {
		t.Fatal("nil birthday relative path changed")
	}
	helper := assets.NewAssetHelper(t.TempDir(), nil)
	app := &renderapp.App{Assets: helper}
	if got := birthdayRelativePath(app, helper.Primary()+"/static_images/test.png"); got != "static_images/test.png" {
		t.Fatalf("relative asset path = %q", got)
	}
	if got := charaIconPath(helper, 999); !strings.Contains(got, "chr_icon_999.png") {
		t.Fatalf("fallback icon path = %q", got)
	}
	if got := birthdayCardImagePath(app, renderregion.JP, " "); got != "" {
		t.Fatalf("empty card image path = %q", got)
	}
}

func TestBirthdaySelectionValidationBranches(t *testing.T) {
	selection, err := normalizeBirthdaySelection(nil)
	if err != nil || selection.UpcomingIndex != 1 {
		t.Fatalf("default selection = %#v, %v", selection, err)
	}
	selection, err = normalizeBirthdaySelection(&CommandInput{Query: "2"})
	if err != nil || selection.UpcomingIndex != 2 {
		t.Fatalf("numeric selection = %#v, %v", selection, err)
	}
	if _, err := normalizeBirthdaySelection(&CommandInput{Query: "0"}); err == nil {
		t.Fatal("zero birthday index accepted")
	}
	selection, err = normalizeBirthdaySelection(&CommandInput{Query: " miku "})
	if err != nil || selection.Query != "miku" {
		t.Fatalf("query selection = %#v, %v", selection, err)
	}
	params, err := json.Marshal(miscBirthdaySelection{Cid: 21, UpcomingIndex: 3, Query: " ignored "})
	if err != nil {
		t.Fatalf("marshal selection: %v", err)
	}
	selection, err = normalizeBirthdaySelection(&CommandInput{Params: params})
	if err != nil || selection.Cid != 21 || selection.Query != "ignored" {
		t.Fatalf("parameter selection = %#v, %v", selection, err)
	}

	infos := []birthdayCharacterInfo{{Cid: 1}, {Cid: 2}}
	if got, err := selectBirthdayInfo(infos, miscBirthdaySelection{Cid: 2}); err != nil || got.Cid != 2 {
		t.Fatalf("CID selection = %#v, %v", got, err)
	}
	if _, err := selectBirthdayInfo(infos, miscBirthdaySelection{Cid: 99}); err == nil {
		t.Fatal("missing CID selection succeeded")
	}
	if _, err := selectBirthdayInfo(infos, miscBirthdaySelection{UpcomingIndex: 3}); err == nil {
		t.Fatal("out-of-range upcoming selection succeeded")
	}
	if _, err := resolveBirthdayCharacterID(context.Background(), nil, renderregion.JP, " "); err == nil {
		t.Fatal("empty character query resolved")
	}
	if _, err := resolveBirthdayCharacterID(context.Background(), nil, renderregion.JP, "unknown-character"); err == nil || !strings.Contains(err.Error(), "service unavailable") {
		t.Fatalf("unconfigured character lookup error = %v", err)
	}
	if _, err := lookupBirthdayCharacterIDs(context.Background(), nil, renderregion.JP, "miku"); err == nil {
		t.Fatal("unconfigured birthday lookup succeeded")
	}
}
