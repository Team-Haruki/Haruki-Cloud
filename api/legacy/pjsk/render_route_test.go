package pjsk

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/config"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	rendercard "haruki-cloud/internal/pjsk/render/card"
	renderdeck "haruki-cloud/internal/pjsk/render/deck"
	rendereducation "haruki-cloud/internal/pjsk/render/education"
	renderevent "haruki-cloud/internal/pjsk/render/event"
	rendergacha "haruki-cloud/internal/pjsk/render/gacha"
	renderhonor "haruki-cloud/internal/pjsk/render/honor"
	"haruki-cloud/internal/pjsk/render/masterdata"
	rendermisc "haruki-cloud/internal/pjsk/render/misc"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	renderprofile "haruki-cloud/internal/pjsk/render/profile"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	renderscore "haruki-cloud/internal/pjsk/render/score"
	rendersk "haruki-cloud/internal/pjsk/render/sk"
	renderstamp "haruki-cloud/internal/pjsk/render/stamp"
	renderuserdata "haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"
	"haruki-cloud/utils/imagecache"
	sekaiapi "haruki-cloud/utils/sekai"

	"github.com/gofiber/fiber/v3"
)

type renderEnvelope struct {
	Status  int             `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type routeEventSource struct {
	region renderregion.Value
	events []*masterdata.Event
}

func (s *routeEventSource) DefaultRegion() renderregion.Value { return s.region }

func (s *routeEventSource) GetEventByID(id int) (*masterdata.Event, error) {
	for _, eventInfo := range s.events {
		if eventInfo.ID == id {
			copy := *eventInfo
			return &copy, nil
		}
	}
	return nil, fiber.ErrNotFound
}

func (s *routeEventSource) GetEventByCardID(cardID int) (*masterdata.Event, error) {
	return nil, fiber.ErrNotFound
}

func (s *routeEventSource) GetEvents() []*masterdata.Event {
	out := make([]*masterdata.Event, 0, len(s.events))
	for _, eventInfo := range s.events {
		copy := *eventInfo
		out = append(out, &copy)
	}
	return out
}

func (s *routeEventSource) GetEventCards(eventID int) ([]*masterdata.Card, error) {
	return nil, nil
}

func (s *routeEventSource) GetEventBannerCharacterID(eventID int) (int, error) {
	return 0, fiber.ErrNotFound
}

func (s *routeEventSource) GetEventDeckBonuses(eventID int) ([]*masterdata.EventDeckBonus, error) {
	return nil, nil
}

func (s *routeEventSource) GetGameCharacterUnit(id int) (*masterdata.GameCharacterUnit, error) {
	return nil, fiber.ErrNotFound
}

func (s *routeEventSource) GetBanEvents(charID int) []*masterdata.Event { return nil }

func (s *routeEventSource) GetWorldBloomChapters(eventID int) []*masterdata.WorldBloom {
	return nil
}

func (s *routeEventSource) GetCharacterByID(id int) (*masterdata.Character, error) {
	return nil, fiber.ErrNotFound
}

type routeTrackerSource struct{}

func (routeTrackerSource) GetLatestRankingByRank(server string, eventID, rank int) (*sekaiapi.LatestRankingResponse, error) {
	score := 1000000 + rank
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    strconv.Itoa(10000 + rank),
			Score:     score,
			Rank:      rank,
			Timestamp: 1704067200,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.Itoa(10000 + rank),
			Name:   "TrackerUser",
		},
	}, nil
}

func (routeTrackerSource) GetLatestRankingByUser(server string, eventID int, userID int64) (*sekaiapi.LatestRankingResponse, error) {
	score := 1500000 + int(userID%1000)
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    strconv.FormatInt(userID, 10),
			Score:     score,
			Rank:      321,
			Timestamp: 1704067200,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(userID, 10),
			Name:   "TrackerUIDUser",
		},
	}, nil
}

func (routeTrackerSource) GetLatestWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*sekaiapi.WorldBloomLatestRankingResponse, error) {
	score := 2000000 + rank
	return &sekaiapi.WorldBloomLatestRankingResponse{
		RankData: sekaiapi.WorldBloomRankDataPoint{
			RankDataPoint: sekaiapi.RankDataPoint{
				UserID:    strconv.Itoa(20000 + rank),
				Score:     score,
				Rank:      rank,
				Timestamp: 1704067200,
			},
			CharacterID: &characterID,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.Itoa(20000 + rank),
			Name:   "WLTrackerUser",
		},
	}, nil
}

func (routeTrackerSource) GetLatestWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomLatestRankingResponse, error) {
	score := 2500000 + int(userID%1000)
	return &sekaiapi.WorldBloomLatestRankingResponse{
		RankData: sekaiapi.WorldBloomRankDataPoint{
			RankDataPoint: sekaiapi.RankDataPoint{
				UserID:    strconv.FormatInt(userID, 10),
				Score:     score,
				Rank:      432,
				Timestamp: 1704067200,
			},
			CharacterID: &characterID,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(userID, 10),
			Name:   "WLTrackerUIDUser",
		},
	}, nil
}

func (routeTrackerSource) GetRankingScoreGrowth(server string, eventID, interval int) ([]sekaiapi.ScoreGrowthPoint, error) {
	earlier := int64(1704067200)
	diff := int64(interval)
	growthRank1 := 900
	growthRank100 := 3000
	earlierRank1 := 1001000
	earlierRank100 := 1100000
	return []sekaiapi.ScoreGrowthPoint{
		{
			Rank:             1,
			ScoreLatest:      earlierRank1 + growthRank1,
			ScoreEarlier:     &earlierRank1,
			TimestampLatest:  earlier + diff,
			TimestampEarlier: &earlier,
			TimeDiff:         &diff,
			Growth:           &growthRank1,
		},
		{
			Rank:             100,
			ScoreLatest:      earlierRank100 + growthRank100,
			ScoreEarlier:     &earlierRank100,
			TimestampLatest:  earlier + diff,
			TimestampEarlier: &earlier,
			TimeDiff:         &diff,
			Growth:           &growthRank100,
		},
	}, nil
}

func (routeTrackerSource) GetWorldBloomRankingScoreGrowth(server string, eventID, characterID, interval int) ([]sekaiapi.ScoreGrowthPoint, error) {
	return routeTrackerSource{}.GetRankingScoreGrowth(server, eventID, interval)
}

func (routeTrackerSource) TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error) {
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{
				UserID:    strconv.Itoa(10000 + rank),
				Score:     1000000 + rank,
				Rank:      rank,
				Timestamp: 1704067200,
			},
			{
				UserID:    strconv.Itoa(10000 + rank),
				Score:     1005000 + rank,
				Rank:      rank,
				Timestamp: 1704070800,
			},
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.Itoa(10000 + rank),
			Name:   "TrackerUser",
		},
	}, nil
}

func (routeTrackerSource) TraceWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*sekaiapi.WorldBloomTraceRankingResponse, error) {
	return &sekaiapi.WorldBloomTraceRankingResponse{
		RankData: []sekaiapi.WorldBloomRankDataPoint{
			{
				RankDataPoint: sekaiapi.RankDataPoint{
					UserID:    strconv.Itoa(20000 + rank),
					Score:     2000000 + rank,
					Rank:      rank,
					Timestamp: 1704067200,
				},
				CharacterID: &characterID,
			},
			{
				RankDataPoint: sekaiapi.RankDataPoint{
					UserID:    strconv.Itoa(20000 + rank),
					Score:     2005000 + rank,
					Rank:      rank,
					Timestamp: 1704070800,
				},
				CharacterID: &characterID,
			},
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.Itoa(20000 + rank),
			Name:   "WLTrackerUser",
		},
	}, nil
}

func (routeTrackerSource) TraceRankingByUser(server string, eventID int, userID int64) (*sekaiapi.TraceRankingResponse, error) {
	return nil, nil
}

func (routeTrackerSource) TraceWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomTraceRankingResponse, error) {
	return nil, nil
}

type routeGachaSource struct {
	region   renderregion.Value
	gachas   []*masterdata.Gacha
	gachaMap map[int]*masterdata.Gacha
	cardMap  map[int]*masterdata.Card
}

func (s *routeGachaSource) DefaultRegion() renderregion.Value { return s.region }

func (s *routeGachaSource) GetGachaByID(id int) (*masterdata.Gacha, error) {
	if item, ok := s.gachaMap[id]; ok {
		copy := *item
		return &copy, nil
	}
	return nil, fiber.ErrNotFound
}

func (s *routeGachaSource) GetGachas() []*masterdata.Gacha {
	out := make([]*masterdata.Gacha, 0, len(s.gachas))
	for _, item := range s.gachas {
		copy := *item
		out = append(out, &copy)
	}
	return out
}

func (s *routeGachaSource) GetCardByID(id int) (*masterdata.Card, error) {
	if item, ok := s.cardMap[id]; ok {
		copy := *item
		return &copy, nil
	}
	return nil, fiber.ErrNotFound
}

type routeCardSource struct {
	region       renderregion.Value
	cards        map[int]*masterdata.Card
	characters   map[int]*masterdata.Character
	skills       map[int]*masterdata.Skill
	gachasByCard map[int]*masterdata.Gacha
	costumes     map[int][]*masterdata.Costume3d
}

type routeDeckCardSource struct {
	region renderregion.Value
	cards  map[int]*masterdata.Card
}

func (s *routeDeckCardSource) DefaultRegion() renderregion.Value { return s.region }

func (s *routeDeckCardSource) GetCardByID(id int) (*masterdata.Card, error) {
	if item, ok := s.cards[id]; ok {
		copy := *item
		copy.CardParameters = append([]masterdata.CardParameter(nil), item.CardParameters...)
		return &copy, nil
	}
	return nil, fiber.ErrNotFound
}

func (s *routeCardSource) DefaultRegion() renderregion.Value { return s.region }

func (s *routeCardSource) GetCardByID(id int) (*masterdata.Card, error) {
	if item, ok := s.cards[id]; ok {
		copy := *item
		if item.CardParameters != nil {
			copy.CardParameters = append([]masterdata.CardParameter(nil), item.CardParameters...)
		}
		return &copy, nil
	}
	return nil, fiber.ErrNotFound
}

func (s *routeCardSource) GetCardByCharacterAndSeq(characterID, seq int) (*masterdata.Card, error) {
	for _, item := range s.cards {
		if item.CharacterID == characterID {
			copy := *item
			if item.CardParameters != nil {
				copy.CardParameters = append([]masterdata.CardParameter(nil), item.CardParameters...)
			}
			return &copy, nil
		}
	}
	return nil, fiber.ErrNotFound
}

func (s *routeCardSource) FilterCards(info *rendercard.CardQueryInfo) ([]*masterdata.Card, error) {
	out := make([]*masterdata.Card, 0, len(s.cards))
	for _, item := range s.cards {
		if info != nil && info.CharacterID != 0 && item.CharacterID != info.CharacterID {
			continue
		}
		copy := *item
		if item.CardParameters != nil {
			copy.CardParameters = append([]masterdata.CardParameter(nil), item.CardParameters...)
		}
		out = append(out, &copy)
	}
	return out, nil
}

func (s *routeCardSource) GetCharacterByID(id int) (*masterdata.Character, error) {
	if item, ok := s.characters[id]; ok {
		copy := *item
		return &copy, nil
	}
	return nil, fiber.ErrNotFound
}

func (s *routeCardSource) GetUnitByCardID(cardID int) (string, error) {
	cardInfo, ok := s.cards[cardID]
	if !ok {
		return "", fiber.ErrNotFound
	}
	character, ok := s.characters[cardInfo.CharacterID]
	if !ok {
		return "", fiber.ErrNotFound
	}
	return character.Unit, nil
}

func (s *routeCardSource) GetCardSupplyType(cardInfo *masterdata.Card) string {
	if cardInfo == nil {
		return ""
	}
	return "常驻"
}

func (s *routeCardSource) GetSkillByID(id int) (*masterdata.Skill, error) {
	if item, ok := s.skills[id]; ok {
		copy := *item
		if item.SkillEffects != nil {
			copy.SkillEffects = append([]masterdata.SkillEffect(nil), item.SkillEffects...)
		}
		return &copy, nil
	}
	return nil, fiber.ErrNotFound
}

func (s *routeCardSource) FormatSkillDescription(skillInfo *masterdata.Skill, cardCharacterID int) string {
	return "score up"
}

func (s *routeCardSource) GetGachaByCardID(cardID int) (*masterdata.Gacha, error) {
	if item, ok := s.gachasByCard[cardID]; ok {
		copy := *item
		return &copy, nil
	}
	return nil, fiber.ErrNotFound
}

func (s *routeCardSource) GetCostume3dsByCardID(cardID int) ([]*masterdata.Costume3d, error) {
	items := s.costumes[cardID]
	out := make([]*masterdata.Costume3d, 0, len(items))
	for _, item := range items {
		copy := *item
		out = append(out, &copy)
	}
	return out, nil
}

type routeStampSource struct {
	region renderregion.Value
	stamps []masterdata.Stamp
}

func (s *routeStampSource) DefaultRegion() renderregion.Value { return s.region }

func (s *routeStampSource) GetStamps() ([]masterdata.Stamp, error) {
	return append([]masterdata.Stamp(nil), s.stamps...), nil
}

type routeHonorSource struct {
	region  renderregion.Value
	honors  map[int]*masterdata.Honor
	groups  map[int]*masterdata.HonorGroup
	bonds   map[int]*masterdata.BondsHonor
	gcuByID map[int]*masterdata.GameCharacterUnit
}

func (s *routeHonorSource) DefaultRegion() renderregion.Value { return s.region }

func (s *routeHonorSource) GetHonorByID(id int) (*masterdata.Honor, error) {
	if item, ok := s.honors[id]; ok {
		copy := *item
		if item.Levels != nil {
			copy.Levels = append([]masterdata.HonorLevel(nil), item.Levels...)
		}
		return &copy, nil
	}
	return nil, fiber.ErrNotFound
}

func (s *routeHonorSource) GetHonorGroupByID(id int) (*masterdata.HonorGroup, error) {
	if item, ok := s.groups[id]; ok {
		copy := *item
		return &copy, nil
	}
	return nil, fiber.ErrNotFound
}

func (s *routeHonorSource) GetBondsHonorByID(id int) (*masterdata.BondsHonor, error) {
	if item, ok := s.bonds[id]; ok {
		copy := *item
		return &copy, nil
	}
	return nil, fiber.ErrNotFound
}

func (s *routeHonorSource) GetGameCharacterUnitByID(id int) (*masterdata.GameCharacterUnit, bool) {
	if item, ok := s.gcuByID[id]; ok {
		copy := *item
		return &copy, true
	}
	return nil, false
}

type routeProfileSource struct {
	region        renderregion.Value
	cards         map[int]*masterdata.Card
	honors        map[int]*masterdata.Honor
	groups        map[int]*masterdata.HonorGroup
	bonds         map[int]*masterdata.BondsHonor
	gcuByID       map[int]*masterdata.GameCharacterUnit
	frames        map[int]*masterdata.PlayerFrame
	frameGroups   map[int]*masterdata.PlayerFrameGroup
	honorEventIDs map[int]int
}

func (s *routeProfileSource) DefaultRegion() renderregion.Value { return s.region }

func (s *routeProfileSource) GetCardByID(id int) (*masterdata.Card, error) {
	if item, ok := s.cards[id]; ok {
		copy := *item
		return &copy, nil
	}
	return nil, fiber.ErrNotFound
}

func (s *routeProfileSource) GetHonorByID(id int) (*masterdata.Honor, error) {
	if item, ok := s.honors[id]; ok {
		copy := *item
		if item.Levels != nil {
			copy.Levels = append([]masterdata.HonorLevel(nil), item.Levels...)
		}
		return &copy, nil
	}
	return nil, fiber.ErrNotFound
}

func (s *routeProfileSource) GetHonorGroupByID(id int) (*masterdata.HonorGroup, error) {
	if item, ok := s.groups[id]; ok {
		copy := *item
		return &copy, nil
	}
	return nil, fiber.ErrNotFound
}

func (s *routeProfileSource) GetBondsHonorByID(id int) (*masterdata.BondsHonor, error) {
	if item, ok := s.bonds[id]; ok {
		copy := *item
		return &copy, nil
	}
	return nil, fiber.ErrNotFound
}

func (s *routeProfileSource) GetGameCharacterUnitByID(id int) (*masterdata.GameCharacterUnit, bool) {
	if item, ok := s.gcuByID[id]; ok {
		copy := *item
		return &copy, true
	}
	return nil, false
}

func (s *routeProfileSource) GetPlayerFrameByID(id int) (*masterdata.PlayerFrame, error) {
	if item, ok := s.frames[id]; ok {
		copy := *item
		return &copy, nil
	}
	return nil, fiber.ErrNotFound
}

func (s *routeProfileSource) GetPlayerFrameGroupByID(id int) (*masterdata.PlayerFrameGroup, error) {
	if item, ok := s.frameGroups[id]; ok {
		copy := *item
		return &copy, nil
	}
	return nil, fiber.ErrNotFound
}

func (s *routeProfileSource) GetEventIDByHonorID(honorID int) int {
	return s.honorEventIDs[honorID]
}

type routeMusicSource struct {
	region              renderregion.Value
	musics              map[int]*masterdata.Music
	localizedTitles     map[int][]string
	difficulties        map[int][]*masterdata.MusicDifficulty
	vocals              map[int][]*masterdata.MusicVocal
	tags                map[int][]string
	characters          map[int]*masterdata.Character
	events              map[int]*masterdata.Event
	primaryEventByMusic map[int]int
	musicByEvent        map[int]int
	banEvents           map[int][]*masterdata.Event
	limited             map[int][]*masterdata.LimitedTimeMusic
}

func (s *routeMusicSource) DefaultRegion() renderregion.Value { return s.region }

type routeEducationSource struct {
	region         renderregion.Value
	rewardsByChar  map[int][]*rendereducation.ChallengeReward
	boxesByPurpose map[string]map[int]*rendereducation.ResourceBox
}

func (s *routeEducationSource) DefaultRegion() renderregion.Value { return s.region }

func (s *routeEducationSource) GetChallengeRewardsByCharacter(charID int) []*rendereducation.ChallengeReward {
	items := s.rewardsByChar[charID]
	out := make([]*rendereducation.ChallengeReward, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		copy := *item
		out = append(out, &copy)
	}
	return out
}

func (s *routeEducationSource) GetResourceBoxByPurpose(purpose string, id int) *rendereducation.ResourceBox {
	boxes := s.boxesByPurpose[purpose]
	item := boxes[id]
	if item == nil {
		return nil
	}
	copy := *item
	copy.Details = append([]rendereducation.ResourceBoxDetail(nil), item.Details...)
	return &copy
}

func (s *routeEducationSource) GetResourceBoxesByPurpose(purpose string) []*rendereducation.ResourceBox {
	return nil
}

func (s *routeEducationSource) GetAreaItem(id int) *rendereducation.AreaItem {
	return nil
}

func (s *routeEducationSource) GetAreaItemLevels(areaItemID int) []*rendereducation.AreaItemLevel {
	return nil
}

func (s *routeEducationSource) GetAreaItemLevel(areaItemID, level int) *rendereducation.AreaItemLevel {
	return nil
}

func (s *routeEducationSource) GetCharacterRank(characterID, rank int) *rendereducation.CharacterRank {
	return nil
}

func (s *routeEducationSource) GetMysekaiGateLevel(gateID, level int) *rendereducation.MysekaiGateLevel {
	return nil
}

func (s *routeEducationSource) GetShopItemByResourceBoxID(resourceBoxID int) *rendereducation.ShopItem {
	return nil
}

func (s *routeMusicSource) SearchMusic(query string) (*masterdata.Music, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fiber.ErrNotFound
	}
	if id, err := strconv.Atoi(query); err == nil {
		return s.GetMusicByID(id)
	}
	lower := strings.ToLower(query)
	for _, item := range s.musics {
		if strings.EqualFold(item.Title, query) || strings.Contains(strings.ToLower(item.Title), lower) {
			copy := *item
			if item.Categories != nil {
				copy.Categories = append([]string(nil), item.Categories...)
			}
			return &copy, nil
		}
		for _, title := range s.localizedTitles[item.ID] {
			if strings.EqualFold(title, query) || strings.Contains(strings.ToLower(title), lower) {
				copy := *item
				if item.Categories != nil {
					copy.Categories = append([]string(nil), item.Categories...)
				}
				return &copy, nil
			}
		}
	}
	return nil, fiber.ErrNotFound
}

func (s *routeMusicSource) GetMusicByID(id int) (*masterdata.Music, error) {
	if item, ok := s.musics[id]; ok {
		copy := *item
		if item.Categories != nil {
			copy.Categories = append([]string(nil), item.Categories...)
		}
		return &copy, nil
	}
	return nil, fiber.ErrNotFound
}

func (s *routeMusicSource) GetMusicByEventID(eventID int) (*masterdata.Music, error) {
	musicID, ok := s.musicByEvent[eventID]
	if !ok {
		return nil, fiber.ErrNotFound
	}
	return s.GetMusicByID(musicID)
}

func (s *routeMusicSource) GetMusics() []*masterdata.Music {
	out := make([]*masterdata.Music, 0, len(s.musics))
	for _, item := range s.musics {
		copy := *item
		if item.Categories != nil {
			copy.Categories = append([]string(nil), item.Categories...)
		}
		out = append(out, &copy)
	}
	return out
}

func (s *routeMusicSource) GetBanEvents(charID int) []*masterdata.Event {
	items := s.banEvents[charID]
	out := make([]*masterdata.Event, 0, len(items))
	for _, item := range items {
		copy := *item
		out = append(out, &copy)
	}
	return out
}

func (s *routeMusicSource) GetMusicLocalizedTitles(musicID int) ([]string, error) {
	return append([]string(nil), s.localizedTitles[musicID]...), nil
}

func (s *routeMusicSource) GetMusicDifficulties(musicID int) ([]*masterdata.MusicDifficulty, error) {
	items := s.difficulties[musicID]
	out := make([]*masterdata.MusicDifficulty, 0, len(items))
	for _, item := range items {
		copy := *item
		out = append(out, &copy)
	}
	return out, nil
}

func (s *routeMusicSource) GetMusicVocals(musicID int) ([]*masterdata.MusicVocal, error) {
	items := s.vocals[musicID]
	out := make([]*masterdata.MusicVocal, 0, len(items))
	for _, item := range items {
		copy := *item
		if item.Characters != nil {
			copy.Characters = append([]masterdata.MusicVocalCharacter(nil), item.Characters...)
		}
		out = append(out, &copy)
	}
	return out, nil
}

func (s *routeMusicSource) GetMusicTags(musicID int) ([]string, error) {
	return append([]string(nil), s.tags[musicID]...), nil
}

func (s *routeMusicSource) GetCharacterByID(id int) (*masterdata.Character, error) {
	if item, ok := s.characters[id]; ok {
		copy := *item
		return &copy, nil
	}
	return nil, fiber.ErrNotFound
}

func (s *routeMusicSource) GetPrimaryEventByMusicID(musicID int) (*masterdata.Event, error) {
	eventID, ok := s.primaryEventByMusic[musicID]
	if !ok {
		return nil, fiber.ErrNotFound
	}
	if item, ok := s.events[eventID]; ok {
		copy := *item
		return &copy, nil
	}
	return nil, fiber.ErrNotFound
}

func (s *routeMusicSource) GetLimitedTimeMusics(musicID int) []*masterdata.LimitedTimeMusic {
	items := s.limited[musicID]
	out := make([]*masterdata.LimitedTimeMusic, 0, len(items))
	for _, item := range items {
		copy := *item
		out = append(out, &copy)
	}
	return out
}

func TestPJSKEventBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/event/list/build", `{"region":"jp","include_past":true,"include_future":true}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			EventInfo []struct {
				EventName string `json:"event_name"`
			} `json:"event_info"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != eventListDrawingEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if len(data.Payload.EventInfo) != 1 || data.Payload.EventInfo[0].EventName != "JP Event" {
		t.Fatalf("unexpected payload: %+v", data.Payload.EventInfo)
	}
}

func TestPJSKEventRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != eventListDrawingEndpoint {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PNGDATA"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/event/list/render", strings.NewReader(`{"region":"jp","include_past":true,"include_future":true}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "PNGDATA" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKEventRecordBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/event/record/build", `{"event_info":[{"id":1,"event_name":"Event"}],"user_info":{"id":"1","region":"JP","nickname":"Test","source":"suite","update_time":1,"is_hide_uid":true,"leader_image_path":"leader.png","has_frame":false}}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			EventInfo []struct {
				ID int `json:"id"`
			} `json:"event_info"`
			UserInfo struct {
				Nickname string `json:"nickname"`
			} `json:"user_info"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != eventRecordDrawingEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if len(data.Payload.EventInfo) != 1 || data.Payload.EventInfo[0].ID != 1 || data.Payload.UserInfo.Nickname != "Test" {
		t.Fatalf("unexpected payload: %+v", data.Payload)
	}
}

func TestPJSKEventRecordRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != eventRecordDrawingEndpoint {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("RECORDPNG"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/event/record/render", strings.NewReader(`{"event_info":[{"id":1,"event_name":"Event"}],"user_info":{"id":"1","region":"JP","nickname":"Test","source":"suite","update_time":1,"is_hide_uid":true,"leader_image_path":"leader.png","has_frame":false}}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "RECORDPNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKEventRecordRenderRouteUsesRenderCacheHit(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "cache-hit.png")
	if err := os.WriteFile(cacheFile, []byte("CACHEDPNG"), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}

	var drawingCalls int32
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&drawingCalls, 1)
		t.Fatalf("drawing api should not be called on cache hit")
	}))
	defer drawingServer.Close()

	cacheServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cache" {
			t.Fatalf("unexpected cache path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected cache method: %s", r.Method)
		}
		key := r.URL.Query().Get("key")
		if len(key) != 64 {
			t.Fatalf("unexpected cache key: %q", key)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"key":"` + key + `","file_path":"` + cacheFile + `","created_at":"2026-03-22T00:00:00Z","expires_at":"2026-03-22T01:00:00Z"}`))
	}))
	defer cacheServer.Close()

	drawingClient := drawing.NewHarukiDrawingClient(drawingServer.URL)
	drawingClient.SetRenderCache(drawing.NewRenderCacheClient(drawing.RenderCacheConfig{
		BaseURL:    cacheServer.URL,
		StorageDir: t.TempDir(),
		TTL:        5 * time.Minute,
	}))

	app := fiber.New()
	runtime := testRenderApp(t, drawingClient)
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/event/record/render", strings.NewReader(`{"event_info":[{"id":1,"event_name":"Event"}],"user_info":{"id":"1","region":"JP","nickname":"Test","source":"suite","update_time":1,"is_hide_uid":true,"leader_image_path":"leader.png","has_frame":false}}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "CACHEDPNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
	if atomic.LoadInt32(&drawingCalls) != 0 {
		t.Fatalf("drawing api should not have been called")
	}
}

func TestPJSKEventRecordRenderRouteStoresRenderCacheOnMiss(t *testing.T) {
	storageDir := t.TempDir()
	var drawingCalls int32
	var postCalls int32

	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&drawingCalls, 1)
		if r.URL.Path != eventRecordDrawingEndpoint {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("MISSPNG"))
	}))
	defer drawingServer.Close()

	cacheServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cache" {
			t.Fatalf("unexpected cache path: %s", r.URL.Path)
		}

		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"record not found"}`))
		case http.MethodPost:
			atomic.AddInt32(&postCalls, 1)
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			if got := r.Form.Get("api_path"); got != strings.Trim(eventRecordDrawingEndpoint, "/") {
				t.Fatalf("unexpected api_path: %s", got)
			}
			if got := r.Form.Get("user_id"); got != "1" {
				t.Fatalf("unexpected user_id: %s", got)
			}
			if got := r.Form.Get("ttl"); got != "300" {
				t.Fatalf("unexpected ttl: %s", got)
			}
			key := r.Form.Get("key")
			if len(key) != 64 {
				t.Fatalf("unexpected key: %q", key)
			}
			filePath := r.Form.Get("file_path")
			expectedBase := filepath.Join(storageDir, "api", "pjsk", "event", "record") + string(filepath.Separator)
			if !strings.HasPrefix(filePath, expectedBase) || !strings.Contains(filePath, string(filepath.Separator)+"1"+string(filepath.Separator)) {
				t.Fatalf("unexpected file_path: %s", filePath)
			}
			data, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatalf("read cached file: %v", err)
			}
			if string(data) != "MISSPNG" {
				t.Fatalf("unexpected cached file body: %s", string(data))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"message":"ok","key":"` + key + `","old_deleted_count":0,"api_path":"` + r.Form.Get("api_path") + `","user_id":"1","file_path":"` + filePath + `","expires_at":"2026-03-22T01:00:00Z"}`))
		default:
			t.Fatalf("unexpected cache method: %s", r.Method)
		}
	}))
	defer cacheServer.Close()

	drawingClient := drawing.NewHarukiDrawingClient(drawingServer.URL)
	drawingClient.SetRenderCache(drawing.NewRenderCacheClient(drawing.RenderCacheConfig{
		BaseURL:    cacheServer.URL,
		StorageDir: storageDir,
		TTL:        5 * time.Minute,
	}))

	app := fiber.New()
	runtime := testRenderApp(t, drawingClient)
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/event/record/render", strings.NewReader(`{"event_info":[{"id":1,"event_name":"Event"}],"user_info":{"id":"1","region":"JP","nickname":"Test","source":"suite","update_time":1,"is_hide_uid":true,"leader_image_path":"leader.png","has_frame":false}}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "MISSPNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
	if atomic.LoadInt32(&drawingCalls) != 1 {
		t.Fatalf("expected drawing api to be called once, got %d", atomic.LoadInt32(&drawingCalls))
	}
	if atomic.LoadInt32(&postCalls) != 1 {
		t.Fatalf("expected cache register to be called once, got %d", atomic.LoadInt32(&postCalls))
	}
}

func TestPJSKCardDetailBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/card/detail/build", `{"query":"1001","region":"jp"}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			CardInfo struct {
				CardID int `json:"card_id"`
			} `json:"card_info"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != cardDetailDrawingEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if data.Payload.CardInfo.CardID != 1001 {
		t.Fatalf("unexpected payload: %+v", data.Payload)
	}
}

func TestPJSKCardListRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != cardListDrawingEndpoint {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("CARDLISTPNG"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/card/list/render", strings.NewReader(`{"card_ids":[1001],"region":"jp"}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "CARDLISTPNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKCardBoxBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/card/box/build", `[{"query":"1001","region":"jp"}]`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			Cards []struct {
				Card struct {
					CardID int `json:"card_id"`
				} `json:"card"`
			} `json:"cards"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != cardBoxDrawingEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if len(data.Payload.Cards) != 1 || data.Payload.Cards[0].Card.CardID != 1001 {
		t.Fatalf("unexpected payload: %+v", data.Payload)
	}
}

func TestPJSKDeckRecommendBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := deckRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/deck/recommend/build", `{"region":"jp","profile":{"id":"1","region":"JP","nickname":"Deck User","source":"suite","update_time":1,"is_hide_uid":true,"leader_image_path":"leader.png","has_frame":false},"deck_data":[{"card_data":[{"card_thumbnail":{"card_id":1001,"card_thumbnail_path":"thumb.png","rare":"rarity_4","frame_img_path":"frame.png","attr_img_path":"attr.png","rare_img_path":"rare.png","train_rank":0},"chara_id":1,"skill_level":"4","is_after_training":true,"skill_rate":120,"event_bonus_rate":20,"is_before_story":true,"is_after_story":true,"has_canvas_bonus":false}]}],"recommend_type":"event"}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			RecommendType string `json:"recommend_type"`
			DeckData      []struct {
				CardData []struct {
					CharaID int `json:"chara_id"`
				} `json:"card_data"`
			} `json:"deck_data"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != deckRecommendEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if data.Payload.RecommendType != "event" || len(data.Payload.DeckData) != 1 || len(data.Payload.DeckData[0].CardData) != 1 || data.Payload.DeckData[0].CardData[0].CharaID != 1 {
		t.Fatalf("unexpected payload: %+v", data.Payload)
	}
}

func TestPJSKDeckRecommendAutoBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := deckRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/deck/recommend/auto/build", `{"region":"jp","recommend_type":"event","limit":2}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			RecommendType string  `json:"recommend_type"`
			EventID       *int    `json:"event_id"`
			EventName     *string `json:"event_name"`
			LiveType      *string `json:"live_type"`
			DeckData      []struct {
				TotalPower *int `json:"total_power"`
				CardData   []struct {
					CardThumbnail struct {
						CardID int `json:"card_id"`
					} `json:"card_thumbnail"`
				} `json:"card_data"`
			} `json:"deck_data"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != deckRecommendEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Payload.RecommendType != "event" {
		t.Fatalf("unexpected recommend type: %s", data.Payload.RecommendType)
	}
	if data.Payload.EventID == nil || *data.Payload.EventID != 1 || data.Payload.EventName == nil || *data.Payload.EventName != "Deck Event" {
		t.Fatalf("unexpected event payload: %+v", data.Payload)
	}
	if data.Payload.LiveType == nil || *data.Payload.LiveType != "multi" {
		t.Fatalf("unexpected live type: %+v", data.Payload.LiveType)
	}
	if len(data.Payload.DeckData) != 1 || data.Payload.DeckData[0].TotalPower == nil || *data.Payload.DeckData[0].TotalPower <= 0 {
		t.Fatalf("unexpected deck data: %+v", data.Payload.DeckData)
	}
	if len(data.Payload.DeckData[0].CardData) != 2 || data.Payload.DeckData[0].CardData[0].CardThumbnail.CardID != 1002 {
		t.Fatalf("unexpected card order: %+v", data.Payload.DeckData[0].CardData)
	}
}

func TestPJSKDeckRecommendAutoRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != deckRecommendEndpoint {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("DECKPNG"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := deckRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/deck/recommend/auto/render", strings.NewReader(`{"region":"jp","recommend_type":"event","limit":2}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "DECKPNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKGachaBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/gacha/list/build", `{"region":"jp","include_past":true}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			Gachas []struct {
				Name string `json:"name"`
			} `json:"gachas"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != gachaListDrawingEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if len(data.Payload.Gachas) != 1 || data.Payload.Gachas[0].Name != "JP Gacha" {
		t.Fatalf("unexpected payload: %+v", data.Payload.Gachas)
	}
}

func TestPJSKGachaRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != gachaListDrawingEndpoint {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("GACHAPNG"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/gacha/list/render", strings.NewReader(`{"region":"jp","include_past":true}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "GACHAPNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKStampBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/stamp/list/build", `{"region":"jp"}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			Stamps []struct {
				ID int `json:"id"`
			} `json:"stamps"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != stampListDrawingEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if len(data.Payload.Stamps) != 1 || data.Payload.Stamps[0].ID != 5001 {
		t.Fatalf("unexpected payload: %+v", data.Payload.Stamps)
	}
}

func TestPJSKStampRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != stampListDrawingEndpoint {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("STAMPPNG"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/stamp/list/render", strings.NewReader(`{"region":"jp"}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "STAMPPNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKHonorBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/honor/build", `{"region":"jp","honor_id":7001,"honor_level":3}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			HonorType *string `json:"honor_type"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != honorDrawingEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if data.Payload.HonorType == nil || *data.Payload.HonorType != "normal" {
		t.Fatalf("unexpected payload: %+v", data.Payload)
	}
}

func TestPJSKHonorRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != honorDrawingEndpoint {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("HONORPNG"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/honor/render", strings.NewReader(`{"region":"jp","honor_id":7001,"honor_level":3}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "HONORPNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKProfileBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := profileRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/profile/build", `{"region":"jp"}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			Profile struct {
				Nickname string `json:"nickname"`
				HasFrame bool   `json:"has_frame"`
			} `json:"profile"`
			Rank   int    `json:"rank"`
			Word   string `json:"word"`
			Pcards []struct {
				CardID int `json:"card_id"`
			} `json:"pcards"`
			FramePaths struct {
				Base string `json:"base"`
			} `json:"frame_paths"`
			MusicDifficultyCount []struct {
				Difficulty string `json:"difficulty"`
				Clear      int    `json:"clear"`
				Fc         int    `json:"fc"`
				Ap         int    `json:"ap"`
			} `json:"music_difficulty_count"`
			CharacterRank []struct {
				CharacterID int `json:"character_id"`
				Rank        int `json:"rank"`
			} `json:"character_rank"`
			SoloLive struct {
				CharacterID int `json:"character_id"`
				Score       int `json:"score"`
				Rank        int `json:"rank"`
			} `json:"solo_live"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != profileDrawingEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if data.Payload.Profile.Nickname != "Profile User" || !data.Payload.Profile.HasFrame {
		t.Fatalf("unexpected profile payload: %+v", data.Payload.Profile)
	}
	if data.Payload.Rank != 123 {
		t.Fatalf("unexpected rank: %d", data.Payload.Rank)
	}
	if data.Payload.Word != "hello profile" {
		t.Fatalf("unexpected word: %s", data.Payload.Word)
	}
	if len(data.Payload.Pcards) != 2 || data.Payload.Pcards[0].CardID != 1002 {
		t.Fatalf("unexpected pcards: %+v", data.Payload.Pcards)
	}
	if data.Payload.FramePaths.Base != "player_frame/frame_pack/9001/horizontal/frame_base.png" {
		t.Fatalf("unexpected frame path: %s", data.Payload.FramePaths.Base)
	}
	if len(data.Payload.MusicDifficultyCount) < 5 || data.Payload.MusicDifficultyCount[4].Difficulty != "master" || data.Payload.MusicDifficultyCount[4].Clear != 10 || data.Payload.MusicDifficultyCount[4].Fc != 5 || data.Payload.MusicDifficultyCount[4].Ap != 1 {
		t.Fatalf("unexpected music counts: %+v", data.Payload.MusicDifficultyCount)
	}
	if len(data.Payload.CharacterRank) != 1 || data.Payload.CharacterRank[0].CharacterID != 1 || data.Payload.CharacterRank[0].Rank != 30 {
		t.Fatalf("unexpected character rank: %+v", data.Payload.CharacterRank)
	}
	if data.Payload.SoloLive.CharacterID != 2 || data.Payload.SoloLive.Score != 234567 || data.Payload.SoloLive.Rank != 7 {
		t.Fatalf("unexpected solo live: %+v", data.Payload.SoloLive)
	}
}

func TestPJSKProfileRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != profileDrawingEndpoint {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PROFILEPNG"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := profileRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/profile/render", strings.NewReader(`{"region":"jp"}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "PROFILEPNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKMiscBirthdayBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/misc/chara-birthday/build", `{"cid":1,"month":8,"day":31,"cards":[{"id":1001,"thumbnail_path":"thumb.png"}]}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			Cid int `json:"cid"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != charaBirthdayEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if data.Payload.Cid != 1 {
		t.Fatalf("unexpected payload: %+v", data.Payload)
	}
}

func TestPJSKMiscBirthdayRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != charaBirthdayEndpoint {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("BIRTHDAYPNG"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/misc/chara-birthday/render", strings.NewReader(`{"cid":1,"month":8,"day":31,"cards":[{"id":1001,"thumbnail_path":"thumb.png"}]}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "BIRTHDAYPNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKMysekaiResourceBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := mysekaiRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/mysekai/resource/build", `{"region":"jp"}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			GateID    int `json:"gate_id"`
			GateLevel int `json:"gate_level"`
			Profile   struct {
				MysekaiLevel *int `json:"mysekai_level"`
			} `json:"profile"`
			VisitCharacters []struct {
				SdImagePath string `json:"sd_image_path"`
			} `json:"visit_characters"`
			SiteResourceNumbers []struct {
				ImagePath string `json:"image_path"`
			} `json:"site_resource_numbers"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != mysekaiResourceEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if data.Payload.GateID != 1 || data.Payload.GateLevel != 1 {
		t.Fatalf("unexpected gate payload: %+v", data.Payload)
	}
	if data.Payload.Profile.MysekaiLevel == nil || *data.Payload.Profile.MysekaiLevel != 15 {
		t.Fatalf("unexpected mysekai level: %+v", data.Payload.Profile)
	}
	if len(data.Payload.VisitCharacters) != 1 || data.Payload.VisitCharacters[0].SdImagePath != "character/character_sd_l/chr_sp_1.png" {
		t.Fatalf("unexpected visit characters: %+v", data.Payload.VisitCharacters)
	}
	if len(data.Payload.SiteResourceNumbers) != 1 || data.Payload.SiteResourceNumbers[0].ImagePath != "mysekai/site/sitemap/texture/img_harvest_site_5.png" {
		t.Fatalf("unexpected site resources: %+v", data.Payload.SiteResourceNumbers)
	}
}

func TestPJSKMysekaiResourceRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != mysekaiResourceEndpoint {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("MYSEKAIPNG"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := mysekaiRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/mysekai/resource/render", strings.NewReader(`{"region":"jp"}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "MYSEKAIPNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKMysekaiFixtureListBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := mysekaiRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/mysekai/fixture-list/build", `{"region":"jp"}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			ShowID          bool   `json:"show_id"`
			ProgressMessage string `json:"progress_message"`
			MainGenres      []struct {
				Name      string `json:"name"`
				SubGenres []struct {
					Fixtures []struct {
						ID       int  `json:"id"`
						Obtained bool `json:"obtained"`
					} `json:"fixtures"`
				} `json:"sub_genres"`
			} `json:"main_genres"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != mysekaiFixtureListEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if !data.Payload.ShowID {
		t.Fatalf("expected show_id=true")
	}
	if !strings.Contains(data.Payload.ProgressMessage, "1/1") {
		t.Fatalf("unexpected progress: %s", data.Payload.ProgressMessage)
	}
	if len(data.Payload.MainGenres) != 1 || data.Payload.MainGenres[0].Name != "Main A" {
		t.Fatalf("unexpected main genres: %+v", data.Payload.MainGenres)
	}
	if len(data.Payload.MainGenres[0].SubGenres) != 1 || len(data.Payload.MainGenres[0].SubGenres[0].Fixtures) != 2 {
		t.Fatalf("unexpected fixture list: %+v", data.Payload.MainGenres[0].SubGenres)
	}
}

func TestPJSKMysekaiFixtureDetailBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := mysekaiRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/mysekai/fixture-detail/build", `{"region":"jp","query":"2001"}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  []struct {
			Title         string `json:"title"`
			MainGenreName string `json:"main_genre_name"`
			CostMaterials []struct {
				Quantity int `json:"quantity"`
			} `json:"cost_materials"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != mysekaiFixtureDetailEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if len(data.Payload) != 1 || data.Payload[0].Title != "【JP-2001】Wood Chair" || data.Payload[0].MainGenreName != "Main A" {
		t.Fatalf("unexpected payload: %+v", data.Payload)
	}
	if len(data.Payload[0].CostMaterials) != 1 || data.Payload[0].CostMaterials[0].Quantity != 2 {
		t.Fatalf("unexpected cost materials: %+v", data.Payload[0].CostMaterials)
	}
}

func TestPJSKMysekaiDoorUpgradeBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := mysekaiRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/mysekai/door-upgrade/build", `{"region":"jp","query":"1"}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			GateMaterials []struct {
				ID             int `json:"id"`
				LevelMaterials []struct {
					Level int `json:"level"`
				} `json:"level_materials"`
			} `json:"gate_materials"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != mysekaiDoorUpgradeEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if len(data.Payload.GateMaterials) != 1 || data.Payload.GateMaterials[0].ID != 1 {
		t.Fatalf("unexpected gate materials: %+v", data.Payload.GateMaterials)
	}
	if len(data.Payload.GateMaterials[0].LevelMaterials) != 1 || data.Payload.GateMaterials[0].LevelMaterials[0].Level != 2 {
		t.Fatalf("unexpected level materials: %+v", data.Payload.GateMaterials[0].LevelMaterials)
	}
}

func TestPJSKMysekaiMusicRecordBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := mysekaiRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/mysekai/music-record/build", `{"region":"jp","show_id":true}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			ProgressMessage      string `json:"progress_message"`
			CategoryMusicrecords []struct {
				Tag          string `json:"tag"`
				Musicrecords []struct {
					ID       *int `json:"id"`
					Obtained bool `json:"obtained"`
				} `json:"musicrecords"`
			} `json:"category_musicrecords"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != mysekaiMusicRecordEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if !strings.Contains(data.Payload.ProgressMessage, "1/1") {
		t.Fatalf("unexpected progress: %s", data.Payload.ProgressMessage)
	}
	if len(data.Payload.CategoryMusicrecords) != 1 || data.Payload.CategoryMusicrecords[0].Tag != "light_music_club" {
		t.Fatalf("unexpected categories: %+v", data.Payload.CategoryMusicrecords)
	}
	if len(data.Payload.CategoryMusicrecords[0].Musicrecords) != 1 || data.Payload.CategoryMusicrecords[0].Musicrecords[0].ID == nil || *data.Payload.CategoryMusicrecords[0].Musicrecords[0].ID != 101 || !data.Payload.CategoryMusicrecords[0].Musicrecords[0].Obtained {
		t.Fatalf("unexpected records: %+v", data.Payload.CategoryMusicrecords[0].Musicrecords)
	}
}

func TestPJSKMysekaiTalkListBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := mysekaiRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/mysekai/talk-list/build", `{"region":"jp","query":"ick"}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			SdImagePath      string `json:"sd_image_path"`
			ProgressMessage  string `json:"progress_message"`
			SingleMainGenres []struct {
				Name string `json:"name"`
			} `json:"single_main_genres"`
			MultiReads []struct {
				NoreadNum int `json:"noread_num"`
			} `json:"multi_reads"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != mysekaiTalkListEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Payload.SdImagePath != "character/character_sd_l/chr_sp_1.png" {
		t.Fatalf("unexpected sd image path: %s", data.Payload.SdImagePath)
	}
	if !strings.Contains(data.Payload.ProgressMessage, "0/2") {
		t.Fatalf("unexpected progress: %s", data.Payload.ProgressMessage)
	}
	if len(data.Payload.SingleMainGenres) != 1 || data.Payload.SingleMainGenres[0].Name != "Main A" {
		t.Fatalf("unexpected single genres: %+v", data.Payload.SingleMainGenres)
	}
	if len(data.Payload.MultiReads) != 1 || data.Payload.MultiReads[0].NoreadNum != 1 {
		t.Fatalf("unexpected multi reads: %+v", data.Payload.MultiReads)
	}
}

func TestPJSKScoreControlBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/score/control/build", `{"music_id":1,"target_point":100,"music_cover_path":"jacket/jacket_s_001_rip/jacket_s_001.png"}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			MusicCoverPath string `json:"music_cover_path"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != scoreControlEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if data.Payload.MusicCoverPath != "music/jacket/jacket_s_001/jacket_s_001.png" {
		t.Fatalf("unexpected payload: %+v", data.Payload)
	}
}

func TestPJSKScoreCustomRoomBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/score/custom-room/build", `{"target_point":100,"candidate_pairs":[[1,2]],"music_list_map":{"1":[{"music_cover":"jacket/jacket_s_002_rip/jacket_s_002.png"}]}}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			MusicListMap map[string][]struct {
				MusicCover string `json:"music_cover"`
			} `json:"music_list_map"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != scoreCustomRoomEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	items := data.Payload.MusicListMap["1"]
	if len(items) != 1 || items[0].MusicCover != "music/jacket/jacket_s_002/jacket_s_002.png" {
		t.Fatalf("unexpected payload: %+v", data.Payload.MusicListMap)
	}
}

func TestPJSKScoreMusicMetaRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != scoreMusicMetaEndpoint {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SCOREPNG"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/score/music-meta/render", strings.NewReader(`[{"music_id":1,"music_cover_path":"jacket/jacket_s_001_rip/jacket_s_001.png","metas":[]}]`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "SCOREPNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKScoreMusicBoardRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != scoreMusicBoardEndpoint {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("BOARDPNG"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/score/music-board/render", strings.NewReader(`{"items":[{"music_id":1,"music_cover_path":"jacket/jacket_s_003_rip/jacket_s_003.png"}]}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "BOARDPNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKMusicDetailBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/music/detail/build", `{"query":"100","region":"jp"}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			MusicInfo struct {
				ID    int    `json:"id"`
				Title string `json:"title"`
			} `json:"music_info"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != musicDetailDrawingEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if data.Payload.MusicInfo.ID != 100 || data.Payload.MusicInfo.Title == "" {
		t.Fatalf("unexpected payload: %+v", data.Payload)
	}
}

func TestPJSKMusicBriefListBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/music/brief-list/build", `{"music_ids":[100],"difficulty":"master","region":"jp"}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			MusicList []struct {
				ID    int `json:"id"`
				Level int `json:"level"`
			} `json:"music_list"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != musicBriefDrawingEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if len(data.Payload.MusicList) != 1 || data.Payload.MusicList[0].ID != 100 || data.Payload.MusicList[0].Level != 31 {
		t.Fatalf("unexpected payload: %+v", data.Payload.MusicList)
	}
}

func TestPJSKMusicListBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/music/list/build", `{"difficulty":"master","region":"jp","show_id":true,"include_leaks":true}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			RequiredDifficulties string `json:"required_difficulties"`
			MusicList            []struct {
				ID int `json:"id"`
			} `json:"music_list"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != musicListDrawingEndpoint(true, true) {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if data.Payload.RequiredDifficulties != "master" || len(data.Payload.MusicList) != 1 || data.Payload.MusicList[0].ID != 100 {
		t.Fatalf("unexpected payload: %+v", data.Payload)
	}
}

func TestPJSKMusicListRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/music/list" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("show_id"); got != "true" {
			t.Fatalf("unexpected show_id query: %s", got)
		}
		if got := r.URL.Query().Get("show_leak"); got != "true" {
			t.Fatalf("unexpected show_leak query: %s", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("MUSICLISTPNG"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/music/list/render", strings.NewReader(`{"difficulty":"master","region":"jp","show_id":true,"include_leaks":true}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "MUSICLISTPNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKMusicProgressBuildRouteReturnsSnapshotPayload(t *testing.T) {
	app := fiber.New()
	runtime := musicSnapshotRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/music/progress/build", `{"difficulty":"master","region":"jp"}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			Difficulty string `json:"difficulty"`
			Profile    struct {
				Profile struct {
					Nickname string `json:"nickname"`
				} `json:"profile"`
			} `json:"profile"`
			Counts []struct {
				Level    int `json:"level"`
				Total    int `json:"total"`
				NotClear int `json:"not_clear"`
				Clear    int `json:"clear"`
				Fc       int `json:"fc"`
				Ap       int `json:"ap"`
			} `json:"counts"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != musicProgressEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if data.Payload.Difficulty != "master" {
		t.Fatalf("unexpected difficulty: %s", data.Payload.Difficulty)
	}
	if data.Payload.Profile.Profile.Nickname != "Snapshot User" {
		t.Fatalf("unexpected profile payload: %+v", data.Payload.Profile)
	}
	if len(data.Payload.Counts) != 2 {
		t.Fatalf("unexpected counts length: %d", len(data.Payload.Counts))
	}
	if first := data.Payload.Counts[0]; first.Level != 31 || first.Total != 2 || first.Clear != 2 || first.Fc != 2 || first.Ap != 1 || first.NotClear != 0 {
		t.Fatalf("unexpected first count payload: %+v", first)
	}
	if second := data.Payload.Counts[1]; second.Level != 32 || second.Total != 1 || second.NotClear != 1 || second.Clear != 0 {
		t.Fatalf("unexpected second count payload: %+v", second)
	}
}

func TestPJSKMusicProgressRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != musicProgressEndpoint {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PROGRESSPNG"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := musicSnapshotRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/music/progress/render", strings.NewReader(`{"difficulty":"master","region":"jp"}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "PROGRESSPNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKMusicRewardsDetailBuildRouteReturnsSnapshotPayload(t *testing.T) {
	app := fiber.New()
	runtime := musicSnapshotRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/music/rewards/detail/build", `{"region":"jp","rank_rewards":50,"combo_rewards":{"master":[{"level":100,"reward":50}]}}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			RankRewards   int    `json:"rank_rewards"`
			JewelIconPath string `json:"jewel_icon_path"`
			ShardIconPath string `json:"shard_icon_path"`
			Profile       struct {
				Profile struct {
					Nickname string `json:"nickname"`
				} `json:"profile"`
			} `json:"profile"`
			ComboRewards map[string][]struct {
				Level  int `json:"level"`
				Reward int `json:"reward"`
			} `json:"combo_rewards"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != musicRewardsDetailEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if data.Payload.RankRewards != 50 || data.Payload.Profile.Profile.Nickname != "Snapshot User" {
		t.Fatalf("unexpected payload: %+v", data.Payload)
	}
	if data.Payload.JewelIconPath != "lunabot_static_images/jewel.png" || data.Payload.ShardIconPath != "lunabot_static_images/shard.png" {
		t.Fatalf("unexpected reward icon paths: jewel=%s shard=%s", data.Payload.JewelIconPath, data.Payload.ShardIconPath)
	}
	if len(data.Payload.ComboRewards["master"]) != 1 || len(data.Payload.ComboRewards["hard"]) != 0 || len(data.Payload.ComboRewards["append"]) != 0 {
		t.Fatalf("unexpected combo rewards payload: %+v", data.Payload.ComboRewards)
	}
}

func TestPJSKMusicRewardsBasicRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != musicRewardsBasicEndpoint {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("REWARDSPNG"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := musicSnapshotRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/music/rewards/basic/render", strings.NewReader(`{"region":"jp","rank_rewards":"10","combo_rewards":{"master":"5"}}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "REWARDSPNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKMusicChartBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/music/chart/build", `{"query":"100","region":"jp","difficulty":"master"}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			MusicID    int    `json:"music_id"`
			Difficulty string `json:"difficulty"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != musicChartDrawingEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if data.Payload.MusicID != 100 || data.Payload.Difficulty != "master" {
		t.Fatalf("unexpected payload: %+v", data.Payload)
	}
}

func TestPJSKEducationPowerBonusBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/education/power-bonus/build", `{"profile":{"id":"1","region":"JP","nickname":"Test","source":"suite","update_time":1,"is_hide_uid":true,"leader_image_path":"leader.png","has_frame":false},"chara_bonuses":[{"chara_id":1,"chara_icon_path":"icon.png","area_item":1,"rank":2,"fixture":3,"total":6}]}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			CharaBonuses []struct {
				CharaID int `json:"chara_id"`
			} `json:"chara_bonuses"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != educationPowerEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if len(data.Payload.CharaBonuses) != 1 || data.Payload.CharaBonuses[0].CharaID != 1 {
		t.Fatalf("unexpected payload: %+v", data.Payload)
	}
}

func TestPJSKEducationChallengeLiveBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := challengeLiveRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/education/challenge-live/build", `{"region":"jp"}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			Profile struct {
				Nickname        string `json:"nickname"`
				LeaderImagePath string `json:"leader_image_path"`
			} `json:"profile"`
			MaxScore            int `json:"max_score"`
			CharacterChallenges []struct {
				CharaID int `json:"chara_id"`
				Rank    int `json:"rank"`
				Score   int `json:"score"`
				Jewel   int `json:"jewel"`
				Shard   int `json:"shard"`
			} `json:"character_challenges"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != educationChallengeEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if data.Payload.Profile.Nickname != "Test User" || data.Payload.Profile.LeaderImagePath != "user/leader.png" {
		t.Fatalf("unexpected profile payload: %+v", data.Payload.Profile)
	}
	if data.Payload.MaxScore != 3000000 {
		t.Fatalf("unexpected max score: %d", data.Payload.MaxScore)
	}
	if len(data.Payload.CharacterChallenges) != 26 {
		t.Fatalf("unexpected challenge count: %d", len(data.Payload.CharacterChallenges))
	}
	first := data.Payload.CharacterChallenges[0]
	if first.CharaID != 1 || first.Rank != 5 || first.Score != 123456 || first.Jewel != 300 || first.Shard != 10 {
		t.Fatalf("unexpected first challenge payload: %+v", first)
	}
}

func TestPJSKEducationLeaderCountRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != educationLeaderEndpoint {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("LEADERPNG"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/education/leader-count/render", strings.NewReader(`{"profile":{"id":"1","region":"JP","nickname":"Test","source":"suite","update_time":1,"is_hide_uid":true,"leader_image_path":"leader.png","has_frame":false},"leader_counts":[{"chara_id":1,"chara_icon_path":"icon.png","play_count":10,"ex_level":5,"ex_count":2}],"max_play_count":10}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "LEADERPNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKEducationChallengeLiveRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != educationChallengeEndpoint {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("CHALLENGEPNG"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := challengeLiveRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/education/challenge-live/render", strings.NewReader(`{"region":"jp"}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "CHALLENGEPNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKSKLineBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/sk/line/build", `{"full":true,"id":1,"region":"jp","start_at":1,"aggregate_at":2,"name":"Test Event","banner_img_path":"banner.png","ranks":[{"rank":1,"name":"A","time":"2024-01-01T00:00:00Z"}]}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			Full  bool `json:"full"`
			ID    int  `json:"id"`
			Ranks []struct {
				Rank int `json:"rank"`
			} `json:"ranks"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != skLineEndpoint(true) {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if !data.Payload.Full || data.Payload.ID != 1 || len(data.Payload.Ranks) != 1 || data.Payload.Ranks[0].Rank != 1 {
		t.Fatalf("unexpected payload: %+v", data.Payload)
	}
}

func TestPJSKSKQueryRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != skQueryEndpoint {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKPNG"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/sk/query/render", strings.NewReader(`{"id":1,"region":"jp","name":"Test Event","aggregate_at":2,"ranks":[{"rank":1,"name":"A","time":"2024-01-01T00:00:00Z"}]}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "SKPNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKSKQueryTrackerBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	runtime.SK.SetTrackerIntegration(
		routeTrackerSource{},
		&routeEventSource{
			region: renderregion.JP,
			events: []*masterdata.Event{
				{ID: 101, Name: "Tracker Event", StartAt: 111, AggregateAt: 222, AssetBundleName: "event_101"},
			},
		},
		assets.NewAssetHelper("", nil),
	)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/sk/query/tracker/build", `{"event_id":101,"region":"jp","ranks":[100,1,1]}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			ID          int    `json:"id"`
			AggregateAt int64  `json:"aggregate_at"`
			Name        string `json:"name"`
			Ranks       []struct {
				Rank int `json:"rank"`
			} `json:"ranks"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != skQueryEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if data.Payload.ID != 101 {
		t.Fatalf("unexpected event id: %d", data.Payload.ID)
	}
	if data.Payload.Name != "Tracker Event" {
		t.Fatalf("unexpected event name: %s", data.Payload.Name)
	}
	if data.Payload.AggregateAt != 222 {
		t.Fatalf("unexpected aggregate_at: %d", data.Payload.AggregateAt)
	}
	if len(data.Payload.Ranks) != 2 || data.Payload.Ranks[0].Rank != 1 || data.Payload.Ranks[1].Rank != 100 {
		t.Fatalf("unexpected ranks: %+v", data.Payload.Ranks)
	}
}

func TestPJSKSKQueryTrackerBuildRouteSupportsUID(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	runtime.SK.SetTrackerIntegration(
		routeTrackerSource{},
		&routeEventSource{
			region: renderregion.JP,
			events: []*masterdata.Event{
				{ID: 101, Name: "Tracker Event", StartAt: 111, AggregateAt: 222, AssetBundleName: "event_101"},
			},
		},
		assets.NewAssetHelper("", nil),
	)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/sk/query/tracker/build", `{"event_id":101,"region":"jp","user_id":1234567890}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Payload struct {
			ID    int `json:"id"`
			Ranks []struct {
				Rank int `json:"rank"`
			} `json:"ranks"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Payload.ID != 101 {
		t.Fatalf("unexpected event id: %d", data.Payload.ID)
	}
	if len(data.Payload.Ranks) != 1 || data.Payload.Ranks[0].Rank != 321 {
		t.Fatalf("unexpected uid rank payload: %+v", data.Payload.Ranks)
	}
}

func TestPJSKSKLineTrackerRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/line" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("full"); got != "true" {
			t.Fatalf("unexpected full query value: %s", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKLINE_TRACKER_PNG"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	runtime.SK.SetTrackerIntegration(
		routeTrackerSource{},
		&routeEventSource{
			region: renderregion.JP,
			events: []*masterdata.Event{
				{ID: 101, Name: "Tracker Event", StartAt: 111, AggregateAt: 222, AssetBundleName: "event_101"},
			},
		},
		assets.NewAssetHelper("", nil),
	)
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/sk/line/tracker/render", strings.NewReader(`{"event_id":101,"region":"jp","ranks":[1],"full":true}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "SKLINE_TRACKER_PNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKSKCheckRoomTrackerBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	runtime.SK.SetTrackerIntegration(
		routeTrackerSource{},
		&routeEventSource{
			region: renderregion.JP,
			events: []*masterdata.Event{
				{ID: 101, Name: "Tracker Event", StartAt: 111, AggregateAt: 222, AssetBundleName: "event_101"},
			},
		},
		assets.NewAssetHelper("", nil),
	)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/sk/check-room/tracker/build", `{"event_id":101,"region":"jp","ranks":[100,1]}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Payload  struct {
			Eid   int `json:"eid"`
			Ranks []struct {
				Rank int `json:"rank"`
			} `json:"ranks"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != skCheckRoomEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Payload.Eid != 101 {
		t.Fatalf("unexpected event id: %d", data.Payload.Eid)
	}
	if len(data.Payload.Ranks) != 2 || data.Payload.Ranks[0].Rank != 1 || data.Payload.Ranks[1].Rank != 100 {
		t.Fatalf("unexpected ranks: %+v", data.Payload.Ranks)
	}
}

func TestPJSKSKSpeedTrackerBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	runtime.SK.SetTrackerIntegration(
		routeTrackerSource{},
		&routeEventSource{
			region: renderregion.JP,
			events: []*masterdata.Event{
				{ID: 101, Name: "Tracker Event", StartAt: 111, AggregateAt: 222, AssetBundleName: "event_101"},
			},
		},
		assets.NewAssetHelper("", nil),
	)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/sk/speed/tracker/build", `{"event_id":101,"region":"jp","ranks":[100,1]}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Payload  struct {
			EventID     int    `json:"event_id"`
			RequestType string `json:"request_type"`
			Period      int64  `json:"period"`
			Ranks       []struct {
				Rank int `json:"rank"`
			} `json:"ranks"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != skSpeedEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Payload.EventID != 101 {
		t.Fatalf("unexpected event id: %d", data.Payload.EventID)
	}
	if data.Payload.RequestType != "tracker" {
		t.Fatalf("unexpected request_type: %s", data.Payload.RequestType)
	}
	if data.Payload.Period <= 0 {
		t.Fatalf("unexpected period: %d", data.Payload.Period)
	}
	if len(data.Payload.Ranks) != 2 || data.Payload.Ranks[0].Rank != 1 || data.Payload.Ranks[1].Rank != 100 {
		t.Fatalf("unexpected ranks: %+v", data.Payload.Ranks)
	}
}

func TestPJSKSKRankTraceTrackerRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/rank-trace" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKTRACE_TRACKER_PNG"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	runtime.SK.SetTrackerIntegration(
		routeTrackerSource{},
		&routeEventSource{
			region: renderregion.JP,
			events: []*masterdata.Event{
				{ID: 101, Name: "Tracker Event", StartAt: 111, AggregateAt: 222, AssetBundleName: "event_101"},
			},
		},
		assets.NewAssetHelper("", nil),
	)
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/sk/rank-trace/tracker/render", strings.NewReader(`{"event_id":101,"region":"jp","ranks":[100]}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "SKTRACE_TRACKER_PNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKRenderRoutesRequireAuthorizationWhenConfigured(t *testing.T) {
	oldAuth := config.Cfg.Backend.AcceptAuthorization
	oldUA := config.Cfg.Backend.AcceptUserAgent
	config.Cfg.Backend.AcceptAuthorization = "Bearer internal-token"
	config.Cfg.Backend.AcceptUserAgent = ""
	defer func() {
		config.Cfg.Backend.AcceptAuthorization = oldAuth
		config.Cfg.Backend.AcceptUserAgent = oldUA
	}()

	app := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/event/list/build", `{"region":"jp","include_past":true,"include_future":true}`)
	if resp.Status != fiber.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got status=%d message=%s", resp.Status, resp.Message)
	}
}

func requestRenderRoute(t *testing.T, app *fiber.App, method, path, body string) renderEnvelope {
	t.Helper()

	req, err := http.NewRequest(method, path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	var envelope renderEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode response: %v raw=%s", err, string(payload))
	}
	return envelope
}

func testRenderApp(t *testing.T, drawingClient *drawing.HarukiDrawingClient) *renderapp.App {
	t.Helper()

	cardSource := &routeCardSource{
		region: renderregion.JP,
		cards: map[int]*masterdata.Card{
			1001: {
				ID:              1001,
				CharacterID:     5,
				CardRarityType:  "rarity_4",
				Attr:            "cute",
				Prefix:          "Test Card",
				AssetBundleName: "card_1001",
				ReleaseAt:       1700000000000,
				SkillID:         9001,
				CardSkillName:   "Score Up",
				CardParameters: []masterdata.CardParameter{
					{CardParameterType: "param1", Power: 100},
					{CardParameterType: "param2", Power: 200},
					{CardParameterType: "param3", Power: 300},
				},
			},
		},
		characters: map[int]*masterdata.Character{
			5: {ID: 5, FirstName: "花里", GivenName: "实乃理", Unit: "idol"},
		},
		skills: map[int]*masterdata.Skill{
			9001: {ID: 9001, DescriptionSpriteName: "score_up"},
		},
		gachasByCard: map[int]*masterdata.Gacha{
			1001: {ID: 3001, Name: "Test Gacha", StartAt: 1700000000000, EndAt: 1700003600000},
		},
		costumes: map[int][]*masterdata.Costume3d{
			1001: {
				{ID: 4001, CharacterID: 5, AssetBundleName: "costume_4001"},
			},
		},
	}
	cardController := rendercard.NewController(cardSource, nil, drawingClient, assets.NewAssetHelper("", nil))

	eventSource := &routeEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{
			{ID: 1, EventType: "marathon", Name: "JP Event", AssetBundleName: "jp_event", StartAt: 100, AggregateAt: 200},
		},
	}
	eventController := renderevent.NewController(eventSource, drawingClient, assets.NewAssetHelper("", nil))

	gachaItem := &masterdata.Gacha{
		ID:              1,
		Name:            "JP Gacha",
		GachaType:       "ceil",
		AssetBundleName: "jp_gacha",
		StartAt:         100,
		EndAt:           200,
		GachaDetails: []masterdata.GachaDetail{
			{CardID: 1001, Weight: 100},
		},
		GachaCardRarityRates: []masterdata.GachaCardRarityRate{
			{CardRarityType: "rarity_4", LotteryType: "normal", Rate: 100},
		},
	}
	gachaSource := &routeGachaSource{
		region: renderregion.JP,
		gachas: []*masterdata.Gacha{gachaItem},
		gachaMap: map[int]*masterdata.Gacha{
			1: gachaItem,
		},
		cardMap: map[int]*masterdata.Card{
			1001: {ID: 1001, CardRarityType: "rarity_4", Attr: "cool", AssetBundleName: "card_1001"},
		},
	}
	gachaController := rendergacha.NewController(gachaSource, drawingClient, assets.NewAssetHelper("", nil))

	honorSource := &routeHonorSource{
		region: renderregion.JP,
		honors: map[int]*masterdata.Honor{
			7001: {
				ID:              7001,
				GroupID:         7010,
				HonorRarity:     "high",
				AssetBundleName: "honor_test",
			},
		},
		groups: map[int]*masterdata.HonorGroup{
			7010: {
				ID:        7010,
				HonorType: "normal",
			},
		},
		bonds:   map[int]*masterdata.BondsHonor{},
		gcuByID: map[int]*masterdata.GameCharacterUnit{},
	}
	honorController := renderhonor.NewController(honorSource, drawingClient, assets.NewAssetHelper("", nil))
	educationController := rendereducation.NewController(drawingClient, assets.NewAssetHelper("", nil), nil, renderregion.JP)
	miscController := rendermisc.NewController(drawingClient)
	musicSource := &routeMusicSource{
		region: renderregion.JP,
		musics: map[int]*masterdata.Music{
			100: {
				ID:              100,
				Categories:      []string{"mv_3d"},
				Title:           "Tell Your World",
				Pronunciation:   "tell your world",
				Lyricist:        "kz",
				Composer:        "kz",
				Arranger:        "kz",
				AssetBundleName: "jacket_s_001",
				PublishedAt:     1700000000000,
			},
		},
		localizedTitles: map[int][]string{
			100: {"Tell Your World", "向你诉说世界"},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			100: {
				{ID: 1, MusicID: 100, MusicDifficulty: "expert", PlayLevel: 26, TotalNoteCount: 777},
				{ID: 2, MusicID: 100, MusicDifficulty: "master", PlayLevel: 31, TotalNoteCount: 999},
			},
		},
		vocals: map[int][]*masterdata.MusicVocal{
			100: {
				{
					ID:              1,
					MusicID:         100,
					MusicVocalType:  "virtual_singer",
					Caption:         "バーチャル・シンガーver.",
					AssetBundleName: "vs_test",
					Characters: []masterdata.MusicVocalCharacter{
						{ID: 1, MusicID: 100, MusicVocalID: 1, CharacterType: "virtual_singer", CharacterID: 21},
					},
				},
			},
		},
		tags: map[int][]string{
			100: {"miku"},
		},
		characters: map[int]*masterdata.Character{
			21: {ID: 21, FirstName: "初音", GivenName: "未来", Unit: "piapro"},
		},
		events: map[int]*masterdata.Event{
			1: {ID: 1, Name: "JP Event", AssetBundleName: "jp_event", StartAt: 100, AggregateAt: 200},
		},
		primaryEventByMusic: map[int]int{
			100: 1,
		},
		musicByEvent: map[int]int{
			1: 100,
		},
		banEvents: map[int][]*masterdata.Event{},
		limited: map[int][]*masterdata.LimitedTimeMusic{
			100: {
				{ID: 1, MusicID: 100, StartAt: 1700000000000, EndAt: 1700003600000},
			},
		},
	}
	musicController := rendermusic.NewController(musicSource, drawingClient, assets.NewAssetHelper("", nil), nil, nil)
	scoreController := renderscore.NewController(drawingClient)
	skController := rendersk.NewController(drawingClient)

	stampSource := &routeStampSource{
		region: renderregion.JP,
		stamps: []masterdata.Stamp{
			{ID: 5001, AssetBundleName: "stamp_test"},
		},
	}
	stampController := renderstamp.NewController(stampSource, drawingClient, assets.NewAssetHelper("", nil))

	return &renderapp.App{
		Drawing:    drawingClient,
		Assets:     assets.NewAssetHelper("", nil),
		Cards:      cardController,
		Edu:        educationController,
		Events:     eventController,
		Gachas:     gachaController,
		Honors:     honorController,
		Misc:       miscController,
		Music:      musicController,
		Score:      scoreController,
		SK:         skController,
		Stamps:     stampController,
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}
}

func musicSnapshotRenderApp(t *testing.T, drawingClient *drawing.HarukiDrawingClient) *renderapp.App {
	t.Helper()

	tempDir := t.TempDir()
	writeFixtureFile(t, filepath.Join(tempDir, "user", "leader.png"), []byte("leader"))
	writeFixtureFile(t, filepath.Join(tempDir, "lunabot_static_images", "jewel.png"), []byte("jewel"))
	writeFixtureFile(t, filepath.Join(tempDir, "lunabot_static_images", "shard.png"), []byte("shard"))

	userJSON := `{
		"now": 1700000000000,
		"userGamedata": {"userId": 10001, "name": "Snapshot User", "deck": 1, "rank": 123},
		"userProfile": {"profileImageType": "default", "word": "hello", "twitterId": "test"},
		"userDecks": [{"deckId": 1, "leader": 1001, "subLeader": 0, "member1": 0, "member2": 0, "member3": 0, "member4": 0, "member5": 0}],
		"userCards": [{"cardId": 1001, "level": 60, "masterRank": 5, "specialTrainingStatus": "done", "defaultImage": "normal", "episodes": []}],
		"userMusicResults": [
			{"musicId": 100, "musicDifficultyType": "master", "playResult": "clear", "fullComboFlg": true, "fullPerfectFlg": true},
			{"musicId": 101, "musicDifficultyType": "master", "playResult": "clear", "fullComboFlg": true, "fullPerfectFlg": false}
		],
		"userChallengeLiveSoloResults": [],
		"userChallengeLiveSoloStages": [],
		"userChallengeLiveSoloHighScoreRewards": []
	}`
	userJSONPath := filepath.Join(tempDir, "user.json")
	writeFixtureFile(t, userJSONPath, []byte(userJSON))

	assetHelper := assets.NewAssetHelper(tempDir, nil)
	snapshot := renderuserdata.NewLocalFileService(nil, assetHelper, renderuserdata.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userJSONPath,
	})

	musicSource := &routeMusicSource{
		region: renderregion.JP,
		musics: map[int]*masterdata.Music{
			100: {ID: 100, Title: "Song A", Pronunciation: "song a", AssetBundleName: "jacket_s_100", PublishedAt: 1700000000000},
			101: {ID: 101, Title: "Song B", Pronunciation: "song b", AssetBundleName: "jacket_s_101", PublishedAt: 1700000000000},
			102: {ID: 102, Title: "Song C", Pronunciation: "song c", AssetBundleName: "jacket_s_102", PublishedAt: 1700000000000},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			100: {{ID: 1, MusicID: 100, MusicDifficulty: "master", PlayLevel: 31, TotalNoteCount: 900}},
			101: {{ID: 2, MusicID: 101, MusicDifficulty: "master", PlayLevel: 31, TotalNoteCount: 901}},
			102: {{ID: 3, MusicID: 102, MusicDifficulty: "master", PlayLevel: 32, TotalNoteCount: 902}},
		},
		localizedTitles:     map[int][]string{},
		vocals:              map[int][]*masterdata.MusicVocal{},
		tags:                map[int][]string{},
		characters:          map[int]*masterdata.Character{},
		events:              map[int]*masterdata.Event{},
		primaryEventByMusic: map[int]int{},
		musicByEvent:        map[int]int{},
		banEvents:           map[int][]*masterdata.Event{},
		limited:             map[int][]*masterdata.LimitedTimeMusic{},
	}

	return &renderapp.App{
		Drawing: drawingClient,
		Assets:  assetHelper,
		Music:   rendermusic.NewController(musicSource, drawingClient, assetHelper, snapshot, nil),
	}
}

func deckRenderApp(t *testing.T, drawingClient *drawing.HarukiDrawingClient) *renderapp.App {
	t.Helper()

	tempDir := t.TempDir()
	writeFixtureFile(t, filepath.Join(tempDir, "user", "leader.png"), []byte("leader"))

	userJSON := `{
		"now": 1700000000000,
		"userGamedata": {"userId": 10001, "name": "Deck User", "deck": 1, "rank": 88},
		"userProfile": {"profileImageType": "default", "word": "deck hello", "twitterId": "deck_test"},
		"userDecks": [{"deckId": 1, "leader": 1001, "subLeader": 0, "member1": 1002, "member2": 1003, "member3": 0, "member4": 0, "member5": 0}],
		"userCards": [
			{"cardId": 1001, "level": 60, "masterRank": 1, "specialTrainingStatus": "done", "defaultImage": "normal", "episodes": []},
			{"cardId": 1002, "level": 50, "masterRank": 2, "specialTrainingStatus": "done", "defaultImage": "normal", "episodes": []},
			{"cardId": 1003, "level": 40, "masterRank": 0, "specialTrainingStatus": "not_done", "defaultImage": "normal", "episodes": []}
		],
		"userMusicResults": [],
		"userChallengeLiveSoloResults": [],
		"userChallengeLiveSoloStages": [],
		"userChallengeLiveSoloHighScoreRewards": []
	}`
	userJSONPath := filepath.Join(tempDir, "user.json")
	writeFixtureFile(t, userJSONPath, []byte(userJSON))

	assetHelper := assets.NewAssetHelper(tempDir, nil)
	snapshot := renderuserdata.NewLocalFileService(nil, assetHelper, renderuserdata.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userJSONPath,
	})

	now := time.Now().UnixMilli()
	cardSource := &routeDeckCardSource{
		region: renderregion.JP,
		cards: map[int]*masterdata.Card{
			1001: {
				ID:              1001,
				CharacterID:     1,
				CardRarityType:  "rarity_4",
				Attr:            "cute",
				AssetBundleName: "card_1001",
				CardParameters: []masterdata.CardParameter{
					{CardParameterType: "param1", Power: 1000},
					{CardParameterType: "param2", Power: 1000},
					{CardParameterType: "param3", Power: 1000},
				},
			},
			1002: {
				ID:                              1002,
				CharacterID:                     2,
				CardRarityType:                  "rarity_4",
				Attr:                            "cool",
				AssetBundleName:                 "card_1002",
				SpecialTrainingPower1BonusFixed: 100,
				SpecialTrainingPower2BonusFixed: 100,
				SpecialTrainingPower3BonusFixed: 100,
				CardParameters: []masterdata.CardParameter{
					{CardParameterType: "param1", Power: 1500},
					{CardParameterType: "param2", Power: 1400},
					{CardParameterType: "param3", Power: 1300},
				},
			},
			1003: {
				ID:              1003,
				CharacterID:     3,
				CardRarityType:  "rarity_3",
				Attr:            "pure",
				AssetBundleName: "card_1003",
				CardParameters: []masterdata.CardParameter{
					{CardParameterType: "param1", Power: 800},
					{CardParameterType: "param2", Power: 700},
					{CardParameterType: "param3", Power: 600},
				},
			},
		},
	}
	eventSource := &routeEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{
			{ID: 1, Name: "Deck Event", AssetBundleName: "deck_event", StartAt: now - 60000, AggregateAt: now + 60000},
		},
	}

	return &renderapp.App{
		Drawing: drawingClient,
		Assets:  assetHelper,
		Decks:   renderdeck.NewController(cardSource, eventSource, drawingClient, assetHelper, snapshot, renderregion.JP),
	}
}

func profileRenderApp(t *testing.T, drawingClient *drawing.HarukiDrawingClient) *renderapp.App {
	t.Helper()

	tempDir := t.TempDir()
	writeFixtureFile(t, filepath.Join(tempDir, "user", "leader.png"), []byte("leader"))

	userJSON := `{
		"now": 1700000000000,
		"userGamedata": {"userId": 10001, "name": "Profile User", "deck": 1, "rank": 123},
		"userProfile": {"profileImageType": "default", "word": "<#abc>hello profile", "twitterId": "profile_test"},
		"userDecks": [{"deckId": 1, "leader": 1001, "subLeader": 0, "member1": 1002, "member2": 1003, "member3": 0, "member4": 0, "member5": 0}],
		"userCards": [
			{"cardId": 1001, "level": 60, "masterRank": 5, "specialTrainingStatus": "done", "defaultImage": "normal", "episodes": []},
			{"cardId": 1002, "level": 50, "masterRank": 2, "specialTrainingStatus": "done", "defaultImage": "special_training", "episodes": []},
			{"cardId": 1003, "level": 40, "masterRank": 1, "specialTrainingStatus": "not_done", "defaultImage": "normal", "episodes": []}
		],
		"userMusicResults": [{"musicId": 100, "musicDifficultyType": "master", "playResult": "clear", "fullComboFlg": true, "fullPerfectFlg": true}],
		"userChallengeLiveSoloResults": [{"characterId": 2, "highScore": 234567}],
		"userChallengeLiveSoloStages": [{"characterId": 2, "rank": 7}],
		"userChallengeLiveSoloHighScoreRewards": [],
		"userCharacters": [{"characterId": 1, "characterRank": 30}],
		"userMusicDifficultyClearCounts": [{"musicDifficultyType": "master", "liveClear": 10, "fullCombo": 5, "allPerfect": 1}],
		"userPlayerFrames": [{"playerFrameId": 9001, "playerFrameAttachStatus": "equipped"}],
		"userHonors": [],
		"userProfileHonors": [],
		"userEventResults": []
	}`
	userJSONPath := filepath.Join(tempDir, "user.json")
	writeFixtureFile(t, userJSONPath, []byte(userJSON))

	assetHelper := assets.NewAssetHelper(tempDir, nil)
	snapshot := renderuserdata.NewLocalFileService(nil, assetHelper, renderuserdata.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userJSONPath,
	})

	source := &routeProfileSource{
		region: renderregion.JP,
		cards: map[int]*masterdata.Card{
			1002: {ID: 1002, CharacterID: 1, CardRarityType: "rarity_4", Attr: "cute", AssetBundleName: "card_1002"},
			1003: {ID: 1003, CharacterID: 2, CardRarityType: "rarity_3", Attr: "cool", AssetBundleName: "card_1003"},
		},
		honors:        map[int]*masterdata.Honor{},
		groups:        map[int]*masterdata.HonorGroup{},
		bonds:         map[int]*masterdata.BondsHonor{},
		gcuByID:       map[int]*masterdata.GameCharacterUnit{},
		frames:        map[int]*masterdata.PlayerFrame{9001: {ID: 9001, PlayerFrameGroupID: 9101}},
		frameGroups:   map[int]*masterdata.PlayerFrameGroup{9101: {ID: 9101, AssetBundleName: "frame_pack"}},
		honorEventIDs: map[int]int{},
	}

	return &renderapp.App{
		Drawing:  drawingClient,
		Assets:   assetHelper,
		Profiles: renderprofile.NewController(source, drawingClient, assetHelper, snapshot),
	}
}

func challengeLiveRenderApp(t *testing.T, drawingClient *drawing.HarukiDrawingClient) *renderapp.App {
	t.Helper()

	tempDir := t.TempDir()
	writeFixtureFile(t, filepath.Join(tempDir, "user", "leader.png"), []byte("leader"))
	writeFixtureFile(t, filepath.Join(tempDir, "chara_icon", "ick.png"), []byte("icon"))
	writeFixtureFile(t, filepath.Join(tempDir, "lunabot_static_images", "jewel.png"), []byte("jewel"))
	writeFixtureFile(t, filepath.Join(tempDir, "lunabot_static_images", "shard.png"), []byte("shard"))

	userJSON := `{
		"now": 1700000000000,
		"userGamedata": {"userId": 10001, "name": "Test User", "deck": 1, "rank": 123},
		"userProfile": {"profileImageType": "default", "word": "hello", "twitterId": "test"},
		"userDecks": [{"deckId": 1, "leader": 1001, "subLeader": 0, "member1": 0, "member2": 0, "member3": 0, "member4": 0, "member5": 0}],
		"userCards": [{"cardId": 1001, "level": 60, "masterRank": 5, "specialTrainingStatus": "done", "defaultImage": "normal", "episodes": []}],
		"userMusicResults": [],
		"userChallengeLiveSoloResults": [{"characterId": 1, "highScore": 123456}],
		"userChallengeLiveSoloStages": [{"characterId": 1, "rank": 5}],
		"userChallengeLiveSoloHighScoreRewards": []
	}`
	userJSONPath := filepath.Join(tempDir, "user.json")
	writeFixtureFile(t, userJSONPath, []byte(userJSON))

	assetHelper := assets.NewAssetHelper(tempDir, nil)
	snapshot := renderuserdata.NewLocalFileService(nil, assetHelper, renderuserdata.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userJSONPath,
	})

	educationSource := &routeEducationSource{
		region: renderregion.JP,
		rewardsByChar: map[int][]*rendereducation.ChallengeReward{
			1: {
				{
					ID:            101,
					CharacterID:   1,
					HighScore:     1000000,
					ResourceBoxID: 201,
				},
			},
		},
		boxesByPurpose: map[string]map[int]*rendereducation.ResourceBox{
			"challenge_live_high_score": {
				201: {
					ID:                 201,
					ResourceBoxPurpose: "challenge_live_high_score",
					Details: []rendereducation.ResourceBoxDetail{
						{ResourceType: "jewel", ResourceID: 1, ResourceQuantity: 300},
						{ResourceType: "material", ResourceID: 15, ResourceQuantity: 10},
					},
				},
			},
		},
	}

	educationController := rendereducation.NewController(drawingClient, assetHelper, snapshot, renderregion.JP)
	educationController.RegisterSource(educationSource)

	return &renderapp.App{
		Drawing: drawingClient,
		Assets:  assetHelper,
		Edu:     educationController,
	}
}

func mysekaiRenderApp(t *testing.T, drawingClient *drawing.HarukiDrawingClient) *renderapp.App {
	t.Helper()

	tempDir := t.TempDir()
	writeFixtureFile(t, filepath.Join(tempDir, "user", "leader.png"), []byte("leader"))

	userJSON := map[string]interface{}{
		"now": 1700000000000,
		"userGamedata": map[string]interface{}{
			"userId": 10001,
			"name":   "MySekai User",
			"deck":   1,
			"rank":   80,
		},
		"userProfile": map[string]interface{}{
			"profileImageType": "default",
			"word":             "hello mysekai",
			"twitterId":        "mysekai_test",
		},
		"userDecks": []map[string]interface{}{
			{"deckId": 1, "leader": 1001, "subLeader": 0, "member1": 0, "member2": 0, "member3": 0, "member4": 0, "member5": 0},
		},
		"userCards": []map[string]interface{}{
			{"cardId": 1001, "level": 60, "masterRank": 5, "specialTrainingStatus": "done", "defaultImage": "normal", "episodes": []interface{}{}},
		},
		"userMusicResults":                      []interface{}{},
		"userChallengeLiveSoloResults":          []interface{}{},
		"userChallengeLiveSoloStages":           []interface{}{},
		"userChallengeLiveSoloHighScoreRewards": []interface{}{},
	}
	userJSONPath := filepath.Join(tempDir, "user.json")
	writeJSONFixture(t, userJSONPath, userJSON)

	mysekaiJSON := map[string]interface{}{
		"userMysekaiGateCharacterVisit": map[string]interface{}{
			"userMysekaiGate": map[string]interface{}{
				"mysekaiGateId":    1,
				"mysekaiGateLevel": 1,
			},
			"userMysekaiGateCharacters": []map[string]interface{}{
				{"mysekaiGameCharacterUnitGroupId": 5001, "isReservation": true},
			},
		},
		"userMysekaiGamedata": map[string]interface{}{
			"mysekaiRank": 15,
		},
		"mysekaiPhenomenaSchedules": []map[string]interface{}{
			{"mysekaiPhenomenaId": 1},
			{"mysekaiPhenomenaId": 2},
		},
		"updatedResources": map[string]interface{}{
			"now": 1700000000000,
			"userMysekaiHarvestMaps": []map[string]interface{}{
				{
					"mysekaiSiteId": 5,
					"userMysekaiSiteHarvestResourceDrops": []map[string]interface{}{
						{"resourceType": "mysekai_material", "resourceId": 12, "quantity": 3, "mysekaiSiteHarvestResourceDropStatus": "before_drop"},
						{"resourceType": "mysekai_music_record", "resourceId": 3001, "quantity": 1, "mysekaiSiteHarvestResourceDropStatus": "before_drop"},
					},
				},
			},
			"userMysekaiBlueprints": []map[string]interface{}{
				{"mysekaiBlueprintId": 1001},
			},
			"userMysekaiMaterials": []map[string]interface{}{
				{"mysekaiMaterialId": 11, "quantity": 5},
				{"mysekaiMaterialId": 12, "quantity": 0},
			},
			"userMysekaiGates": []map[string]interface{}{
				{"mysekaiGateId": 1, "mysekaiGateLevel": 1},
			},
			"userMysekaiMusicRecords": []map[string]interface{}{
				{"mysekaiMusicRecordId": 3001, "obtainedAt": 1700000000000},
			},
			"userMysekaiCharacterTalks": []map[string]interface{}{},
		},
	}
	mysekaiJSONPath := filepath.Join(tempDir, "mysekai.json")
	writeJSONFixture(t, mysekaiJSONPath, mysekaiJSON)

	masterdataDir := filepath.Join(tempDir, "masterdata")
	writeJSONFixture(t, filepath.Join(masterdataDir, "mysekaiGameCharacterUnitGroups.json"), []map[string]interface{}{
		{"id": 5001, "gameCharacterUnitId1": 1},
		{"id": 5002, "gameCharacterUnitId1": 1, "gameCharacterUnitId2": 2},
	})
	writeJSONFixture(t, filepath.Join(masterdataDir, "gameCharacterUnits.json"), []map[string]interface{}{
		{"id": 1, "gameCharacterId": 1},
		{"id": 2, "gameCharacterId": 2},
	})
	writeJSONFixture(t, filepath.Join(masterdataDir, "gameCharacters.json"), []map[string]interface{}{
		{"id": 1, "firstName": "星乃", "givenName": "一歌"},
		{"id": 2, "firstName": "天马", "givenName": "咲希"},
	})
	writeJSONFixture(t, filepath.Join(masterdataDir, "mysekaiMaterials.json"), []map[string]interface{}{
		{"id": 11, "iconAssetbundleName": "wood", "mysekaiMaterialRarityType": "rarity_1"},
		{"id": 12, "iconAssetbundleName": "rare_wood", "mysekaiMaterialRarityType": "rarity_3"},
	})
	writeJSONFixture(t, filepath.Join(masterdataDir, "mysekaiItems.json"), []map[string]interface{}{
		{"id": 21, "iconAssetbundleName": "ticket"},
	})
	writeJSONFixture(t, filepath.Join(masterdataDir, "mysekaiMusicRecords.json"), []map[string]interface{}{
		{"id": 3001, "externalId": 101, "mysekaiMusicTrackType": "music"},
	})
	writeJSONFixture(t, filepath.Join(masterdataDir, "musics.json"), []map[string]interface{}{
		{"id": 101, "assetbundleName": "jacket_s_101", "publishedAt": 1600000000000},
	})
	writeJSONFixture(t, filepath.Join(masterdataDir, "musicTags.json"), []map[string]interface{}{
		{"id": 1, "musicId": 101, "musicTag": "light_music_club"},
	})
	writeJSONFixture(t, filepath.Join(masterdataDir, "limitedTimeMusics.json"), []map[string]interface{}{})
	writeJSONFixture(t, filepath.Join(masterdataDir, "mysekaiFixtures.json"), []map[string]interface{}{
		{
			"id":                             2001,
			"name":                           "Wood Chair",
			"assetbundleName":                "wood_chair",
			"mysekaiFixtureType":             "furniture",
			"mysekaiFixtureMainGenreId":      1,
			"mysekaiFixtureSubGenreId":       11,
			"gridSize":                       map[string]interface{}{"width": 1, "depth": 1, "height": 1},
			"firstPutCost":                   10,
			"secondPutCost":                  15,
			"isAssembled":                    true,
			"isDisassembled":                 true,
			"mysekaiFixturePlayerActionType": "touch",
			"isGameCharacterAction":          true,
			"mysekaiFixtureAnotherColors": []map[string]interface{}{
				{"colorCode": "#FFFFFF"},
			},
			"mysekaiFixtureTagGroup": map[string]interface{}{
				"mysekaiFixtureTagId1": 701,
			},
		},
		{
			"id":                             2002,
			"name":                           "Birthday Lamp（一歌）",
			"assetbundleName":                "birthday_lamp",
			"mysekaiFixtureType":             "furniture",
			"mysekaiFixtureMainGenreId":      1,
			"mysekaiFixtureSubGenreId":       11,
			"gridSize":                       map[string]interface{}{"width": 1, "depth": 1, "height": 1},
			"firstPutCost":                   8,
			"secondPutCost":                  12,
			"isAssembled":                    true,
			"isDisassembled":                 false,
			"mysekaiFixturePlayerActionType": "no_action",
			"isGameCharacterAction":          false,
		},
	})
	writeJSONFixture(t, filepath.Join(masterdataDir, "mysekaiFixtureMainGenres.json"), []map[string]interface{}{
		{"id": 1, "name": "Main A", "assetbundleName": "main_a"},
	})
	writeJSONFixture(t, filepath.Join(masterdataDir, "mysekaiFixtureSubGenres.json"), []map[string]interface{}{
		{"id": 11, "name": "Sub A", "assetbundleName": "sub_a"},
	})
	writeJSONFixture(t, filepath.Join(masterdataDir, "mysekaiBlueprints.json"), []map[string]interface{}{
		{"id": 1001, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 2001, "isEnableSketch": true, "isObtainedByConvert": false, "craftCountLimit": 0},
	})
	writeJSONFixture(t, filepath.Join(masterdataDir, "mysekaiBlueprintMysekaiMaterialCosts.json"), []map[string]interface{}{
		{"id": 1, "mysekaiBlueprintId": 1001, "mysekaiMaterialId": 11, "quantity": 2},
	})
	writeJSONFixture(t, filepath.Join(masterdataDir, "mysekaiFixtureOnlyDisassembleMaterials.json"), []map[string]interface{}{
		{"id": 1, "mysekaiFixtureId": 2001, "mysekaiMaterialId": 11, "quantity": 1},
	})
	writeJSONFixture(t, filepath.Join(masterdataDir, "mysekaiFixtureTags.json"), []map[string]interface{}{
		{"id": 701, "name": "Cozy"},
	})
	writeJSONFixture(t, filepath.Join(masterdataDir, "mysekaiGateMaterialGroups.json"), []map[string]interface{}{
		{"id": 1, "groupId": 1001, "mysekaiMaterialId": 11, "quantity": 2},
		{"id": 2, "groupId": 1002, "mysekaiMaterialId": 12, "quantity": 1},
	})
	writeJSONFixture(t, filepath.Join(masterdataDir, "characterArchiveMysekaiCharacterTalkGroups.json"), []map[string]interface{}{
		{"id": 9001, "archiveDisplayType": "normal"},
		{"id": 9002, "archiveDisplayType": "normal"},
	})
	writeJSONFixture(t, filepath.Join(masterdataDir, "mysekaiCharacterTalkConditions.json"), []map[string]interface{}{
		{"id": 7001, "mysekaiCharacterTalkConditionType": "mysekai_fixture_id", "mysekaiCharacterTalkConditionTypeValue": 2001},
	})
	writeJSONFixture(t, filepath.Join(masterdataDir, "mysekaiCharacterTalkConditionGroups.json"), []map[string]interface{}{
		{"id": 7101, "mysekaiCharacterTalkConditionId": 7001},
	})
	writeJSONFixture(t, filepath.Join(masterdataDir, "mysekaiCharacterTalks.json"), []map[string]interface{}{
		{"id": 8001, "mysekaiCharacterTalkConditionGroupId": 7101, "mysekaiGameCharacterUnitGroupId": 5001, "characterArchiveMysekaiCharacterTalkGroupId": 9001},
		{"id": 8002, "mysekaiCharacterTalkConditionGroupId": 7101, "mysekaiGameCharacterUnitGroupId": 5002, "characterArchiveMysekaiCharacterTalkGroupId": 9002},
	})
	writeJSONFixture(t, filepath.Join(masterdataDir, "mysekai", "system", "fixture_reaction_data", "fixture_reaction_data.json"), map[string]interface{}{
		"FixturerRactions": []map[string]interface{}{
			{
				"FixtureId": 2001,
				"ReactionCharacter": []map[string]interface{}{
					{"CharacterUnitIds": []int{1}},
					{"CharacterUnitIds": []int{1, 2}},
				},
			},
		},
	})

	assetHelper := assets.NewAssetHelper(tempDir, nil)
	snapshot := renderuserdata.NewLocalFileService(nil, assetHelper, renderuserdata.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userJSONPath,
		MySekaiJSON:   mysekaiJSONPath,
	})

	return &renderapp.App{
		Drawing: drawingClient,
		Assets:  assetHelper,
		MySekai: rendermysekai.NewController(drawingClient, snapshot, masterdataDir, renderregion.JP, nil),
	}
}

func writeFixtureFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture dir: %v", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
}

func writeJSONFixture(t *testing.T, path string, value interface{}) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal fixture json: %v", err)
	}
	writeFixtureFile(t, path, data)
}
