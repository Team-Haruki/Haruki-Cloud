package provider

import (
	sekaiDB "haruki-cloud/database/sekai"
	renderregion "haruki-cloud/internal/pjsk/region"
)

type dbMySekaiProvider struct {
	client *sekaiDB.Client
	region renderregion.Value
}

func (p *dbMySekaiProvider) Configured() bool {
	return false
}

func (p *dbMySekaiProvider) LoadList(_ string) []map[string]any {
	return nil
}

func (p *dbMySekaiProvider) LoadMapByID(_ string) map[int]map[string]any {
	return nil
}

func (p *dbMySekaiProvider) LoadObject(_ string, _ any) bool {
	return false
}
