package alias

import (
	"context"

	pjskdb "haruki-cloud/database/pjsk"
	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/internal/cluster"
)

func NewService(sekai *sekaiDB.Client, pjsk *pjskdb.Client, identity IdentityResolver) *Service {
	if sekai == nil || pjsk == nil {
		return nil
	}
	return &Service{
		sekai:    sekai,
		pjsk:     pjsk,
		identity: identity,
	}
}

func (s *Service) IsReady() bool {
	return s != nil && s.sekai != nil && s.pjsk != nil
}

func (s *Service) SetReadOnly(readOnly bool) {
	if s == nil {
		return
	}
	s.readOnly = readOnly
}

// SetChangeListener registers fn to run after every committed alias
// approval or deletion.
func (s *Service) SetChangeListener(fn ChangeListener) {
	if s == nil {
		return
	}
	s.onChange = fn
}

func (s *Service) notifyChanged(ctx context.Context, changes []AliasChange) {
	if s == nil || s.onChange == nil || len(changes) == 0 {
		return
	}
	s.onChange(ctx, changes)
}

func (s *Service) requireWritable() error {
	if s == nil {
		return cluster.ErrReadOnly
	}
	return cluster.EnsureWritable(s.readOnly)
}
