package handler

import (
	"context"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/internal/testutil"
)

func TestCustomProfileCollectorKeysCustomizedHonors(t *testing.T) {
	card := sekaiapi.UserCustomProfileCard{CustomProfileCard: sekaiapi.ProfileCardData{
		Honors: []sekaiapi.HonorData{
			{ID: 4, FullSize: true, HonorBackgroundID: new(10102), HonorWordID: new(10101)},
			{ID: 4, FullSize: true},
			{ID: 4, FullSize: false, HonorWordID: new(10102)},
		},
	}}
	resp := &sekaiapi.GetAnotherProfileResponse{
		UserHonors:        []sekaiapi.UserHonor{{HonorID: 4, Level: 12}},
		UserProfileHonors: []sekaiapi.UserProfileHonor{{Seq: 1, HonorID: 4, HonorLevel: 12, HonorBackgroundID: new(10101)}},
	}
	c := newCustomProfileResourceCollector(card, resp)

	custom, ok := c.honorQueries["4:12:main:10102:10101"]
	testutil.Require(t, ok && *custom.HonorBackgroundID == 10102 && *custom.HonorWordID == 10101, "customized key = %+v ok=%v", custom, ok)
	plain, ok := c.honorQueries["4:12:main"]
	testutil.Require(t, ok && plain.HonorBackgroundID == nil && plain.HonorWordID == nil, "plain key must hold the uncustomized slot: %+v", plain)
	sub, ok := c.honorQueries["4:12:sub:0:10102"]
	testutil.Require(t, ok, "sub customized key missing: %+v", c.honorQueries)
	_, ok = c.honorQueries["4:12:sub"]
	testutil.Require(t, ok && *sub.HonorWordID == 10102, "a customized-only slot also fills the plain key")

	profile, ok := c.profileHonors["profile:1"]
	testutil.Require(t, ok && *profile.HonorBackgroundID == 10101, "profile honor = %+v", profile)
	_, ok = c.profileHonors["4:12:main:10101:0"]
	testutil.Require(t, ok, "profile customized key missing: %+v", c.profileHonors)

	for _, id := range []int{10101, 10102} {
		_, bg := c.honorBgIDs[id]
		_, word := c.honorWordIDs[id]
		testutil.Require(t, bg && word, "layer id %d not collected: bg=%v word=%v", id, bg, word)
	}

	// Without a provider (or a region without the tables) nothing is sent
	// and nothing fails.
	resources := drawing.CustomProfileResources{}
	collectCustomProfileHonorLayerResources(context.Background(), nil, renderregion.JP, c, resources)
	_, hasBg := resources["honorBackgrounds"]
	testutil.Require(t, !hasBg, "no rows must be sent without a row source: %+v", resources)
}

func TestCustomProfileCollectorUncustomizedHonorsKeepPlainKeys(t *testing.T) {
	card := sekaiapi.UserCustomProfileCard{CustomProfileCard: sekaiapi.ProfileCardData{
		Honors: []sekaiapi.HonorData{{ID: 8, FullSize: true}},
	}}
	resp := &sekaiapi.GetAnotherProfileResponse{UserHonors: []sekaiapi.UserHonor{{HonorID: 8, Level: 3}}}
	c := newCustomProfileResourceCollector(card, resp)
	testutil.Require(t, len(c.honorQueries) == 1, "old-client card must produce only the plain key: %+v", c.honorQueries)
	_, ok := c.honorQueries["8:3:main"]
	testutil.Require(t, ok && len(c.honorBgIDs) == 0 && len(c.honorWordIDs) == 0, "old-client card keys = %+v", c.honorQueries)
}
