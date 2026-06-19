// Package identity provides shared user identity resolution across all modules
// (pjsk, chunithm, etc.). It maps (im_platform, im_user_id) to a haruki_user_id,
// automatically creating a new user record when one does not yet exist.
package identity

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"

	usersdb "haruki-cloud/database/users"
	"haruki-cloud/database/users/user"
	"haruki-cloud/internal/cluster"
)

// Resolver resolves an IM identity to a haruki_user_id.
// It is safe for concurrent use and can be shared across modules.
type Resolver struct {
	db       *usersdb.Client
	readOnly bool
}

// NewResolver creates a new Resolver backed by the given users DB client.
func NewResolver(db *usersdb.Client) *Resolver {
	return &Resolver{db: db}
}

func (r *Resolver) SetReadOnly(readOnly bool) {
	if r == nil {
		return
	}
	r.readOnly = readOnly
}

// ResolveOrCreate returns the haruki_user_id for (platform, userID).
// If no matching record exists, a new user is inserted with a generated
// 6-digit ID and the resolved ID is returned.
func (r *Resolver) ResolveOrCreate(ctx context.Context, platform, userID string) (int, error) {
	u, err := r.db.User.Query().
		Where(user.Platform(platform), user.UserID(userID)).
		Only(ctx)
	if err == nil {
		return u.ID, nil
	}
	if !usersdb.IsNotFound(err) {
		return 0, err
	}
	if err := cluster.EnsureWritable(r.readOnly); err != nil {
		return 0, err
	}

	// User not found — generate a random 6-digit ID and create a record.
	// Retry a small number of times to handle the rare ID collision.
	for range 5 {
		id, err := generateUserID()
		if err != nil {
			return 0, err
		}
		created, err := r.db.User.Create().
			SetID(id).
			SetPlatform(platform).
			SetUserID(userID).
			Save(ctx)
		if err == nil {
			return created.ID, nil
		}
		if !usersdb.IsConstraintError(err) {
			return 0, err
		}
		// Constraint error means ID collision — retry with a new ID.
	}

	return 0, errors.New("identity: failed to generate a unique user ID after retries")
}

// generateUserID returns a cryptographically random integer in [100000, 999999].
func generateUserID() (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()) + 100000, nil
}
