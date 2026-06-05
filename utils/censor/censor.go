package censor

import (
	"context"
	"fmt"
	"strings"
	"time"

	ent "haruki-cloud/database/censor"
	"haruki-cloud/database/censor/imagemodcache"
	"haruki-cloud/database/censor/namelog"
	"haruki-cloud/database/censor/result"
	"haruki-cloud/database/censor/shortbio"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/utils/logger"
)

type ResultStatus string

const (
	ResultCompliant    ResultStatus = "合规"
	ResultNonCompliant ResultStatus = "不合规"
)

type ImageModerator interface {
	ImageModerationURL(ctx context.Context, imageURL string) (IMSSuggestion, error)
}

type TextModerator interface {
	TextCensor(text string) (map[string]any, error)
}

type Service struct {
	Client         *ent.Client
	TextCensorAPI  TextModerator  // text censor (Baidu)
	ImageCensorAPI ImageModerator // image censor (Tencent IMS); nil = disabled
	Logger         *logger.Logger
}

func (s *Service) CensorName(ctx context.Context, harukiUserID int, userID string, name string, server string) bool {
	if name == "" || strings.EqualFold(strings.TrimSpace(server), string(renderregion.CN)) {
		return true
	}

	existing, err := s.Client.Result.
		Query().
		Where(result.NameEQ(name)).
		Only(ctx)
	if err == nil && existing != nil {
		if existing.Result != nil {
			return *existing.Result == 1
		}
		return false
	}

	data, err := s.TextCensorAPI.TextCensor(name)
	if err != nil {
		s.Logger.Errorf("审核名字请求失败，不写入审核缓存: %v", err)
		return false
	}

	censorResult := 0
	if conclusion, ok := data["conclusion"].(string); ok && conclusion == string(ResultCompliant) {
		censorResult = 1
	} else {
		s.Logger.Debugf("名字审核不通过: harukiUserID: %d", harukiUserID)
	}

	_, err = s.Client.Result.
		Create().
		SetName(name).
		SetResult(censorResult).
		Save(ctx)
	if err != nil {
		s.Logger.Errorf("插入 censor_result 失败: %v", err)
	}

	exists, _ := s.Client.NameLog.
		Query().
		Where(
			namelog.UserIDEQ(fmt.Sprint(userID)),
			namelog.NameEQ(name),
			namelog.HarukiUserIDEQ(harukiUserID),
		).
		Exist(ctx)
	if !exists {
		text := string(ResultCompliant)
		if censorResult == 0 {
			text = string(ResultNonCompliant)
		}
		_, err := s.Client.NameLog.
			Create().
			SetUserID(fmt.Sprint(userID)).
			SetName(name).
			SetHarukiUserID(harukiUserID).
			SetResult(text).
			SetTime(time.Now()).
			Save(ctx)
		if err != nil {
			s.Logger.Errorf("插入 name_log 失败: %v", err)
		}
	}

	return censorResult == 1
}

func (s *Service) CensorShortBio(ctx context.Context, harukiUserID int, userID string, content string, server string) bool {
	if content == "" || strings.EqualFold(strings.TrimSpace(server), string(renderregion.CN)) {
		return true
	}

	existing, err := s.Client.ShortBio.
		Query().
		Where(shortbio.ContentEQ(content)).
		Only(ctx)
	if err == nil && existing != nil {
		if existing.Result != nil {
			return *existing.Result == string(ResultCompliant)
		}
		return false
	}

	data, err := s.TextCensorAPI.TextCensor(content)
	if err != nil {
		s.Logger.Errorf("审核短句请求失败，不写入审核缓存: %v", err)
		return false
	}

	censorResult := ResultNonCompliant
	if conclusion, ok := data["conclusion"].(string); ok && conclusion == string(ResultCompliant) {
		censorResult = ResultCompliant
	}

	_, err = s.Client.ShortBio.
		Create().
		SetUserID(fmt.Sprint(userID)).
		SetContent(content).
		SetHarukiUserID(harukiUserID).
		SetResult(string(censorResult)).
		Save(ctx)
	if err != nil {
		s.Logger.Errorf("插入 short_bio 失败: %v", err)
	}

	return censorResult == ResultCompliant
}

// CensorImage submits an image CDN URL to Tencent IMS for content moderation.
// Results are cached in the ent image_mod_cache table to avoid redundant API calls.
// Returns true if the image passes, the image censor is not configured, or the request fails.
func (s *Service) CensorImage(ctx context.Context, harukiUserID int, imageURL string) bool {
	if s.ImageCensorAPI == nil {
		return true
	}
	// Check ent cache first
	existing, err := s.Client.ImageModCache.
		Query().
		Where(imagemodcache.URLEQ(imageURL)).
		Only(ctx)
	if err == nil && existing != nil {
		return existing.Result == string(IMSSuggestionPass)
	}

	suggestion, err := s.ImageCensorAPI.ImageModerationURL(ctx, imageURL)
	if err != nil {
		s.Logger.Errorf("图片审核请求失败，跳过审核并按通过处理: %v", err)
		return true
	}
	if suggestion != IMSSuggestionPass {
		s.Logger.Debugf("图片审核不通过 (suggestion=%s): %s", suggestion, imageURL)
	}

	// Insert into cache
	create := s.Client.ImageModCache.Create().
		SetURL(imageURL).
		SetResult(string(suggestion)).
		SetCreatedAt(time.Now())
	if harukiUserID > 0 {
		create = create.SetHarukiUserID(harukiUserID)
	}
	if _, err := create.Save(ctx); err != nil {
		s.Logger.Errorf("插入 image_mod_cache 失败: %v", err)
	}

	return suggestion == IMSSuggestionPass
}

// NewService creates a censor Service with both text (Baidu) and image (Tencent IMS) censors.
// Pass empty tencentSecretID / tencentSecretKey to disable image censoring.
func NewService(
	baiduAPIKey, baiduSecretKey string,
	tencentSecretID, tencentSecretKey, tencentRegion, tencentBizType string,
	client *ent.Client,
) *Service {
	var imageCensor ImageModerator
	if tencentSecretID != "" && tencentSecretKey != "" {
		imageCensor = NewTencentIMSClient(tencentSecretID, tencentSecretKey, tencentRegion, tencentBizType)
	}
	return &Service{
		Client:         client,
		TextCensorAPI:  NewBaiduTextCensorClient(baiduAPIKey, baiduSecretKey),
		ImageCensorAPI: imageCensor,
		Logger:         logger.NewLoggerFromGlobal("HarukiContentCensorService"),
	}
}

func (s *Service) Close() error {
	if s == nil || s.Client == nil {
		return nil
	}
	return s.Client.Close()
}
