package upstreamerr

import (
	"net/http"
	"strings"
)

// Structured error codes: the "code" field of an error body. A code is
// matched before the message and the status, so a service that sends one is
// classified without reading its wording. Pinned by TestUpstreamCodeContract.
const (
	// Drawing: "code" of a failed render ({"detail": ..., "code": ...}).
	RenderCodeAssetMissing     = "asset_missing"
	RenderCodeAssetBroken      = "asset_broken"
	RenderCodeDataInsufficient = "data_insufficient"
	RenderCodeContentTooLarge  = "content_too_large"
)

var codeRules = map[Service]map[string]Kind{
	ServiceRender: {
		RenderCodeAssetMissing:     KindAssetMissing,
		RenderCodeAssetBroken:      KindAssetBroken,
		RenderCodeDataInsufficient: KindDataInsufficient,
		RenderCodeContentTooLarge:  KindContentTooLarge,
	},
}

// MatchCode maps a service's structured error code to a Kind (KindUnknown
// for an empty or unknown code).
func MatchCode(service Service, code string) Kind {
	if kind, ok := codeRules[service][strings.TrimSpace(code)]; ok {
		return kind
	}
	return KindUnknown
}

// The upstream error vocabulary Cloud depends on. These strings are written
// by other repositories (Toolbox, SekaiAPI, the tracker, the deck service,
// Drawing); changing them there silently changes the reply a user gets, so
// every one of them is pinned by TestUpstreamContract. Prefer status codes
// and structured codes: a message is only matched where neither decides.
// The Drawing messages are the fallback for a Drawing that predates "code".
const (
	// Toolbox: "message" field of 403 / 404 / 401 bodies.
	ToolboxMessageInvalidPlatform  = "invalid platform or platform_user_id"
	ToolboxMessageOwnerBanned      = "account owner is banned"
	ToolboxMessageBindingNotFound  = "account binding not found"
	ToolboxMessageGameDataNotFound = "game data not found"
	ToolboxMessageServiceDown      = "toolbox service unavailable"
	ToolboxMessageMissingToken     = "missing token"
	ToolboxMessageInvalidToken     = "invalid token"

	// SekaiAPI: "message" field of non-2xx bodies.
	GameDataMessageMissingToken  = "missing token"
	GameDataMessageInvalidToken  = "invalid token"
	GameDataMessageNotAuthorized = "not authorized for this server"
	GameDataMessageInvalidAPI    = "invalid api type"
	GameDataMessageInternal      = "internal server error"
	GameDataMessageUpstreamDown  = "upstream unavailable"

	// Tracker: "message" field of non-2xx bodies.
	RankingMessageRateLimited    = "rate limited by tracker"
	RankingMessageNoHeartbeat    = "no heartbeat found"
	RankingMessageInvalidServer  = "invalid server"
	RankingMessageRecordNotFound = "ranking record not found"
	RankingMessageNotFound       = "not found"

	// Deck service: "error" field of non-2xx bodies.
	DeckMessageFixedConflict      = "fixed_characters and fixed_cards cannot be used together"
	DeckMessageEventNotFound      = "event not found for eventid:"
	DeckMessageMasterDataNotFound = "master data not found"
	DeckMessageMusicMetasNotFound = "music metas not found"
	DeckMessageMusicMetaNotFound  = "music meta not found"
	DeckMessageUserdataHashNeeded = "userdata_hash is required"
	DeckMessageUserdataHashGone   = "user data not found for userdata_hash"
	DeckMessageUnsupportedMedia   = "unsupported media type"
	DeckMessageUnsupportedContent = "unsupported content type"
	DeckMessageInvalidContentType = "invalid content type"
	DeckMessageInvalidPayload     = "invalid recommend payload"
	DeckMessageNeedsBatchOptions  = "requires batch_options"
	DeckMessageNoUserData         = "no user data bytes available"
	DeckMessageUserDataRequired   = "user data is required"

	// Drawing: any part of a non-2xx body saying the data is too thin to draw.
	RenderMessageDataInsufficient   = "data insufficient"
	RenderMessageInsufficientData   = "insufficient data"
	RenderMessageNotEnoughData      = "not enough data"
	RenderMessageDataInsufficientZh = "数据不足" //copylint:ignore Drawing error message (upstream contract), never shown
	RenderMessageIndexerOutOfBounds = "single positional indexer is out-of-bounds"
	RenderMessageOutOfBoundsDash    = "out-of-bounds"
	RenderMessageOutOfBounds        = "out of bounds"
	RenderMessageIndexOutOfRange    = "index out of range"

	// Drawing: "detail" field of 4xx bodies.
	RenderMessageContentTooLarge = "content size is too large"
	RenderMessageCanvasTooLarge  = "canvas size is too large"
	RenderMessageTargetMissing   = "target file not found"
	RenderMessageFileMissing     = "file not found"
	RenderMessageNoSuchFile      = "no such file or directory"
	RenderMessageAssetPathEmpty  = "asset path is empty"
	RenderMessageImageMissingZh  = "图片文件不存在" //copylint:ignore Drawing error message (upstream contract), never shown
	RenderMessageCannotIdentify  = "cannot identify image file"
	RenderMessageReadImage       = "failed to read image"
	RenderMessageOpenImage       = "failed to open image"
	RenderMessageDownload        = "download"
	RenderMessageFailed          = "failed"
)

type messageRule struct {
	contains []string // every fragment must appear (case-insensitive)
	kind     Kind
}

var messageRules = map[Service][]messageRule{
	ServiceToolbox: {
		{[]string{ToolboxMessageInvalidPlatform}, KindAccessDenied},
		{[]string{ToolboxMessageOwnerBanned}, KindOwnerBanned},
		{[]string{ToolboxMessageBindingNotFound}, KindAccountNotBound},
		{[]string{ToolboxMessageGameDataNotFound}, KindDataNotUploaded},
		{[]string{ToolboxMessageServiceDown}, KindUnavailable},
		{[]string{ToolboxMessageMissingToken}, KindAuth},
		{[]string{ToolboxMessageInvalidToken}, KindAuth},
	},
	ServiceGameData: {
		{[]string{GameDataMessageMissingToken}, KindAuth},
		{[]string{GameDataMessageInvalidToken}, KindAuth},
		{[]string{GameDataMessageNotAuthorized}, KindAuth},
		{[]string{GameDataMessageInvalidAPI}, KindRejected},
		{[]string{GameDataMessageInternal}, KindBadResponse},
		{[]string{GameDataMessageUpstreamDown}, KindUnavailable},
	},
	ServiceRanking: {
		{[]string{RankingMessageRateLimited}, KindRateLimited},
		{[]string{RankingMessageNoHeartbeat}, KindNoRankingData},
		{[]string{RankingMessageInvalidServer}, KindRegionUnsupported},
		{[]string{RankingMessageRecordNotFound}, KindRankingNotFound},
		{[]string{RankingMessageNotFound}, KindNotFound},
	},
	ServiceDeck: {
		{[]string{DeckMessageFixedConflict}, KindIncompatible},
		{[]string{DeckMessageEventNotFound}, KindDataNotSynced},
		{[]string{DeckMessageMasterDataNotFound}, KindDataNotSynced},
		{[]string{DeckMessageMusicMetasNotFound}, KindDataNotSynced},
		{[]string{DeckMessageMusicMetaNotFound}, KindDataNotSynced},
		{[]string{DeckMessageUserdataHashNeeded}, KindCacheExpired},
		{[]string{DeckMessageUserdataHashGone}, KindCacheExpired},
		{[]string{DeckMessageUnsupportedMedia}, KindIncompatible},
		{[]string{DeckMessageUnsupportedContent}, KindIncompatible},
		{[]string{DeckMessageInvalidContentType}, KindIncompatible},
		{[]string{DeckMessageInvalidPayload}, KindRejected},
		{[]string{DeckMessageNeedsBatchOptions}, KindRejected},
		{[]string{DeckMessageNoUserData}, KindUserDataInvalid},
		{[]string{DeckMessageUserDataRequired}, KindUserDataInvalid},
	},
	ServiceRender: {
		{[]string{RenderMessageDataInsufficient}, KindDataInsufficient},
		{[]string{RenderMessageInsufficientData}, KindDataInsufficient},
		{[]string{RenderMessageNotEnoughData}, KindDataInsufficient},
		{[]string{RenderMessageDataInsufficientZh}, KindDataInsufficient},
		{[]string{RenderMessageIndexerOutOfBounds}, KindDataInsufficient},
		{[]string{RenderMessageOutOfBoundsDash}, KindDataInsufficient},
		{[]string{RenderMessageOutOfBounds}, KindDataInsufficient},
		{[]string{RenderMessageIndexOutOfRange}, KindDataInsufficient},
		{[]string{RenderMessageContentTooLarge}, KindContentTooLarge},
		{[]string{RenderMessageCanvasTooLarge}, KindContentTooLarge},
		{[]string{RenderMessageTargetMissing}, KindAssetMissing},
		{[]string{RenderMessageFileMissing}, KindAssetMissing},
		{[]string{RenderMessageNoSuchFile}, KindAssetMissing},
		{[]string{RenderMessageAssetPathEmpty}, KindAssetMissing},
		{[]string{RenderMessageImageMissingZh}, KindAssetMissing},
		{[]string{RenderMessageCannotIdentify}, KindAssetBroken},
		{[]string{RenderMessageReadImage}, KindAssetBroken},
		{[]string{RenderMessageOpenImage}, KindAssetBroken},
		{[]string{RenderMessageDownload, RenderMessageFailed}, KindAssetDownload},
	},
}

// classifyMessage maps a service's status and message to a Kind: first the
// message rules of the service, then the status.
func classifyMessage(service Service, status int, message string) Kind {
	if kind := MatchMessage(service, message); kind != KindUnknown {
		return kind
	}
	return classifyStatus(service, status)
}

// MatchMessage applies only the message rules of service. Clients use it to
// pick their typed sentinels so the vocabulary lives in one place.
func MatchMessage(service Service, message string) Kind {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return KindUnknown
	}
	for _, rule := range messageRules[service] {
		matched := true
		for _, fragment := range rule.contains {
			if !strings.Contains(lower, strings.ToLower(fragment)) {
				matched = false
				break
			}
		}
		if matched {
			return rule.kind
		}
	}
	return KindUnknown
}

func classifyStatus(service Service, status int) Kind {
	switch {
	case status == 0:
		return KindUnknown
	case status == http.StatusUnauthorized:
		return KindAuth
	case status == http.StatusForbidden:
		switch service {
		case ServiceToolbox:
			return KindAccessDenied
		default:
			return KindAuth
		}
	case status == http.StatusNotFound:
		switch service {
		case ServiceToolbox:
			return KindDataNotUploaded
		case ServiceGameData:
			return KindPlayerNotFound
		case ServiceRanking:
			return KindRankingNotFound
		case ServiceRender:
			// An unknown route: the renderer does not have this view yet.
			return KindIncompatible
		default:
			return KindRejected
		}
	case status == http.StatusUnsupportedMediaType:
		return KindIncompatible
	case status == http.StatusTooManyRequests:
		return KindRateLimited
	case status == http.StatusServiceUnavailable && (service == ServiceGameData || service == ServiceRanking):
		return KindMaintenance
	case status == http.StatusBadGateway, status == http.StatusServiceUnavailable, status == http.StatusGatewayTimeout:
		return KindUnavailable
	case status >= http.StatusInternalServerError:
		return KindBadResponse
	case status >= http.StatusBadRequest:
		return KindRejected
	default:
		return KindBadResponse
	}
}
