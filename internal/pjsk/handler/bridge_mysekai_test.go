package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"haruki-cloud/api/bot/onebot11"
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

func TestExecuteConcurrentMessagesRunsJobsInParallelAndPreservesOrder(t *testing.T) {
	started := make(chan string, 2)
	finished := make(chan string, 2)
	release := make(chan struct{})

	job := func(name string, delay time.Duration) concurrentMessageJob {
		return func(context.Context) (onebot11.Message, error) {
			started <- name
			<-release
			time.Sleep(delay)
			finished <- name
			return onebot11.Message{onebot11.Text(name)}, nil
		}
	}

	resultCh := make(chan onebot11.Message, 1)
	errCh := make(chan error, 1)
	go func() {
		message, err := executeConcurrentMessages(
			context.Background(),
			job("resource", 80*time.Millisecond),
			job("map", 0),
		)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- message
	}()

	startedSet := make(map[string]bool, 2)
	timeout := time.After(2 * time.Second)
	for len(startedSet) < 2 {
		select {
		case name := <-started:
			startedSet[name] = true
		case <-timeout:
			t.Fatal("expected both concurrent jobs to start before release")
		}
	}

	close(release)

	select {
	case err := <-errCh:
		t.Fatalf("executeConcurrentMessages() error = %v", err)
	case message := <-resultCh:
		if got := messageTextOrder(message); len(got) != 2 || got[0] != "resource" || got[1] != "map" {
			t.Fatalf("unexpected message order: %v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for concurrent message execution")
	}

	firstFinished := <-finished
	if firstFinished != "map" {
		t.Fatalf("expected map to finish first, got %q", firstFinished)
	}
}

func TestExecuteConcurrentMessagesReturnsJobError(t *testing.T) {
	expectedErr := errors.New("boom")
	_, err := executeConcurrentMessages(
		context.Background(),
		func(context.Context) (onebot11.Message, error) {
			return nil, expectedErr
		},
	)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}

func messageTextOrder(message onebot11.Message) []string {
	var texts []string
	for _, segment := range message {
		text, ok := segment.Data.(onebot11.TextData)
		if !ok {
			texts = append(texts, fmt.Sprintf("non-text:%T", segment.Data))
			continue
		}
		texts = append(texts, text.Text)
	}
	return texts
}
