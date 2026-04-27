package handler

import (
	"errors"
	"strings"

	"haruki-cloud/internal/onebot11"
	renderevent "haruki-cloud/internal/pjsk/render/event"
	"haruki-cloud/internal/pjsk/render/releasecheck"
)

func normalizeCardUserFacingError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[onebot11.ReplayError](err); ok {
		return err
	}
	var unreleased *releasecheck.UnreleasedError
	if errors.As(err, &unreleased) && unreleased.Kind == releasecheck.KindCard {
		return onebot11.NewReplayError("该卡牌还未上线")
	}
	return err
}

func normalizeMusicUserFacingError(err error) error {
	return normalizeMusicUserFacingErrorForLookup(err, DefaultRegionStr, "")
}

func normalizeMusicUserFacingErrorForLookup(err error, region string, fallbackQuery string) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[onebot11.ReplayError](err); ok {
		return err
	}
	var unreleased *releasecheck.UnreleasedError
	if errors.As(err, &unreleased) && unreleased.Kind == releasecheck.KindMusic {
		return onebot11.NewReplayError("该歌曲还未上线")
	}
	if query, ok := extractMusicNotFoundQuery(err, fallbackQuery); ok {
		return newMusicNotFoundReplayError(region, query)
	}
	return err
}

func newMusicNotFoundReplayError(region string, query string) error {
	return onebot11.NewReplayError(
		"%s服找不到特定的歌: %s\n如果需要查其他区服的歌曲请加区服前缀，如需要查日服的请加jp区服前缀，防止用户想查别的服的歌查到别的服去了",
		musicNotFoundRegionLabel(region),
		musicNotFoundQueryOrFallback(query, ""),
	)
}

func musicNotFoundRegionLabel(region string) string {
	region = strings.ToLower(strings.TrimSpace(regionWithDefault(region)))
	if region == "" {
		return DefaultRegionStr
	}
	return region
}

func extractMusicNotFoundQuery(err error, fallbackQuery string) (string, bool) {
	if err == nil {
		return "", false
	}
	message := strings.TrimSpace(err.Error())
	fallbackQuery = strings.TrimSpace(fallbackQuery)

	const musicNotFoundPrefix = "music not found"
	if message == musicNotFoundPrefix {
		return musicNotFoundQueryOrFallback("", fallbackQuery), true
	}
	if idx := strings.LastIndex(message, musicNotFoundPrefix+":"); idx >= 0 {
		raw := strings.TrimSpace(message[idx+len(musicNotFoundPrefix)+1:])
		return musicNotFoundQueryOrFallback(raw, fallbackQuery), true
	}

	if strings.HasPrefix(message, "music ") && strings.HasSuffix(message, " not found") {
		raw := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(message, "music "), " not found"))
		return musicNotFoundQueryOrFallback(raw, fallbackQuery), true
	}
	return "", false
}

func musicNotFoundQueryOrFallback(query string, fallback string) string {
	query = strings.TrimSpace(query)
	if query == "" || strings.EqualFold(query, "empty query") {
		query = strings.TrimSpace(fallback)
	}
	if query == "" {
		return "未知"
	}
	return query
}

func normalizeEventUserFacingError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[onebot11.ReplayError](err); ok {
		return err
	}
	var unreleased *releasecheck.UnreleasedError
	if errors.As(err, &unreleased) && unreleased.Kind == releasecheck.KindEvent {
		return onebot11.NewReplayError("该活动还未上线")
	}
	if errors.Is(err, renderevent.ErrNoOngoingEvent) {
		return onebot11.NewReplayError("当前无正在举办的活动")
	}
	return err
}

func normalizeGachaUserFacingError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[onebot11.ReplayError](err); ok {
		return err
	}
	var unreleased *releasecheck.UnreleasedError
	if errors.As(err, &unreleased) && unreleased.Kind == releasecheck.KindGacha {
		return onebot11.NewReplayError("该卡池还未上线")
	}
	return err
}
