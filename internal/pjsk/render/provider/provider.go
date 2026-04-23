// Package provider defines a unified MasterDataProvider interface for accessing
// Project SEKAI masterdata. Both database-backed and local-file-backed
// implementations satisfy the same interface, allowing the application to
// switch data sources transparently.
package provider

import (
	renderregion "haruki-cloud/internal/pjsk/region"
)

// MasterDataProvider is the top-level interface that aggregates all
// domain-specific data providers. Obtain a concrete instance via
// NewDatabaseProvider or NewLocalProvider.
type MasterDataProvider interface {
	Region() renderregion.Value

	Cards() CardProvider
	Characters() CharacterProvider
	Skills() SkillProvider
	Events() EventProvider
	Musics() MusicProvider
	Gachas() GachaProvider
	Honors() HonorProvider
	Stamps() StampProvider
	VLives() VLiveProvider
	Education() EducationProvider
	PlayerFrames() PlayerFrameProvider
	MySekai() MySekaiProvider
}
