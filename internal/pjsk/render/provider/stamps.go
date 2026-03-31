package provider

import "haruki-cloud/internal/pjsk/render/masterdata"

// StampProvider exposes stamp masterdata queries.
type StampProvider interface {
	GetAll() ([]masterdata.Stamp, error)
}
