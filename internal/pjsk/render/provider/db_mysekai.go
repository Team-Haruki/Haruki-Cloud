package provider

import (
	sekaiDB "haruki-cloud/database/sekai"
	renderregion "haruki-cloud/internal/pjsk/region"
)

type dbMySekaiProvider struct {
	client *sekaiDB.Client
	region renderregion.Value
	local  *localMySekaiProvider
}

func (p *dbMySekaiProvider) Configured() bool {
	return p != nil && p.local != nil && p.local.Configured()
}

func (p *dbMySekaiProvider) LoadList(filename string) []map[string]any {
	if p == nil || p.local == nil {
		return nil
	}
	return p.local.LoadList(filename)
}

func (p *dbMySekaiProvider) LoadMapByID(filename string) map[int]map[string]any {
	if p == nil || p.local == nil {
		return nil
	}
	return p.local.LoadMapByID(filename)
}

func (p *dbMySekaiProvider) LoadObject(filename string, target any) bool {
	if p == nil || p.local == nil {
		return false
	}
	return p.local.LoadObject(filename, target)
}
