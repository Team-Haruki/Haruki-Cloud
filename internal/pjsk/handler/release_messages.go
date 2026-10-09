package handler

import (
	"haruki-cloud/internal/pjsk/notfound"
)

// The lookup normalizers below finish the typed errors of the card, song,
// event and gacha lookups for the command's region: a plain "not found"
// becomes the form naming the region and suggesting a region prefix. Not
// released content (releasecheck.UnreleasedError) and the other typed errors
// already carry their reply; upstream failures are classified.

func normalizeCardUserFacingErrorForLookup(err error, region string, fallbackQuery string) error {
	return normalizeLookupError(err, region, fallbackQuery)
}

func normalizeMusicUserFacingErrorForLookup(err error, region string, fallbackQuery string) error {
	return normalizeLookupError(err, region, fallbackQuery)
}

func normalizeEventUserFacingErrorForRegion(err error, region string) error {
	return normalizeLookupError(err, region, "")
}

func normalizeGachaUserFacingError(err error) error {
	return normalizeLookupError(err, DefaultRegionStr, "")
}

func normalizeLookupError(err error, region, fallbackQuery string) error {
	if err == nil {
		return nil
	}
	if isUserFacingError(err) {
		return notfound.InRegion(err, regionWithDefault(region), fallbackQuery)
	}
	return WrapDomainError(err)
}
