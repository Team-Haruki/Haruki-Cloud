package provider

import (
	"os"
	"path/filepath"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestLocalMasterdataCandidateDirsPreferMatchingRegionRepoOverBrokenRegionDir(t *testing.T) {
	root := t.TempDir()

	brokenJPDir := filepath.Join(root, "jp")
	if err := os.MkdirAll(brokenJPDir, 0o755); err != nil {
		t.Fatalf("mkdir broken jp dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brokenJPDir, "resourceBoxDetails.json"), []byte(`"jp"`), 0o644); err != nil {
		t.Fatalf("write jp details: %v", err)
	}

	cnRepoDir := filepath.Join(root, "haruki-sekai-sc-master", "master")
	if err := os.MkdirAll(cnRepoDir, 0o755); err != nil {
		t.Fatalf("mkdir cn repo dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cnRepoDir, "resourceBoxDetails.json"), []byte(`"cn"`), 0o644); err != nil {
		t.Fatalf("write cn details: %v", err)
	}

	store := newLocalStore(localMasterdataCandidateDirs(brokenJPDir, renderregion.CN)...)
	data, err := store.readFile("resourceBoxDetails.json")
	if err != nil {
		t.Fatalf("read resourceBoxDetails.json: %v", err)
	}
	if got := string(data); got != `"cn"` {
		t.Fatalf("expected CN repo data to win, got %s", got)
	}
}

func TestDatabaseProviderSetLocalMasterdataDirGatesEventLeaks(t *testing.T) {
	root := t.TempDir()
	jpDir := filepath.Join(root, "jp")
	if err := os.MkdirAll(jpDir, 0o755); err != nil {
		t.Fatalf("mkdir jp dir: %v", err)
	}

	provider := &DatabaseProvider{
		region:    renderregion.JP,
		education: &dbEducationProvider{},
		musics:    &dbMusicProvider{},
		events:    &dbEventProvider{},
		mysekai:   &dbMySekaiProvider{},
	}

	provider.SetLocalMasterdataDir(root, false)
	if provider.education.store == nil {
		t.Fatalf("expected education local store to be configured")
	}
	if provider.mysekai.local == nil {
		t.Fatalf("expected mysekai local fallback to be configured")
	}
	if provider.musics.local == nil {
		t.Fatalf("expected music local fallback to be configured")
	}
	if provider.events.local != nil {
		t.Fatalf("expected event leak fallback to stay disabled")
	}

	provider.SetLocalMasterdataDir(root, true)
	if provider.events.local == nil {
		t.Fatalf("expected event leak fallback to be enabled")
	}
}
