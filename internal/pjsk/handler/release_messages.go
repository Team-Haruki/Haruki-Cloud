package handler

import (
	"errors"

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
	return err
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
