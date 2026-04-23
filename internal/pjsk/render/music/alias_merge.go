package music

import (
	"context"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
)

type musicApprovedAliasProvider interface {
	ListApprovedMusicAliases(ctx context.Context, musicID int) ([]string, error)
}

func (c *Controller) appendApprovedMusicAliases(req *drawing.MusicDetailRequest, musicID int) {
	if c == nil || req == nil || musicID <= 0 {
		return
	}
	provider, ok := c.aliases.(musicApprovedAliasProvider)
	if !ok || provider == nil {
		return
	}
	aliases, err := provider.ListApprovedMusicAliases(c.contextOrBackground(), musicID)
	if err != nil {
		return
	}
	req.Alias = mergeMusicAliasTexts(req.Alias, aliases...)
}

func mergeMusicAliasTexts(existing []string, incoming ...string) []string {
	if len(existing) == 0 && len(incoming) == 0 {
		return nil
	}

	result := make([]string, 0, len(existing)+len(incoming))
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	appendValue := func(raw string) {
		value := strings.TrimSpace(raw)
		if value == "" {
			return
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}

	for _, value := range existing {
		appendValue(value)
	}
	for _, value := range incoming {
		appendValue(value)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
