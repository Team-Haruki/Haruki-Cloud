package mysekai

import (
	"strings"
	"testing"

	"haruki-cloud/internal/i18n"
)

// TestFixtureTitleTagHoldsOnlyTheRegion: the renderer draws the 【…】 tag of
// the /msf title as a chip about 90 px wide and fails the whole image when
// it does not fit. A region label fits; a region label plus a fixture ID of
// three or more digits does not, so the ID follows the tag.
func TestFixtureTitleTagHoldsOnlyTheRegion(t *testing.T) {
	for _, code := range i18n.RegionCodes {
		title := i18n.T("mysekai.image.fixture.title", i18n.Data{"Region": i18n.RegionLabel(code), "ID": 12345, "Name": "Wood Chair"})
		tag, rest, ok := strings.Cut(strings.TrimPrefix(title, "【"), "】")
		if !strings.HasPrefix(title, "【") || !ok {
			t.Fatalf("%s: title %q has no 【…】 tag", code, title)
		}
		if tag != i18n.RegionLabel(code).String() {
			t.Errorf("%s: tag %q, want only the region label %q", code, tag, i18n.RegionLabel(code))
		}
		if !strings.Contains(rest, "12345") || !strings.Contains(rest, "Wood Chair") {
			t.Errorf("%s: title %q lost the fixture ID or name", code, title)
		}
	}
}
