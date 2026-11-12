package profile

import (
	"haruki-cloud/config"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/playerframe"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

// SetFrameOverrides is called only by the composition root using file config.
// The controller keeps value copies; no request or database setting can mutate it.
func (c *Controller) SetFrameOverrides(entries []config.PlayerFrameOverride) {
	c.frameOverrides = playerframe.NewOverrides(entries)
}

func (c *Controller) SetFramePreferences(preferences playerframe.Preferences) {
	if c != nil {
		c.framePreferences = preferences
	}
}

func (c *Controller) buildAccountFramePaths(source DataSource, frames []snapshot.RawUserFrame, server, userID string) (*drawing.PlayerFramePaths, bool) {
	var frameSource playerframe.Source
	if source != nil {
		frameSource = source
	}
	paths := playerframe.ResolvePreferredAccount(c.contextOrBackground(), c.framePreferences, frameSource, c.frameOverrides, renderregion.Normalize(server), userID, frames)
	return paths, paths != nil
}
