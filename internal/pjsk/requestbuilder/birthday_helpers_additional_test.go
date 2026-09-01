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
	"haruki-cloud/internal/testutil"
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
		{
			testutil.Require(t, !(len(ids) != 1), "match %q = %v", query, ids)
			testutil.Require(t, !(ids[0] != 2), "match %q = %v", query, ids)
		}

	}
	{
		got := matchBirthdayCharacterIDs(rows, "missing")
		testutil.Require(t, !(got != nil), "missing match = %v", got)
	}
	{

		got := matchBirthdayCharacterIDs(rows, " ")
		testutil.Require(t, !(got != nil), "empty match = %v", got)
	}
	testutil.RequireArgs(t, !(birthdayCharacterMatches(rows[2], "not-a-name")), "unrelated character name matched")

	names := birthdayCharacterNames(&sekaidb.Gamecharacter{FirstName: "Miku", GivenName: "Miku", FirstNameEnglish: "miku"})
	testutil.Require(t, !(len(names) != 3), "deduplicated character names = %#v", names)

	values := []string{"Miku"}
	appendBirthdayCharacterName(&values, " miku ")
	appendBirthdayCharacterName(&values, "")
	{
		testutil.Require(t, !(len(values) != 1), "normalized names = %#v", values)
		testutil.Require(t, !(normalizeBirthdayCharacterText("  Hatsune   Miku ") != "hatsunemiku"), "normalized names = %#v", values)
	}

}

func TestBirthdayDateRegionAndPathHelpers(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	next := nextBirthdayTime(renderregion.JP, 8, 11, now)
	{
		testutil.Require(t, !(next.Year() != 2027), "next birthday = %v", next)
		testutil.Require(t, !(next.Location() != birthdayDisplayLocation), "next birthday = %v", next)
	}
	testutil.Require(t, !(birthdayRegionLocation(renderregion.Unknown).String() != "UTC+9"), "unknown region location = %v", birthdayRegionLocation(renderregion.Unknown))
	{
		testutil.RequireArgs(t, !(birthdayRegionName(renderregion.Unknown) != "日服"), "birthday region names are incorrect")
		testutil.RequireArgs(t, !(birthdayRegionName(renderregion.Value("xx")) != "XX"), "birthday region names are incorrect")
		testutil.RequireArgs(t, !(birthdayRegionName(renderregion.CN) != "国服"), "birthday region names are incorrect")
	}
	{
		testutil.RequireArgs(t, isBirthdayFifthAnniv(renderregion.JP), "fifth anniversary region classification failed")
		testutil.RequireArgs(t, !(isBirthdayFifthAnniv(renderregion.EN)), "fifth anniversary region classification failed")
	}
	{
		testutil.RequireArgs(t, !(birthdayDaysUntil(now, now.Add(-time.Hour)) != 0), "birthday day countdown failed")
		testutil.RequireArgs(t, !(birthdayDaysUntil(now, now.Add(49*time.Hour)) != 2), "birthday day countdown failed")
	}

	event := buildBirthdayEventTime(now, now.Add(30*time.Second))
	testutil.Require(t, !(event.EndAt != now.Add(30*time.Second).UnixMilli()), "short event end = %d", event.EndAt)

	event = buildBirthdayEventTime(now, now.Add(2*time.Hour))
	testutil.Require(t, !(event.EndAt != now.Add(119*time.Minute).UnixMilli()), "normal event end = %d", event.EndAt)
	{
		testutil.RequireArgs(t, !(birthdayRelativePath(nil, "/absolute/path") != "/absolute/path"), "nil birthday relative path changed")
		testutil.RequireArgs(t, !(birthdayRelativePath(&renderapp.App{}, "") != ""), "nil birthday relative path changed")
	}

	helper := assets.NewAssetHelper(t.TempDir(), nil)
	app := &renderapp.App{Assets: helper}
	{
		got := birthdayRelativePath(app, helper.Primary()+"/static_images/test.png")
		testutil.Require(t, !(got != "static_images/test.png"), "relative asset path = %q", got)
	}
	{

		got := charaIconPath(helper, 999)
		testutil.Require(t, strings.Contains(got, "chr_icon_999.png"), "fallback icon path = %q", got)
	}
	{

		got := birthdayCardImagePath(app, renderregion.JP, " ")
		testutil.Require(t, !(got != ""), "empty card image path = %q", got)
	}

}

func TestBirthdaySelectionValidationBranches(t *testing.T) {
	selection, err := normalizeBirthdaySelection(nil)
	{
		testutil.Require(t, !(err != nil), "default selection = %#v, %v", selection, err)
		testutil.Require(t, !(selection.UpcomingIndex != 1), "default selection = %#v, %v", selection, err)
	}

	selection, err = normalizeBirthdaySelection(&CommandInput{Query: "2"})
	{
		testutil.Require(t, !(err != nil), "numeric selection = %#v, %v", selection, err)
		testutil.Require(t, !(selection.UpcomingIndex != 2), "numeric selection = %#v, %v", selection, err)
	}
	{

		_, err := normalizeBirthdaySelection(&CommandInput{Query: "0"})
		testutil.RequireArgs(t, !(err == nil), "zero birthday index accepted")
	}

	selection, err = normalizeBirthdaySelection(&CommandInput{Query: " miku "})
	{
		testutil.Require(t, !(err != nil), "query selection = %#v, %v", selection, err)
		testutil.Require(t, !(selection.Query != "miku"), "query selection = %#v, %v", selection, err)
	}

	params, err := json.Marshal(miscBirthdaySelection{Cid: 21, UpcomingIndex: 3, Query: " ignored "})
	testutil.Require(t, !(err != nil), "marshal selection: %v", err)

	selection, err = normalizeBirthdaySelection(&CommandInput{Params: params})
	{
		testutil.Require(t, !(err != nil), "parameter selection = %#v, %v", selection, err)
		testutil.Require(t, !(selection.Cid != 21), "parameter selection = %#v, %v", selection, err)
		testutil.Require(t, !(selection.Query != "ignored"), "parameter selection = %#v, %v", selection, err)
	}

	infos := []birthdayCharacterInfo{{Cid: 1}, {Cid: 2}}
	{
		got, err := selectBirthdayInfo(infos, miscBirthdaySelection{Cid: 2})
		{
			testutil.Require(t, !(err != nil), "CID selection = %#v, %v", got, err)
			testutil.Require(t, !(got.Cid != 2), "CID selection = %#v, %v", got, err)
		}
	}
	{

		_, err := selectBirthdayInfo(infos, miscBirthdaySelection{Cid: 99})
		testutil.RequireArgs(t, !(err == nil), "missing CID selection succeeded")
	}
	{

		_, err := selectBirthdayInfo(infos, miscBirthdaySelection{UpcomingIndex: 3})
		testutil.RequireArgs(t, !(err == nil), "out-of-range upcoming selection succeeded")
	}
	{

		_, err := resolveBirthdayCharacterID(context.Background(), nil, renderregion.JP, " ")
		testutil.RequireArgs(t, !(err == nil), "empty character query resolved")
	}
	{

		_, err := resolveBirthdayCharacterID(context.Background(), nil, renderregion.JP, "unknown-character")
		{
			testutil.Require(t, !(err == nil), "unconfigured character lookup error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "service unavailable"), "unconfigured character lookup error = %v", err)
		}
	}
	{

		_, err := lookupBirthdayCharacterIDs(context.Background(), nil, renderregion.JP, "miku")
		testutil.RequireArgs(t, !(err == nil), "unconfigured birthday lookup succeeded")
	}

}
