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

// PlayerFrameOverride replaces one account's equipped frame. Horizontal (list-row sprites) is
// required; Vertical (player-cell sprites, used by /profile's panel) is optional — without it
// Drawing falls back to the horizontal sprites there.
type PlayerFrameOverride struct {
	Server     string            `yaml:"server"`
	UserID     string            `yaml:"user_id"`
	Horizontal PlayerFrameParts  `yaml:"horizontal"`
	Vertical   *PlayerFrameParts `yaml:"vertical,omitempty"`
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
		if err := validatePlayerFrameParts(i, "horizontal", e.Horizontal); err != nil {
			return err
		}
		if e.Vertical != nil {
			if err := validatePlayerFrameParts(i, "vertical", *e.Vertical); err != nil {
				return err
			}
		}
	}
	return nil
}

func validatePlayerFrameParts(i int, cell string, p PlayerFrameParts) error {
	for name, value := range map[string]string{"base": p.Base, "centertop": p.CenterTop, "lefttop": p.LeftTop, "righttop": p.RightTop, "leftbottom": p.LeftBottom, "rightbottom": p.RightBottom} {
		if !strings.HasPrefix(value, "static_images/") || path.Clean(value) != value || strings.ContainsAny(value, "\\\x00\r\n") || strings.Contains(value, ":") || value == "static_images/" {
			return fmt.Errorf("player_frame_overrides[%d].%s.%s: expected a clean relative local static_images path", i, cell, name)
		}
	}
	return nil
}
