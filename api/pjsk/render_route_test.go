package pjsk

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"haruki-cloud/config"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	rendercard "haruki-cloud/internal/pjsk/render/card"
	renderevent "haruki-cloud/internal/pjsk/render/event"
	rendergacha "haruki-cloud/internal/pjsk/render/gacha"
	renderhonor "haruki-cloud/internal/pjsk/render/honor"
	"haruki-cloud/internal/pjsk/render/masterdata"
	rendermisc "haruki-cloud/internal/pjsk/render/misc"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	renderscore "haruki-cloud/internal/pjsk/render/score"
	renderstamp "haruki-cloud/internal/pjsk/render/stamp"
	"haruki-cloud/utils/drawing"

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
	musicController := rendermusic.NewController(musicSource, drawingClient, assets.NewAssetHelper("", nil))
	scoreController := renderscore.NewController(drawingClient)

	stampSource := &routeStampSource{
		region: renderregion.JP,
		stamps: []masterdata.Stamp{
			{ID: 5001, AssetBundleName: "stamp_test"},
		},
	}
	stampController := renderstamp.NewController(stampSource, drawingClient, assets.NewAssetHelper("", nil))

	return &renderapp.App{
		Drawing: drawingClient,
		Assets:  assets.NewAssetHelper("", nil),
		Cards:   cardController,
		Events:  eventController,
		Gachas:  gachaController,
		Honors:  honorController,
		Misc:    miscController,
		Music:   musicController,
		Score:   scoreController,
		Stamps:  stampController,
	}
}
