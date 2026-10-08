package server

import (
	"testing"

	"haruki-cloud/internal/storage"
)

func TestImageCacheBucketOnlyForS3Slots(t *testing.T) {
	if got := imageCacheBucket(storage.ProviderConfig{Scheme: "s3", Bucket: " image-cache ", Endpoint: "http://garage.invalid:3900"}); got != "image-cache" {
		t.Fatalf("s3 bucket = %q", got)
	}
	if got := imageCacheBucket(storage.ProviderConfig{Scheme: "fs", Root: "/srv/ic"}); got != "" {
		t.Fatalf("fs bucket = %q", got)
	}
	if got := imageCacheBucket(storage.ProviderConfig{Scheme: "ftp"}); got != "" {
		t.Fatalf("invalid scheme bucket = %q", got)
	}
}
