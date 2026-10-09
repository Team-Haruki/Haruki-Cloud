package handler

import (
	"context"
	"errors"

	"haruki-cloud/internal/core/upstreamerr"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/utils/usererror"
)

// upstreamUserError turns err into a typed user error for a command whose
// upstream is service. A classified upstream failure keeps its own service;
// a bare timeout or network failure (no client tagged it) is attributed to
// service. Other errors pass through unchanged.
func upstreamUserError(err error, service upstreamerr.Service) error {
	if err == nil || isUserFacingError(err) {
		return err
	}
	if typed := upstreamerr.UserError(err); typed != nil {
		return typed
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return usererror.Timeout(upstreamerr.Feature(service), err)
	}
	if upstreamerr.IsNetworkFailure(err) {
		return usererror.Unavailable(upstreamerr.Feature(service), err)
	}
	return err
}

// normalizeTrackerUserFacingError is the reply for a ranking (tracker)
// failure.
func normalizeTrackerUserFacingError(err error) error {
	return upstreamUserError(err, upstreamerr.ServiceRanking)
}

// normalizeDeckServiceUserFacingError is the reply for a deck-service
// failure.
func normalizeDeckServiceUserFacingError(err error) error {
	return upstreamUserError(err, upstreamerr.ServiceDeck)
}

// normalizeSKPlayerTraceDrawingError names the player trace when the
// renderer finds too few trace points to draw.
func normalizeSKPlayerTraceDrawingError(err error) error {
	if err == nil || isUserFacingError(err) {
		return err
	}
	if isDrawingDataInsufficientError(err) {
		return usererror.Wrap(usererror.CodeNotFound, i18n.M("sk.player_trace.data_insufficient"), err)
	}
	return err
}

func isDrawingDataInsufficientError(err error) bool {
	return upstreamerr.Is(err, upstreamerr.KindDataInsufficient)
}
