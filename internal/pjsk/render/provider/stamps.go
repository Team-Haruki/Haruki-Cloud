package provider

import (
	"context"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

// StampProvider exposes stamp masterdata queries.
type StampProvider interface {
	GetAll(ctx context.Context) ([]masterdata.Stamp, error)
}
