package drawing

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func cacheKeyBenchmarkRequests() []struct {
	name, endpoint string
	request        any
} {
	thumb := func(id int) CardFullThumbnailRequest {
		return CardFullThumbnailRequest{CardID: id, CardThumbnailPath: fmt.Sprintf("asset/jp/thumbnail/%d.png", id), Rare: "rarity_4", FrameImgPath: "frame.png", AttrImgPath: "cool.png", RareImgPath: "star.png"}
	}
	profile := &ProfileRequest{Profile: BasicProfile{ID: "7354311836516539153", Region: "jp", Nickname: "测试用户", LeaderImagePath: "leader.png"}, Rank: 300, Word: "profile"}
	for i := 1; i <= 5; i++ {
		profile.Pcards = append(profile.Pcards, thumb(i))
	}
	for i := 1; i <= 26; i++ {
		profile.CharacterRank = append(profile.CharacterRank, CharacterRank{CharacterID: i, Rank: 50})
	}
	for _, diff := range []string{"easy", "normal", "hard", "expert", "master", "append"} {
		profile.MusicDifficultyCount = append(profile.MusicDifficultyCount, MusicClearCount{Difficulty: diff, Clear: 500, Fc: 400, Ap: 100})
	}
	cases := []struct {
		name, endpoint string
		request        any
	}{{"Profile", "/api/pjsk/profile/profile", profile}}
	for _, count := range []int{100, 1000} {
		box := &CardBoxRequest{Region: "jp", ShowID: true, ShowBox: true, UserInfo: &DetailedProfileCardRequest{ID: profile.Profile.ID, Region: "jp", Nickname: "测试用户", Source: "suite", UpdateTime: 1710000000}}
		for i := 1; i <= count; i++ {
			box.Cards = append(box.Cards, UserCard{Card: CardBasic{CardID: i, ThumbnailInfo: []CardFullThumbnailRequest{thumb(i)}}, HasCard: i%3 != 0})
		}
		cases = append(cases, struct {
			name, endpoint string
			request        any
		}{fmt.Sprintf("CardBox%d", count), "/api/pjsk/card/box", box})
	}
	return cases
}

func BenchmarkRenderCacheKeyPipeline(b *testing.B) {
	now := time.Unix(1710000000, 0)
	for _, tc := range cacheKeyBenchmarkRequests() {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				prepared := prepareDrawingRequestBody(tc.endpoint, tc.request, now, context.Background())
				policy, err := buildRenderCachePolicy(tc.endpoint, preparedRenderCachePayload{payload: prepared})
				if err != nil {
					b.Fatal(err)
				}
				if _, err := buildRenderCacheKey(policy); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
