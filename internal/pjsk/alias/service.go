package alias

import (
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

func (s *Service) requireWritable() error {
	if s == nil {
		return cluster.ErrReadOnly
	}
	return cluster.EnsureWritable(s.readOnly)
}
