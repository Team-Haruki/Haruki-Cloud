package profile

import (
	"haruki-cloud/config"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

// SetFrameOverrides is called only by the composition root using file config.
// The controller keeps value copies; no request or database setting can mutate it.
func (c *Controller) SetFrameOverrides(entries []config.PlayerFrameOverride) {
	c.frameOverrides = make(map[string]drawing.PlayerFrameParts, len(entries))
	for _, e := range entries {
		p := e.Horizontal
		c.frameOverrides[e.Server+":"+e.UserID] = drawing.PlayerFrameParts{Base: p.Base, CenterTop: p.CenterTop, LeftTop: p.LeftTop, RightTop: p.RightTop, LeftBottom: p.LeftBottom, RightBottom: p.RightBottom}
	}
}

func (c *Controller) buildAccountFramePaths(source DataSource, frames []snapshot.RawUserFrame, server, userID string) (*drawing.PlayerFramePaths, bool) {
	if parts, ok := c.frameOverrides[server+":"+userID]; ok {
		return &drawing.PlayerFramePaths{FrameType: "single", Base: parts.Base, CenterTop: parts.CenterTop, LeftTop: parts.LeftTop, RightTop: parts.RightTop, LeftBottom: parts.LeftBottom, RightBottom: parts.RightBottom, Horizontal: &parts}, true
	}
	return c.buildFramePaths(source, frames)
}
