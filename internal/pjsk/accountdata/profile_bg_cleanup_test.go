package accountdata

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/profilebgcleanup"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

type controlledBackgroundStore struct {
	storage.Store
	put     func(context.Context, storage.Key) error
	remove  func(context.Context, storage.Key) error
	deletes atomic.Int32
}

func (s *controlledBackgroundStore) Put(ctx context.Context, key storage.Key, data []byte, opts storage.PutOptions) error {
	if s.put != nil {
		if err := s.put(ctx, key); err != nil {
			return err
		}
	}
	return s.Store.Put(ctx, key, data, opts)
}
func (s *controlledBackgroundStore) Delete(ctx context.Context, key storage.Key) error {
	s.deletes.Add(1)
	if s.remove != nil {
		if err := s.remove(ctx, key); err != nil {
			return err
		}
	}
	return s.Store.Delete(ctx, key)
}

func backgroundLedgerFixture(t *testing.T) (*BindingService, *pjskdb.Client, *pjskdb.UserBinding, *controlledBackgroundStore) {
	t.Helper()
	service, db := openAccountCoverageService(t, "background_ledger", accountCoverageValidator{profiles: map[string]string{"jp": "Fixture"}})
	t.Cleanup(func() { _ = db.Close() })
	if _, err := service.Bind(t.Context(), "qq", "42", "9901"); err != nil {
		t.Fatal(err)
	}
	binding, err := service.currentBindingEntity(t.Context(), "qq", "42", "jp")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UserBinding.UpdateOneID(binding.ID).SetVerified(true).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	binding.Verified = true
	store := &controlledBackgroundStore{Store: storagetest.NewMemory()}
	service.bgStorage = profileBGStoreWith(store, pngBytes(t, 3, 5))
	return service, db, binding, store
}
func makeCleanupDue(t *testing.T, db *pjskdb.Client) {
	t.Helper()
	if _, err := db.ProfileBGCleanup.Update().SetNotBefore(time.Now().UTC().Add(-time.Minute)).Save(t.Context()); err != nil {
		t.Fatal(err)
	}
}
func seedBackground(t *testing.T, db *pjskdb.Client, binding *pjskdb.UserBinding, store storage.Store, name string) *drawing.ProfileBgSettings {
	t.Helper()
	key := DefaultProfileBGRelativeDir + "/jp/" + name
	bg := &drawing.ProfileBgSettings{ImgPath: &key, Blur: 3, Alpha: 70}
	if err := db.GameAccount.UpdateOneID(bindingGameAccountID(binding)).SetBg(bg).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(t.Context(), storage.Key(key), []byte("fixture"), storage.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	return bg
}

func TestBackgroundUploadReservesBeforePutAndSurvivesCommitFailure(t *testing.T) {
	service, db, binding, store := backgroundLedgerFixture(t)
	store.put = func(ctx context.Context, key storage.Key) error {
		row, err := db.ProfileBGCleanup.Query().Where(profilebgcleanup.ObjectPathEQ(string(key))).Only(ctx)
		if err != nil {
			return err
		}
		if row.State != profilebgcleanup.StateUploading {
			return errors.New("object PUT lacked upload intent")
		}
		return nil
	}
	forced := errors.New("fixture commit failed")
	db.GameAccount.Use(func(next pjskdb.Mutator) pjskdb.Mutator {
		return pjskdb.MutateFunc(func(context.Context, pjskdb.Mutation) (pjskdb.Value, error) { return nil, forced })
	})
	if _, err := service.setBindingProfileBG(t.Context(), "qq", "42", binding, "https://example.test/bg.png"); !errors.Is(err, forced) {
		t.Fatalf("set=%v", err)
	}
	intent, err := db.ProfileBGCleanup.Query().Only(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(t.Context(), storage.Key(intent.ObjectPath)); err != nil {
		t.Fatal("upload should remain tracked after failed commit", err)
	}
	if _, err := service.CleanupProfileBackgrounds(t.Context(), 10); err != nil {
		t.Fatal(err)
	}
	if store.deletes.Load() != 0 {
		t.Fatal("cleaner removed an upload inside its grace period")
	}
	makeCleanupDue(t, db)
	restarted := NewBindingService(db, service.identity, service.validator)
	restarted.SetProfileBGStorage(service.bgStorage)
	if n, err := restarted.CleanupProfileBackgrounds(t.Context(), 10); err != nil || n != 1 {
		t.Fatalf("restart sweep=%d,%v", n, err)
	}
	if count, err := db.ProfileBGCleanup.Query().Count(t.Context()); err != nil || count != 0 {
		t.Fatalf("cleanup rows=%d,%v", count, err)
	}
	if _, err := store.Get(t.Context(), storage.Key(intent.ObjectPath)); !errors.Is(err, storage.ErrNotExist) {
		t.Fatalf("orphan still exists: %v", err)
	}
}

func TestBackgroundReservationFailureNeverPutsObject(t *testing.T) {
	service, db, binding, store := backgroundLedgerFixture(t)
	var puts atomic.Int32
	store.put = func(context.Context, storage.Key) error { puts.Add(1); return nil }
	forced := errors.New("fixture ledger unavailable")
	db.ProfileBGCleanup.Use(func(next pjskdb.Mutator) pjskdb.Mutator {
		return pjskdb.MutateFunc(func(context.Context, pjskdb.Mutation) (pjskdb.Value, error) { return nil, forced })
	})
	if _, err := service.setBindingProfileBG(t.Context(), "qq", "42", binding, "https://example.test/bg.png"); !errors.Is(err, forced) {
		t.Fatalf("set=%v", err)
	}
	if puts.Load() != 0 {
		t.Fatal("uploaded without a durable intent")
	}
}

func TestBackgroundClearCommitsBeforeDeletionAndRetriesDurably(t *testing.T) {
	service, db, binding, store := backgroundLedgerFixture(t)
	old := seedBackground(t, db, binding, store, "old.jpg")
	failed := errors.New("fixture delete unavailable")
	store.remove = func(context.Context, storage.Key) error { return failed }
	item, err := service.clearBindingProfileBG(t.Context(), "qq", "42", binding)
	if err != nil || item.Bg != nil {
		t.Fatalf("clear=%+v,%v", item, err)
	}
	row, err := db.ProfileBGCleanup.Query().Only(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if row.State != profilebgcleanup.StateDeleting || row.Attempts != 1 {
		t.Fatalf("debt not retained: %+v", row)
	}
	if _, err := store.Get(t.Context(), storage.Key(*old.ImgPath)); err != nil {
		t.Fatal(err)
	}
	store.remove = nil
	makeCleanupDue(t, db)
	restarted := NewBindingService(db, service.identity, service.validator)
	restarted.SetProfileBGStorage(service.bgStorage)
	if n, err := restarted.CleanupProfileBackgrounds(t.Context(), 10); err != nil || n != 1 {
		t.Fatalf("retry=%d,%v", n, err)
	}
}

func TestBackgroundFailedClearPreservesReferencedObject(t *testing.T) {
	service, db, binding, store := backgroundLedgerFixture(t)
	old := seedBackground(t, db, binding, store, "current.jpg")
	forced := errors.New("fixture DB failure")
	db.GameAccount.Use(func(next pjskdb.Mutator) pjskdb.Mutator {
		return pjskdb.MutateFunc(func(context.Context, pjskdb.Mutation) (pjskdb.Value, error) { return nil, forced })
	})
	if _, err := service.clearBindingProfileBG(t.Context(), "qq", "42", binding); !errors.Is(err, forced) {
		t.Fatalf("clear=%v", err)
	}
	if store.deletes.Load() != 0 {
		t.Fatal("deleted object before DB commit")
	}
	current, err := db.GameAccount.Get(t.Context(), bindingGameAccountID(binding))
	if err != nil {
		t.Fatal(err)
	}
	if !sameProfileBGPath(current.Bg, old) {
		t.Fatal("failed clear changed background")
	}
}

func TestConcurrentBackgroundUploadsUseDatabaseCAS(t *testing.T) {
	first, db, binding, store := backgroundLedgerFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var puts atomic.Int32
	store.put = func(ctx context.Context, _ storage.Key) error {
		if puts.Add(1) == 1 {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}
	firstResult := make(chan error, 1)
	go func() {
		_, err := first.setBindingProfileBG(t.Context(), "qq", "42", binding, "https://example.test/first.png")
		firstResult <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("controlled object operation did not begin")
	}
	second := NewBindingService(db, first.identity, first.validator)
	second.SetProfileBGStorage(first.bgStorage)
	winner, err := second.setBindingProfileBG(t.Context(), "qq", "42", binding, "https://example.test/second.png")
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	close(release)
	if err := <-firstResult; !errors.Is(err, errProfileBGChanged) {
		t.Fatalf("stale uploader=%v", err)
	}
	account, err := db.GameAccount.Get(t.Context(), bindingGameAccountID(binding))
	if err != nil {
		t.Fatal(err)
	}
	if !sameProfileBGPath(account.Bg, winner.Bg) || account.BgRevision != 1 {
		t.Fatal("stale upload overwrote the winner")
	}
	if _, err := store.Get(t.Context(), storage.Key(*winner.Bg.ImgPath)); err != nil {
		t.Fatal("winner object is absent", err)
	}
	if count, err := db.ProfileBGCleanup.Query().Count(t.Context()); err != nil || count != 1 {
		t.Fatalf("losing upload debt=%d,%v", count, err)
	}
}

func TestCleanupClaimPreventsLateReferenceAndDuplicateDelete(t *testing.T) {
	service, db, binding, store := backgroundLedgerFixture(t)
	accountID := bindingGameAccountID(binding)
	objectPath := DefaultProfileBGRelativeDir + "/jp/expired.jpg"
	settings := &drawing.ProfileBgSettings{ImgPath: &objectPath}
	intentID, err := service.reserveProfileBGUpload(t.Context(), accountID, settings)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(t.Context(), storage.Key(objectPath), []byte("fixture"), storage.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	makeCleanupDue(t, db)
	row, err := db.ProfileBGCleanup.Get(t.Context(), intentID)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	store.remove = func(ctx context.Context, _ storage.Key) error {
		close(entered)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	first := make(chan error, 1)
	go func() { _, err := service.cleanupProfileBG(t.Context(), row); first <- err }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("controlled object operation did not begin")
	}
	if done, err := service.cleanupProfileBG(t.Context(), row); err != nil || done {
		close(release)
		t.Fatalf("duplicate cleaner=%v,%v", done, err)
	}
	if _, err := service.commitProfileBackground(t.Context(), accountID, 0, nil, settings, intentID); !errors.Is(err, errProfileBGChanged) {
		close(release)
		t.Fatalf("late publication=%v", err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	account, err := db.GameAccount.Get(t.Context(), accountID)
	if err != nil {
		t.Fatal(err)
	}
	if account.Bg != nil || account.BgRevision != 0 {
		t.Fatal("failed publication escaped transaction rollback")
	}
	if store.deletes.Load() != 1 {
		t.Fatalf("deletes=%d", store.deletes.Load())
	}
}

func TestBackgroundCleanerCancellationAndReferencedGuard(t *testing.T) {
	service, db, binding, store := backgroundLedgerFixture(t)
	current := seedBackground(t, db, binding, store, "referenced.jpg")
	if _, err := db.ProfileBGCleanup.Create().SetGameAccountID(bindingGameAccountID(binding)).SetObjectPath(*current.ImgPath).SetState(profilebgcleanup.StatePending).SetNotBefore(time.Now().UTC().Add(-time.Minute)).Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := service.CleanupProfileBackgrounds(canceled, 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled sweep=%v", err)
	}
	if _, err := service.CleanupProfileBackgrounds(t.Context(), 10); err == nil {
		t.Fatal("expected live-reference guard")
	}
	if store.deletes.Load() != 0 {
		t.Fatal("deleted a referenced object")
	}
	stopped := make(chan struct{})
	go func() { service.RunProfileBGCleanup(canceled); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("worker ignored shutdown")
	}
}
