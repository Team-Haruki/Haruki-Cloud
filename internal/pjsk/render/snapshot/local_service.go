package snapshot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/drawing"
)

func NewLocalFileService(sekaiClient *sekaiDB.Client, assetHelper *assets.AssetHelper, cfg LocalFileConfig) *Service {
	return NewLocalFileServiceWithContext(context.Background(), sekaiClient, assetHelper, cfg)
}

func NewLocalFileServiceWithContext(ctx context.Context, sekaiClient *sekaiDB.Client, assetHelper *assets.AssetHelper, cfg LocalFileConfig) *Service {
	service := &Service{
		configured: strings.TrimSpace(cfg.UserJSON) != "",
	}
	if !service.configured {
		return service
	}

	data, err := os.ReadFile(filepath.Clean(cfg.UserJSON))
	if err != nil {
		service.initErr = fmt.Errorf("read user snapshot: %w", err)
		return service
	}
	var mysekaiJSON []byte
	if strings.TrimSpace(cfg.MySekaiJSON) != "" {
		mysekaiJSON, err = os.ReadFile(filepath.Clean(cfg.MySekaiJSON))
		if err != nil {
			service.initErr = fmt.Errorf("read mysekai snapshot: %w", err)
			return service
		}
	}

	var musicMetaJSON []byte
	if strings.TrimSpace(cfg.MusicMetaJSON) != "" {
		musicMetaJSON, err = os.ReadFile(filepath.Clean(cfg.MusicMetaJSON))
		if err != nil {
			service.initErr = fmt.Errorf("read music meta snapshot: %w", err)
			return service
		}
	}

	snapshot, err := NewDefaultSnapshotFactory(sekaiClient, assetHelper).Build(ctx, BuildInput{
		Region:         cfg.DefaultRegion,
		Source:         "suite_dump",
		SuiteJSON:      data,
		MySekaiJSON:    mysekaiJSON,
		MusicMetaJSON:  musicMetaJSON,
		MusicMetaPath:  filepath.Clean(cfg.MusicMetaJSON),
		PersistRawFile: true,
		RawFilePattern: "haruki-pjsk-user-*.json",
	})
	if err != nil {
		service.initErr = err
		return service
	}

	built, ok := snapshot.(*Service)
	if !ok {
		service.initErr = fmt.Errorf("userdata: snapshot factory returned unexpected type %T", snapshot)
		return service
	}
	return built
}

func (s *Service) Configured() bool {
	return s != nil && s.configured
}

func (s *Service) Require() error {
	if s == nil || !s.configured {
		return fmt.Errorf("local user snapshot is not configured")
	}
	if s.initErr != nil {
		return s.initErr
	}
	if s.baseProfile == nil {
		return fmt.Errorf("local user snapshot is unavailable")
	}
	return nil
}

func (s *Service) DetailedProfile(region renderregion.Value) *drawing.DetailedProfileCardRequest {
	if s == nil || s.baseProfile == nil {
		return nil
	}
	profile := *s.baseProfile
	if normalized := renderregion.WithDefault(region); !normalized.IsZero() {
		profile.Region = strings.ToUpper(normalized.String())
	}
	profile.Mode = common.CloneStringPtr(s.baseProfile.Mode)
	profile.FramePath = common.CloneStringPtr(s.baseProfile.FramePath)
	profile.UserCards = append([]any(nil), s.baseProfile.UserCards...)
	return &profile
}

func (s *Service) ProfileCard(region renderregion.Value) *drawing.ProfileCardRequest {
	detail := s.DetailedProfile(region)
	if detail == nil {
		return nil
	}
	source := detail.Source
	update := detail.UpdateTime
	return &drawing.ProfileCardRequest{
		Profile: &drawing.BasicProfile{
			ID:              detail.ID,
			Region:          detail.Region,
			Nickname:        detail.Nickname,
			IsHideUID:       detail.IsHideUID,
			LeaderImagePath: detail.LeaderImagePath,
			HasFrame:        detail.HasFrame,
			FramePath:       common.CloneStringPtr(detail.FramePath),
		},
		DataSources: []drawing.ProfileDataSource{
			{
				Name:       "Suite数据",
				Source:     &source,
				UpdateTime: &update,
				Mode:       common.CloneStringPtr(detail.Mode),
			},
		},
	}
}

func (s *Service) MusicResults(diff string) map[int]string {
	if s == nil {
		return nil
	}
	diffKey := strings.ToLower(strings.TrimSpace(diff))
	source := s.musicResult[diffKey]
	copied := make(map[int]string, len(source))
	for musicID, status := range source {
		copied[musicID] = status
	}
	return copied
}

func (s *Service) GetMusicResult(musicID int, diff string) string {
	if s == nil {
		return ""
	}
	diffKey := strings.ToLower(strings.TrimSpace(diff))
	if result, ok := s.musicResult[diffKey][musicID]; ok {
		return result
	}
	return ""
}

func (s *Service) ChallengeLive() *ChallengeLiveData {
	if s == nil || s.challenge == nil {
		return nil
	}
	return &ChallengeLiveData{
		Results: append([]ChallengeLiveResult(nil), s.challenge.Results...),
		Stages:  append([]ChallengeLiveStage(nil), s.challenge.Stages...),
		Rewards: append([]ChallengeLiveReward(nil), s.challenge.Rewards...),
	}
}

func (s *Service) RawBytes() ([]byte, error) {
	if s == nil || len(s.rawJSON) == 0 {
		return nil, fmt.Errorf("raw user snapshot is unavailable")
	}
	return append([]byte(nil), s.rawJSON...), nil
}

func (s *Service) RawFilePath() string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(s.rawFilePath)
}

func (s *Service) RawData() *RawUserData {
	if s == nil {
		return nil
	}
	return s.rawData
}

func (s *Service) MusicMetaBytes() []byte {
	if s == nil || len(s.musicMetaBytes) == 0 {
		return nil
	}
	return append([]byte(nil), s.musicMetaBytes...)
}

func (s *Service) MusicMetaPath() string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(s.musicMetaPath)
}
