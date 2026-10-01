package config

import (
	"fmt"
	"path"
	"strconv"
	"strings"
)

// PlayerFrameParts names local Drawing assets independently for every sprite.
type PlayerFrameParts struct {
	Base        string `yaml:"base" json:"base"`
	CenterTop   string `yaml:"centertop" json:"centertop"`
	LeftTop     string `yaml:"lefttop" json:"lefttop"`
	RightTop    string `yaml:"righttop" json:"righttop"`
	LeftBottom  string `yaml:"leftbottom" json:"leftbottom"`
	RightBottom string `yaml:"rightbottom" json:"rightbottom"`
}

type PlayerFrameOverride struct {
	Server     string           `yaml:"server"`
	UserID     string           `yaml:"user_id"`
	Horizontal PlayerFrameParts `yaml:"horizontal"`
}

func ValidatePlayerFrameOverrides(entries []PlayerFrameOverride) error {
	seen := make(map[string]bool, len(entries))
	for i, e := range entries {
		switch e.Server {
		case "jp", "cn", "en", "tw", "kr":
		default:
			return fmt.Errorf("player_frame_overrides[%d]: invalid server", i)
		}
		uid, err := strconv.ParseUint(e.UserID, 10, 64)
		if err != nil || uid == 0 || strconv.FormatUint(uid, 10) != e.UserID {
			return fmt.Errorf("player_frame_overrides[%d]: user_id must be a canonical positive decimal string", i)
		}
		key := e.Server + ":" + e.UserID
		if seen[key] {
			return fmt.Errorf("player_frame_overrides[%d]: duplicate account", i)
		}
		seen[key] = true
		for name, value := range map[string]string{"base": e.Horizontal.Base, "centertop": e.Horizontal.CenterTop, "lefttop": e.Horizontal.LeftTop, "righttop": e.Horizontal.RightTop, "leftbottom": e.Horizontal.LeftBottom, "rightbottom": e.Horizontal.RightBottom} {
			if !strings.HasPrefix(value, "static_images/") || path.Clean(value) != value || strings.ContainsAny(value, "\\\x00\r\n") || strings.Contains(value, ":") || value == "static_images/" {
				return fmt.Errorf("player_frame_overrides[%d].horizontal.%s: expected a clean relative local static_images path", i, name)
			}
		}
	}
	return nil
}
