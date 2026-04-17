package music

import (
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

type vocalBuilderTestSource struct {
	music        *masterdata.Music
	vocals       []*masterdata.MusicVocal
	characters   map[int]*masterdata.Character
	outsideByID  map[int]string
	difficulties []*masterdata.MusicDifficulty
}

func (s *vocalBuilderTestSource) DefaultRegion() renderregion.Value             { return renderregion.JP }
func (s *vocalBuilderTestSource) SearchMusic(string) (*masterdata.Music, error) { return s.music, nil }
func (s *vocalBuilderTestSource) GetMusicByID(int) (*masterdata.Music, error)   { return s.music, nil }
func (s *vocalBuilderTestSource) GetMusicByEventID(int) (*masterdata.Music, error) {
	return nil, errNotFound("music")
}
func (s *vocalBuilderTestSource) GetMusics() []*masterdata.Music                { return []*masterdata.Music{s.music} }
func (s *vocalBuilderTestSource) GetBanEvents(int) []*masterdata.Event          { return nil }
func (s *vocalBuilderTestSource) GetMusicLocalizedTitles(int) ([]string, error) { return nil, nil }
func (s *vocalBuilderTestSource) GetMusicDifficulties(int) ([]*masterdata.MusicDifficulty, error) {
	return append([]*masterdata.MusicDifficulty(nil), s.difficulties...), nil
}
func (s *vocalBuilderTestSource) GetMusicVocals(int) ([]*masterdata.MusicVocal, error) {
	return append([]*masterdata.MusicVocal(nil), s.vocals...), nil
}
func (s *vocalBuilderTestSource) GetMusicTags(int) ([]string, error) { return nil, nil }
func (s *vocalBuilderTestSource) GetCharacterByID(id int) (*masterdata.Character, error) {
	if item := s.characters[id]; item != nil {
		return new(*item), nil
	}
	return nil, errNotFound("character")
}
func (s *vocalBuilderTestSource) GetOutsideCharacterByID(id int) (string, error) {
	if name, ok := s.outsideByID[id]; ok {
		return name, nil
	}
	return "", errNotFound("outside character")
}
func (s *vocalBuilderTestSource) GetPrimaryEventByMusicID(int) (*masterdata.Event, error) {
	return nil, errNotFound("event")
}
func (s *vocalBuilderTestSource) GetLimitedTimeMusics(int) []*masterdata.LimitedTimeMusic { return nil }

func TestBuildMusicDetailRequestUsesNameForOutsideCharacters(t *testing.T) {
	source := &vocalBuilderTestSource{
		music: &masterdata.Music{
			ID:              112,
			Title:           "天使のクローバー",
			Composer:        "DIVELA",
			Lyricist:        "DIVELA",
			Arranger:        "DIVELA",
			AssetBundleName: "jacket_s_112",
		},
		difficulties: []*masterdata.MusicDifficulty{
			{MusicID: 112, MusicDifficulty: "easy", PlayLevel: 9, TotalNoteCount: 212},
			{MusicID: 112, MusicDifficulty: "normal", PlayLevel: 13, TotalNoteCount: 390},
			{MusicID: 112, MusicDifficulty: "hard", PlayLevel: 17, TotalNoteCount: 636},
			{MusicID: 112, MusicDifficulty: "expert", PlayLevel: 27, TotalNoteCount: 985},
			{MusicID: 112, MusicDifficulty: "master", PlayLevel: 31, TotalNoteCount: 1235},
		},
		vocals: []*masterdata.MusicVocal{
			{
				ID:              1,
				MusicID:         112,
				MusicVocalType:  "another_vocal",
				Caption:         "Another Vocal",
				AssetBundleName: "an_test",
				Characters: []masterdata.MusicVocalCharacter{
					{CharacterType: "outside_character", CharacterID: 9001},
					{CharacterType: "game_character", CharacterID: 5},
				},
			},
		},
		characters: map[int]*masterdata.Character{
			5: {ID: 5, FirstName: "花里", GivenName: "实乃理", Unit: "idol"},
		},
		outsideByID: map[int]string{
			9001: "重音テト",
		},
	}

	builder := NewBuilder(source, nil, assets.NewAssetHelper("", nil))
	req, err := builder.BuildMusicDetailRequest(source.music, renderregion.JP)
	if err != nil {
		t.Fatalf("BuildMusicDetailRequest() error = %v", err)
	}

	entry, ok := req.Vocal.VocalInfo["30_an_test"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected vocal entry: %#v", req.Vocal.VocalInfo)
	}
	characters, ok := entry["characters"].([]map[string]string)
	if !ok || len(characters) != 2 {
		t.Fatalf("unexpected vocal characters: %#v", entry["characters"])
	}
	if characters[0]["characterName"] != "重音テト" {
		t.Fatalf("expected outside character name, got %#v", characters)
	}
	if _, ok := req.Vocal.VocalAssets["重音テト"]; ok {
		t.Fatalf("outside character should not have vocal asset: %#v", req.Vocal.VocalAssets)
	}
	if req.Vocal.VocalAssets["花里实乃理"] != "static_images/chara_icon/mnr.png" {
		t.Fatalf("expected game character asset, got %#v", req.Vocal.VocalAssets)
	}
}
