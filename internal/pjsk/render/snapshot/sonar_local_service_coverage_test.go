package snapshot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalFileServiceInitializationErrors(t *testing.T) {
	if service := NewLocalFileServiceWithContext(t.Context(), nil, nil, LocalFileConfig{}); service.Configured() {
		t.Fatal("blank local service is configured")
	}
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.json")
	service := NewLocalFileServiceWithContext(t.Context(), nil, nil, LocalFileConfig{UserJSON: missing})
	if service.initErr == nil || !strings.Contains(service.initErr.Error(), "user snapshot") {
		t.Fatalf("missing user snapshot error = %v", service.initErr)
	}
	userJSON := filepath.Join(dir, "user.json")
	if err := os.WriteFile(userJSON, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	service = NewLocalFileServiceWithContext(t.Context(), nil, nil, LocalFileConfig{UserJSON: userJSON, MySekaiJSON: missing})
	if service.initErr == nil || !strings.Contains(service.initErr.Error(), "mysekai snapshot") {
		t.Fatalf("missing MySekai snapshot error = %v", service.initErr)
	}
	service = NewLocalFileServiceWithContext(t.Context(), nil, nil, LocalFileConfig{UserJSON: userJSON, MusicMetaJSON: missing})
	if service.initErr == nil || !strings.Contains(service.initErr.Error(), "music meta snapshot") {
		t.Fatalf("missing music meta snapshot error = %v", service.initErr)
	}
}
