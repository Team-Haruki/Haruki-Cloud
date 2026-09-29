package profile

import (
	"testing"

	"haruki-cloud/internal/pjsk/sekai"
)

func TestAdaptAPIProfileHonorsCarriesHonorLayers(t *testing.T) {
	got := adaptAPIProfileHonors([]sekai.UserProfileHonor{
		{Seq: 1, HonorID: 4, HonorLevel: 14, HonorBackgroundID: new(10102), HonorWordID: new(10101)},
		{Seq: 2, HonorID: 5, HonorLevel: 1},
	})
	if len(got) != 2 || got[0].HonorBackgroundID == nil || *got[0].HonorBackgroundID != 10102 || got[0].HonorWordID == nil || *got[0].HonorWordID != 10101 {
		t.Fatalf("customized profile honor = %+v", got)
	}
	if got[1].HonorBackgroundID != nil || got[1].HonorWordID != nil {
		t.Fatalf("pre-7.0.0 profile honor must stay uncustomized: %+v", got[1])
	}
}
