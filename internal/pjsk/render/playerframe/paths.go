// Package playerframe resolves equipped single and six-slot combination frames.
package playerframe

import (
	"context"
	"path"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
)

type PartLayout struct {
	PartPosition    string `json:"partPosition"`
	GameCharacterID int    `json:"gameCharacterId"`
}

type UserFrame struct {
	PlayerFrameID           int          `json:"playerFrameId"`
	PlayerFrameAttachStatus string       `json:"playerFrameAttachStatus"`
	PartsLayout             []PartLayout `json:"partsLayout,omitempty"`
}

type Source interface {
	DefaultRegion() renderregion.Value
	GetPlayerFrameByID(int) (*masterdata.PlayerFrame, error)
	GetPlayerFrameGroupByID(int) (*masterdata.PlayerFrameGroup, error)
	GetPlayerFramePartsByGroupID(int) (map[int]int, error)
}

// attached reports whether a userPlayerFrames entry is the one on display. The client's
// PlayerFrameAttachStatus enum is {none = -1, first = 0}: the server sends "first" for the
// equipped frame and "none" (or nothing) for the rest of the collection.
func attached(status string) bool {
	status = strings.TrimSpace(status)
	return status != "" && !strings.EqualFold(status, "none")
}

func Build(source Source, frames []UserFrame) *drawing.PlayerFramePaths {
	for _, equipped := range frames {
		if !attached(equipped.PlayerFrameAttachStatus) {
			continue
		}
		frame, err := source.GetPlayerFrameByID(equipped.PlayerFrameID)
		if err != nil || frame == nil {
			return nil
		}
		group, err := source.GetPlayerFrameGroupByID(frame.PlayerFrameGroupID)
		if err != nil || group == nil || strings.TrimSpace(group.AssetBundleName) == "" {
			return nil
		}
		root := path.Join(assets.RegionAssetDir(source.DefaultRegion().String()), "player_frame", group.AssetBundleName, strconv.Itoa(frame.ID))
		image := func(name string) string { return path.Join(root, "vertical", "frame_"+name+".png") }
		if group.PlayerFrameType == "" || group.PlayerFrameType == "single" {
			return &drawing.PlayerFramePaths{FrameType: "single", Base: image("base"), CenterTop: image("centertop"), LeftTop: image("lefttop"), RightTop: image("righttop"), LeftBottom: image("leftbottom"), RightBottom: image("rightbottom")}
		}
		if group.PlayerFrameType != "combination" || len(equipped.PartsLayout) != 6 {
			return nil
		}
		parts, err := source.GetPlayerFramePartsByGroupID(group.ID)
		if err != nil {
			return nil
		}
		slots := make(map[string]int, 6)
		for _, part := range equipped.PartsLayout {
			if _, duplicate := slots[part.PartPosition]; duplicate {
				return nil
			}
			slots[part.PartPosition] = parts[part.GameCharacterID]
		}
		for n := 1; n <= 6; n++ {
			if slots["part"+strconv.Itoa(n)] <= 0 {
				return nil
			}
		}
		partImage := func(slot int, name string) string {
			return path.Join(root, strconv.Itoa(slots["part"+strconv.Itoa(slot)]), "vertical", "frame_"+name+".png")
		}
		return &drawing.PlayerFramePaths{
			FrameType: "combination", Base: partImage(1, "base"),
			LeftTop: partImage(1, "parts1_left"), RightTop: partImage(1, "parts1_right"), CenterTop: partImage(1, "parts1_center"),
			SideLeftTop: partImage(2, "parts2_left"), SideRightTop: partImage(3, "parts3_right"),
			SideLeftBottom: partImage(4, "parts4_left"), SideRightBottom: partImage(5, "parts5_right"),
			LeftBottom: partImage(6, "parts6_left"), RightBottom: partImage(6, "parts6_right"),
		}
	}
	return nil
}

type providerSource struct {
	region renderregion.Value
	ctx    context.Context
	p      provider.PlayerFrameProvider
}

func (s providerSource) DefaultRegion() renderregion.Value { return s.region }

func (s providerSource) GetPlayerFrameByID(id int) (*masterdata.PlayerFrame, error) {
	return s.p.GetByID(s.ctx, id)
}
func (s providerSource) GetPlayerFrameGroupByID(id int) (*masterdata.PlayerFrameGroup, error) {
	return s.p.GetGroupByID(s.ctx, id)
}
func (s providerSource) GetPlayerFramePartsByGroupID(id int) (map[int]int, error) {
	return s.p.GetPartsByGroupID(s.ctx, id)
}

func Resolve(ctx context.Context, p provider.PlayerFrameProvider, region renderregion.Value, frames []UserFrame) *drawing.PlayerFramePaths {
	if p == nil {
		return nil
	}
	return Build(providerSource{ctx: ctx, p: p, region: region}, frames)
}
