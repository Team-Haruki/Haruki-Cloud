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
	"haruki-cloud/internal/observability/commandtrace"
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

type contextTextModerator interface {
	TextCensorContext(ctx context.Context, text string) (map[string]any, error)
}

type Service struct {
	Client         *ent.Client
	TextCensorAPI  TextModerator  // text censor (Baidu)
	ImageCensorAPI ImageModerator // image censor (Tencent IMS); nil = disabled
	Logger         *logger.Logger
}

func (s *Service) CensorName(ctx context.Context, harukiUserID int, userID string, name string, server string) bool {
	ctx = censorContext(ctx)
	if name == "" || strings.EqualFold(strings.TrimSpace(server), string(renderregion.CN)) {
		return true
	}

	finishCache := commandtrace.MeasureOperation(ctx, censorCacheStage)
	existing, err := s.Client.Result.
		Query().
		Where(result.NameEQ(name)).
		Only(ctx)
	finishCache()
	if err == nil && existing != nil {
		if existing.Result != nil {
			return *existing.Result == 1
		}
		return false
	}

	data, err := textCensor(ctx, s.TextCensorAPI, name)
	if err != nil {
		s.Logger.ErrorContext(ctx, "name moderation request failed", "error_type", fmt.Sprintf("%T", err))
		return false
	}

	censorResult := 0
	if conclusion, ok := data["conclusion"].(string); ok && conclusion == string(ResultCompliant) {
		censorResult = 1
	} else {
		s.Logger.DebugContext(ctx, "name moderation rejected")
	}

	finishStore := commandtrace.MeasureOperation(ctx, censorStoreStage)
	_, err = s.Client.Result.
		Create().
		SetName(name).
		SetResult(censorResult).
		Save(ctx)
	finishStore()
	if err != nil {
		s.Logger.ErrorContext(ctx, "name moderation cache store failed", "error_type", fmt.Sprintf("%T", err))
	}

	finishCache = commandtrace.MeasureOperation(ctx, censorCacheStage)
	exists, _ := s.Client.NameLog.
		Query().
		Where(
			namelog.UserIDEQ(fmt.Sprint(userID)),
			namelog.NameEQ(name),
			namelog.HarukiUserIDEQ(harukiUserID),
		).
		Exist(ctx)
	finishCache()
	if !exists {
		text := string(ResultCompliant)
		if censorResult == 0 {
			text = string(ResultNonCompliant)
		}
		finishStore = commandtrace.MeasureOperation(ctx, censorStoreStage)
		_, err := s.Client.NameLog.
			Create().
			SetUserID(fmt.Sprint(userID)).
			SetName(name).
			SetHarukiUserID(harukiUserID).
			SetResult(text).
			SetTime(time.Now()).
			Save(ctx)
		finishStore()
		if err != nil {
			s.Logger.ErrorContext(ctx, "name moderation audit store failed", "error_type", fmt.Sprintf("%T", err))
		}
	}

	return censorResult == 1
}

func (s *Service) CensorShortBio(ctx context.Context, harukiUserID int, userID string, content string, server string) bool {
	ctx = censorContext(ctx)
	if content == "" || strings.EqualFold(strings.TrimSpace(server), string(renderregion.CN)) {
		return true
	}

	finishCache := commandtrace.MeasureOperation(ctx, censorCacheStage)
	existing, err := s.Client.ShortBio.
		Query().
		Where(shortbio.ContentEQ(content)).
		Only(ctx)
	finishCache()
	if err == nil && existing != nil {
		if existing.Result != nil {
			return *existing.Result == string(ResultCompliant)
		}
		return false
	}

	data, err := textCensor(ctx, s.TextCensorAPI, content)
	if err != nil {
		s.Logger.ErrorContext(ctx, "short bio moderation request failed", "error_type", fmt.Sprintf("%T", err))
		return false
	}

	censorResult := ResultNonCompliant
	if conclusion, ok := data["conclusion"].(string); ok && conclusion == string(ResultCompliant) {
		censorResult = ResultCompliant
	}

	finishStore := commandtrace.MeasureOperation(ctx, censorStoreStage)
	_, err = s.Client.ShortBio.
		Create().
		SetUserID(fmt.Sprint(userID)).
		SetContent(content).
		SetHarukiUserID(harukiUserID).
		SetResult(string(censorResult)).
		Save(ctx)
	finishStore()
	if err != nil {
		s.Logger.ErrorContext(ctx, "short bio moderation cache store failed", "error_type", fmt.Sprintf("%T", err))
	}

	return censorResult == ResultCompliant
}

// CensorImage submits an image CDN URL to Tencent IMS for content moderation.
// Results are cached in the ent image_mod_cache table to avoid redundant API calls.
// Returns true if the image passes, the image censor is not configured, or the request fails.
func (s *Service) CensorImage(ctx context.Context, harukiUserID int, imageURL string) bool {
	ctx = censorContext(ctx)
	if s.ImageCensorAPI == nil {
		return true
	}
	// Check ent cache first
	finishCache := commandtrace.MeasureOperation(ctx, censorCacheStage)
	existing, err := s.Client.ImageModCache.
		Query().
		Where(imagemodcache.URLEQ(imageURL)).
		Only(ctx)
	finishCache()
	if err == nil && existing != nil {
		return existing.Result == string(IMSSuggestionPass)
	}

	suggestion, err := s.ImageCensorAPI.ImageModerationURL(ctx, imageURL)
	if err != nil {
		s.Logger.ErrorContext(ctx, "image moderation request failed", "error_type", fmt.Sprintf("%T", err))
		return true
	}
	if suggestion != IMSSuggestionPass {
		s.Logger.Debug("image moderation rejected", "suggestion", suggestion)
	}

	// Insert into cache
	create := s.Client.ImageModCache.Create().
		SetURL(imageURL).
		SetResult(string(suggestion)).
		SetCreatedAt(time.Now())
	if harukiUserID > 0 {
		create = create.SetHarukiUserID(harukiUserID)
	}
	finishStore := commandtrace.MeasureOperation(ctx, censorStoreStage)
	_, err = create.Save(ctx)
	finishStore()
	if err != nil {
		s.Logger.ErrorContext(ctx, "image moderation cache store failed", "error_type", fmt.Sprintf("%T", err))
	}

	return suggestion == IMSSuggestionPass
}

func censorContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func textCensor(ctx context.Context, moderator TextModerator, text string) (map[string]any, error) {
	if contextual, ok := moderator.(contextTextModerator); ok {
		return contextual.TextCensorContext(ctx, text)
	}
	return moderator.TextCensor(text)
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
