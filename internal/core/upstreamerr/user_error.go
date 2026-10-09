package upstreamerr

import (
	"haruki-cloud/internal/i18n"
	"haruki-cloud/utils/usererror"
)

// Feature is the user-facing name of service.
func Feature(service Service) i18n.Message {
	switch service {
	case ServiceToolbox:
		return i18n.FeatureToolbox
	case ServiceRanking:
		return i18n.FeatureRanking
	case ServiceDeck:
		return i18n.FeatureDeck
	case ServiceRender:
		return i18n.FeatureRender
	default:
		return i18n.FeatureGameData
	}
}

// userErrorFor is the generic reply for a classified failure. Callers that
// know more (which account, which data) build their own reply for the
// account-related kinds and fall back to this.
func userErrorFor(class Class, cause error) *usererror.Error {
	feature := Feature(class.Service)
	switch class.Kind {
	case KindNotConfigured, KindAuth, KindIncompatible:
		return usererror.Misconfigured(cause)
	case KindTimeout:
		return usererror.Timeout(feature, cause)
	case KindRateLimited:
		return usererror.Wrap(usererror.CodeUnavailable, i18n.M("upstream.rate_limited", i18n.Data{"Feature": feature}), cause)
	case KindMaintenance:
		return usererror.Wrap(usererror.CodeUnavailable, i18n.M("upstream.maintenance"), cause)
	case KindBadResponse, KindRejected:
		return usererror.Wrap(usererror.CodeUnavailable, i18n.M("upstream.failed", i18n.Data{"Feature": feature}), cause)

	case KindAccountNotBound:
		return usererror.Wrap(usererror.CodeSetup, i18n.M("upstream.toolbox.account_not_bound"), cause)
	case KindDataNotUploaded:
		return usererror.Wrap(usererror.CodeSetup, i18n.M("upstream.toolbox.data_not_uploaded"), cause)
	case KindAccessDenied:
		return usererror.Wrap(usererror.CodeSetup, i18n.M("upstream.toolbox.access_denied"), cause)
	case KindOwnerBanned:
		return usererror.Wrap(usererror.CodeForbidden, i18n.M("upstream.toolbox.owner_banned"), cause)

	case KindPlayerNotFound:
		return usererror.Wrap(usererror.CodeNotFound, i18n.M("upstream.game_data.player_not_found"), cause)

	case KindRankingNotFound:
		return usererror.Wrap(usererror.CodeNotFound, i18n.M("upstream.ranking.not_found"), cause)
	case KindNoRankingData:
		return usererror.Wrap(usererror.CodeUnavailable, i18n.M("upstream.ranking.no_data"), cause)
	case KindRegionUnsupported:
		return usererror.Wrap(usererror.CodeUnavailable, i18n.M("upstream.ranking.region_unsupported"), cause)

	case KindDataInsufficient:
		return usererror.Wrap(usererror.CodeNotFound, i18n.M("upstream.render.data_insufficient"), cause)
	case KindContentTooLarge:
		return usererror.Wrap(usererror.CodeInput, i18n.M("upstream.render.too_large"), cause)
	case KindAssetMissing:
		return usererror.Wrap(usererror.CodeUnavailable, i18n.M("upstream.render.asset_missing"), cause)
	case KindAssetBroken:
		return usererror.Wrap(usererror.CodeUnavailable, i18n.M("upstream.render.asset_broken"), cause)
	case KindAssetDownload:
		return usererror.Wrap(usererror.CodeUnavailable, i18n.M("upstream.render.asset_download"), cause)

	case KindDataNotSynced:
		return usererror.Wrap(usererror.CodeUnavailable, i18n.M("upstream.deck.data_not_synced"), cause)
	case KindCacheExpired:
		return usererror.Wrap(usererror.CodeUnavailable, i18n.M("upstream.deck.cache_expired"), cause)
	case KindUserDataInvalid:
		return usererror.Wrap(usererror.CodeSetup, i18n.M("upstream.deck.user_data_invalid"), cause)
	case KindEmptyResult:
		return usererror.Wrap(usererror.CodeUnavailable, i18n.M("upstream.deck.empty_result"), cause)

	case KindNotFound:
		if class.Service == ServiceRanking {
			return usererror.Wrap(usererror.CodeNotFound, i18n.M("upstream.ranking.no_match"), cause)
		}
		return usererror.Wrap(usererror.CodeUnavailable, i18n.M("upstream.failed", i18n.Data{"Feature": feature}), cause)
	default:
		return usererror.Unavailable(feature, cause)
	}
}
