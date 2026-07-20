package music

import "haruki-cloud/internal/pjsk/meta"

type musicMetaViewSnapshot interface {
	MusicMetaView() *meta.View
}

func (c *Controller) resolveMusicMetaView(region string) *meta.View {
	if c != nil && c.metaLoader != nil {
		if view := c.metaLoader.View(region); view != nil {
			return view
		}
	}
	if snapshot := c.currentSnapshot(); snapshot != nil {
		if source, ok := snapshot.(musicMetaViewSnapshot); ok {
			if view := source.MusicMetaView(); view != nil {
				return view
			}
		}
		if payload := snapshot.MusicMetaBytes(); len(payload) > 0 {
			view, _ := meta.Parse(payload)
			return view
		}
	}
	return nil
}
