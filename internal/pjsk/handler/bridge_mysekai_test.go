package handler

import (
	"context"
	"errors"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	renderuserdata "haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"
)

type unavailableSnapshotProvider struct{}

func (unavailableSnapshotProvider) Resolve(context.Context, renderuserdata.Selector, renderuserdata.ResolveOptions) (renderuserdata.Snapshot, error) {
	return nil, renderuserdata.ErrSnapshotUnavailable
}

type fixedMySekaiPayloadProvider struct {
	payload []byte
}

func (p fixedMySekaiPayloadProvider) Resolve(context.Context, renderuserdata.Selector, bool) ([]byte, error) {
	if len(p.payload) == 0 {
		return nil, errors.New("payload is unavailable")
	}
	return append([]byte(nil), p.payload...), nil
}

func TestResolveMySekaiRenderContextFallsBackToPayloadProvider(t *testing.T) {
	ctx := context.Background()
	service := newBridgeTestBindingService(t)

	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	controller := rendermysekai.NewController(nil, nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{AllowFallback: true})
	app := &renderapp.App{
		Bindings:        service,
		MySekai:         controller,
		Snapshots:       unavailableSnapshotProvider{},
		MySekaiPayloads: fixedMySekaiPayloadProvider{payload: []byte(`{"updatedResources":{"userMysekaiPhotos":[{"seq":1,"imagePath":"photos/test"}]}}`)},
	}

	result, err := resolveMySekaiRenderContext(ctx, app, userQueryParams{
		Mode:           "self",
		Platform:       "qq",
		PlatformUserID: "42",
	}, "jp", false)
	if err != nil {
		t.Fatalf("resolveMySekaiRenderContext() error = %v", err)
	}
	if result.Controller == nil {
		t.Fatal("expected controller")
	}

	photo, err := result.Controller.ResolvePhoto(rendermysekai.PhotoQuery{Region: "jp", Seq: 1})
	if err != nil {
		t.Fatalf("ResolvePhoto() error = %v", err)
	}
	if photo == nil || photo.ImagePath != "photos/test" {
		t.Fatalf("unexpected photo result: %+v", photo)
	}
}

func TestResolveMySekaiRenderContextPrefersSnapshotProfileCard(t *testing.T) {
	ctx := context.Background()
	service := newBridgeTestBindingService(t)

	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	controller := rendermysekai.NewController(nil, nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{AllowFallback: true})
	snapshot := &runtimeSnapshotStub{
		card: &drawing.ProfileCardRequest{
			Profile: &drawing.BasicProfile{Nickname: "snapshot-card"},
			DataSources: []drawing.ProfileDataSource{
				{Name: "Suite数据"},
			},
		},
	}
	app := &renderapp.App{
		Bindings:  service,
		MySekai:   controller,
		Snapshots: &runtimeSnapshotProviderStub{snapshot: snapshot},
	}

	result, err := resolveMySekaiRenderContext(ctx, app, userQueryParams{
		Mode:           "self",
		Platform:       "qq",
		PlatformUserID: "42",
	}, "jp", false)
	if err != nil {
		t.Fatalf("resolveMySekaiRenderContext() error = %v", err)
	}
	if result.Profile == nil || result.Profile.Profile == nil || result.Profile.Profile.Nickname != "snapshot-card" {
		t.Fatalf("expected snapshot profile card, got %+v", result.Profile)
	}
	if len(result.Profile.DataSources) == 0 || result.Profile.DataSources[0].Name != "Suite数据" {
		t.Fatalf("expected suite data source, got %+v", result.Profile.DataSources)
	}
}

func TestExecuteMySekaiRequiresController(t *testing.T) {
	_, err := executeMysekai(NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Module: parser.ModuleMysekai,
		Mode:   "mysekai-photo",
		Region: "jp",
	}, &renderapp.App{}))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "mysekai controller is not configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}
