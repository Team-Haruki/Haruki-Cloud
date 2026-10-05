package sekai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"haruki-cloud/config"
)

const housingTestPath = "/image/mysekai-housing-competition/thumbnail/" +
	"8548d591ce5fccb90b184fa650e57289196556ae763d2a1f3fd2437dd8d78892/" +
	"f5f8c0e8-3e6a-42d2-afb1-b2f2451b48f0"

type housingRoundTripper func(*http.Request) (*http.Response, error)

func (f housingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHousingThumbnailRegions(t *testing.T) {
	for _, region := range []string{"jp", "en"} {
		t.Run(region, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/image/"+region+"/mysekai-housing/hash/uuid" || r.Header.Get(tokenHeader) != "internal-secret" {
					t.Errorf("unexpected upstream request: %s", r.URL.Path)
				}
				_, _ = w.Write([]byte("image"))
			}))
			defer upstream.Close()
			client := NewSekaiAPIClient(&config.SekaiAPIConfig{BaseURL: upstream.URL, Token: "internal-secret"})
			body, err := client.GetMySekaiHousingThumbnail(region, " /hash/uuid ")
			if err != nil || string(body) != "image" {
				t.Fatalf("body=%q err=%v", body, err)
			}
		})
	}
	for region, host := range map[string]string{
		"cn": "mk-prod-tos.tos-cn-shanghai.volces.com",
		"tw": "mkoversea-prod-bucket.s3.ap-northeast-1.amazonaws.com",
		"kr": "mkkorea-prod-bucket.s3.ap-northeast-1.amazonaws.com",
	} {
		t.Run(region, func(t *testing.T) {
			client := NewSekaiAPIClient(&config.SekaiAPIConfig{BaseURL: "http://unused.invalid", Token: "internal-secret"})
			client.imageHTTP.SetTransport(housingRoundTripper(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://"+host+housingTestPath || r.Header.Get(tokenHeader) != "" || r.Header.Get("Authorization") != "" {
					t.Error("incorrect object request or leaked credentials")
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("image")), Request: r}, nil
			}))
			body, err := client.GetMySekaiHousingThumbnail(region, "https://"+host+housingTestPath)
			if err != nil || string(body) != "image" {
				t.Fatalf("body=%q err=%v", body, err)
			}
		})
	}
}

func TestHousingThumbnailRejectsUntrustedURLs(t *testing.T) {
	client := NewSekaiAPIClient(nil)
	client.imageHTTP.SetTransport(housingRoundTripper(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid URL reached transport")
		return nil, nil
	}))
	for _, address := range []string{
		"https://127.0.0.1" + housingTestPath,
		"http://mk-prod-tos.tos-cn-shanghai.volces.com" + housingTestPath,
		"//mk-prod-tos.tos-cn-shanghai.volces.com" + housingTestPath,
		"https://mk-prod-tos.tos-cn-shanghai.volces.com.evil.invalid" + housingTestPath,
		"https://mk-prod-tos.tos-cn-shanghai.volces.com:443" + housingTestPath,
		"https://user@mk-prod-tos.tos-cn-shanghai.volces.com" + housingTestPath,
		"https://mk-prod-tos.tos-cn-shanghai.volces.com/other",
		"https://mk-prod-tos.tos-cn-shanghai.volces.com" + housingTestPath + "#fragment",
		"https://mkoversea-prod-bucket.s3.ap-northeast-1.amazonaws.com" + housingTestPath,
		"https://%xx",
	} {
		t.Run(address, func(t *testing.T) {
			if _, err := client.GetMySekaiHousingThumbnail("cn", address); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestHousingThumbnailErrorsAndCancellation(t *testing.T) {
	address := "https://mk-prod-tos.tos-cn-shanghai.volces.com" + housingTestPath
	for _, status := range []int{http.StatusNotFound, http.StatusFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := NewSekaiAPIClient(nil)
			calls := 0
			client.imageHTTP.SetRetryCount(0).SetTransport(housingRoundTripper(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"http://127.0.0.1/private"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
			}))
			if _, err := client.GetMySekaiHousingThumbnail("cn", address); err == nil {
				t.Fatal("expected error")
			}
			if calls != 1 {
				t.Fatalf("redirect followed: %d requests", calls)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	client := NewSekaiAPIClient(nil).WithContext(ctx)
	client.imageHTTP.SetRetryCount(0).SetTransport(housingRoundTripper(func(r *http.Request) (*http.Response, error) {
		cancel()
		<-r.Context().Done()
		return nil, r.Context().Err()
	}))
	if _, err := client.GetMySekaiHousingThumbnail("cn", address); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
