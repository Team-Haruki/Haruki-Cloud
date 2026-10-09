package accountdata

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/gameaccount"
	"haruki-cloud/database/pjsk/profilebgcleanup"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/storage"
	"haruki-cloud/utils/logger"
	"haruki-cloud/utils/usererror"
)

const (
	profileBGMutationTimeout = 2 * time.Minute
	profileBGUploadGrace     = 10 * time.Minute
	profileBGCleanupLease    = time.Minute
	profileBGDeleteTimeout   = 10 * time.Second
)

var errProfileBGChanged = usererror.New(usererror.CodeUnavailable, i18n.M("profile.bg.changed"))

func loadProfileBackgroundRevision(ctx context.Context, db *pjskdb.Client, accountID int) (*drawing.ProfileBgSettings, int64, error) {
	if db == nil || accountID <= 0 {
		return nil, 0, usererror.Misconfigured(errors.New("profile background account is not configured"))
	}
	account, err := db.GameAccount.Get(ctx, accountID)
	if err != nil {
		return nil, 0, err
	}
	return cloneProfileBGSettings(account.Bg), account.BgRevision, nil
}

func (s *BindingService) reserveProfileBGUpload(ctx context.Context, accountID int, settings *drawing.ProfileBgSettings) (int, error) {
	if !hasCustomProfileBGImage(settings) {
		return 0, fmt.Errorf("profile background upload returned no object")
	}
	if _, err := profileBGCleanupKey(*settings.ImgPath); err != nil {
		return 0, err
	}
	row, err := s.pjskDB.ProfileBGCleanup.Create().SetGameAccountID(accountID).SetObjectPath(*settings.ImgPath).
		SetState(profilebgcleanup.StateUploading).SetNotBefore(time.Now().UTC().Add(profileBGUploadGrace)).Save(ctx)
	if err != nil {
		return 0, err
	}
	return row.ID, nil
}

// The account CAS and cleanup ledger commit together. Expired upload intents
// claimed by a cleaner can no longer publish a reference to their objects.
func (s *BindingService) commitProfileBackground(ctx context.Context, accountID int, revision int64, oldBG, nextBG *drawing.ProfileBgSettings, uploadID int) (int, error) {
	tx, err := s.pjskDB.Tx(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	update := tx.GameAccount.Update().Where(gameaccount.IDEQ(accountID), gameaccount.BgRevisionEQ(revision)).AddBgRevision(1)
	if nextBG == nil {
		update.ClearBg()
	} else {
		update.SetBg(cloneProfileBGSettings(nextBG))
	}
	changed, err := update.Save(ctx)
	if err != nil {
		return 0, err
	}
	if changed != 1 {
		return 0, errProfileBGChanged
	}
	if uploadID != 0 {
		if !hasCustomProfileBGImage(nextBG) {
			return 0, errors.New("profile background upload has no object")
		}
		removed, err := tx.ProfileBGCleanup.Delete().Where(profilebgcleanup.IDEQ(uploadID), profilebgcleanup.GameAccountIDEQ(accountID),
			profilebgcleanup.ObjectPathEQ(*nextBG.ImgPath), profilebgcleanup.StateEQ(profilebgcleanup.StateUploading), profilebgcleanup.NotBeforeGT(time.Now().UTC())).Exec(ctx)
		if err != nil {
			return 0, err
		}
		if removed != 1 {
			return 0, errProfileBGChanged
		}
	}
	cleanupID := 0
	if hasCustomProfileBGImage(oldBG) && !sameProfileBGPath(oldBG, nextBG) {
		row, err := tx.ProfileBGCleanup.Create().SetGameAccountID(accountID).SetObjectPath(*oldBG.ImgPath).
			SetState(profilebgcleanup.StatePending).SetNotBefore(time.Now().UTC()).Save(ctx)
		if err != nil {
			return 0, err
		}
		cleanupID = row.ID
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return cleanupID, nil
}

func profileBGCleanupKey(objectPath string) (storage.Key, error) {
	key, err := profileBGKey(objectPath)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(string(key), DefaultProfileBGRelativeDir+"/") {
		return "", errors.New("profile background cleanup path is outside the managed prefix")
	}
	return key, nil
}

func (s *BindingService) tryProfileBGCleanup(ctx context.Context, id int) {
	if id == 0 || s.bgStorage == nil {
		return
	}
	row, err := s.pjskDB.ProfileBGCleanup.Get(ctx, id)
	if err == nil {
		_, err = s.cleanupProfileBG(ctx, row)
	}
	if err != nil && !pjskdb.IsNotFound(err) {
		logger.WarnContext(ctx, "Profile background cleanup deferred", slog.String("error_type", fmt.Sprintf("%T", err)))
	}
}

// CleanupProfileBackgrounds drains at most limit durable records. Database CAS
// leases allow concurrent processes without in-process-only coordination.
func (s *BindingService) CleanupProfileBackgrounds(ctx context.Context, limit int) (int, error) {
	if s == nil || s.pjskDB == nil || s.bgStorage == nil || s.readOnly {
		return 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	rows, err := s.pjskDB.ProfileBGCleanup.Query().Where(profilebgcleanup.NotBeforeLTE(time.Now().UTC())).Order(pjskdb.Asc(profilebgcleanup.FieldNotBefore)).Limit(limit).All(ctx)
	if err != nil {
		return 0, err
	}
	var failures []error
	completed := 0
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return completed, errors.Join(append(failures, err)...)
		}
		done, err := s.cleanupProfileBG(ctx, row)
		if err != nil {
			failures = append(failures, err)
		}
		if done {
			completed++
		}
	}
	return completed, errors.Join(failures...)
}

func (s *BindingService) cleanupProfileBG(ctx context.Context, row *pjskdb.ProfileBGCleanup) (bool, error) {
	now := time.Now().UTC()
	if row.NotBefore.After(now) {
		return false, nil
	}
	leaseUntil := now.Add(profileBGCleanupLease)
	claimed, err := s.pjskDB.ProfileBGCleanup.Update().Where(profilebgcleanup.IDEQ(row.ID), profilebgcleanup.StateEQ(row.State),
		profilebgcleanup.AttemptsEQ(row.Attempts), profilebgcleanup.NotBeforeLTE(now)).SetState(profilebgcleanup.StateDeleting).SetNotBefore(leaseUntil).AddAttempts(1).Save(ctx)
	if err != nil || claimed != 1 {
		return false, err
	}
	// Protect existing/manual references too. Immutable API-created object keys
	// cannot be restored by stale adjustment after their account CAS changes.
	referenced, err := s.pjskDB.GameAccount.Query().Where(func(selector *sql.Selector) {
		selector.Where(sqljson.ValueEQ(gameaccount.FieldBg, row.ObjectPath, sqljson.Path("img_path")))
	}).Exist(ctx)
	if err != nil {
		return false, err
	}
	if referenced {
		return false, errors.New("profile background cleanup still referenced")
	}
	if _, err := profileBGCleanupKey(row.ObjectPath); err != nil {
		return false, err
	}
	deleteCtx, cancel := context.WithTimeout(ctx, profileBGDeleteTimeout)
	err = s.bgStorage.DeleteProfileBackground(deleteCtx, &drawing.ProfileBgSettings{ImgPath: &row.ObjectPath})
	cancel()
	if err != nil {
		return false, err
	}
	removed, err := s.pjskDB.ProfileBGCleanup.Delete().Where(profilebgcleanup.IDEQ(row.ID), profilebgcleanup.StateEQ(profilebgcleanup.StateDeleting), profilebgcleanup.AttemptsEQ(row.Attempts+1)).Exec(ctx)
	return removed == 1, err
}

// RunProfileBGCleanup is owned by the application's primary-writer lifecycle.
// Shutdown cancels I/O; unfinished work remains durable for the next process.
func (s *BindingService) RunProfileBGCleanup(ctx context.Context) {
	if s == nil || s.readOnly {
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		sweepCtx, cancel := context.WithTimeout(storage.WithBackgroundIO(ctx), 30*time.Second)
		_, err := s.CleanupProfileBackgrounds(sweepCtx, 100)
		cancel()
		if err != nil && ctx.Err() == nil {
			logger.WarnContext(ctx, "Profile background cleanup sweep failed", slog.String("error_type", fmt.Sprintf("%T", err)))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
