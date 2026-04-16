package misc

import (
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
)

func TestBuildCharaBirthdayRequestRejectsMissingCards(t *testing.T) {
	controller := NewController(nil)
	_, err := controller.BuildCharaBirthdayRequest(drawing.CharaBirthdayRequest{
		Cid:   1,
		Month: 8,
		Day:   31,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestBuildCharaBirthdayRequestPassesValidPayload(t *testing.T) {
	controller := NewController(nil)
	req, err := controller.BuildCharaBirthdayRequest(drawing.CharaBirthdayRequest{
		Cid:   1,
		Month: 8,
		Day:   31,
		Cards: []drawing.CharaBirthdayCard{
			{ID: 1001, ThumbnailPath: "thumbnail.png"},
		},
	})
	if err != nil {
		t.Fatalf("BuildCharaBirthdayRequest failed: %v", err)
	}
	if req.Cid != 1 || len(req.Cards) != 1 {
		t.Fatalf("unexpected request: %+v", req)
	}
}
