package score

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
)

type Controller struct {
	drawing *drawing.HarukiDrawingClient
}

func NewController(drawingClient *drawing.HarukiDrawingClient) *Controller {
	return &Controller{drawing: drawingClient}
}

func (c *Controller) WithContext(ctx context.Context) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	clone.drawing = c.drawing.WithContext(ctx)
	return &clone
}

func (c *Controller) BuildScoreControlRequest(req drawing.ScoreControlRequest) (*drawing.ScoreControlRequest, error) {
	if req.MusicID <= 0 || req.TargetPoint <= 0 {
		return nil, fmt.Errorf("invalid score control request")
	}
	req.MusicCoverPath = normalizeScoreCoverPath(req.MusicCoverPath)
	return &req, nil
}

func (c *Controller) RenderScoreControl(req drawing.ScoreControlRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildScoreControlRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateScoreControl(payload)
}

func (c *Controller) BuildCustomRoomScoreRequest(req drawing.CustomRoomScoreRequest) (*drawing.CustomRoomScoreRequest, error) {
	if req.TargetPoint <= 0 || len(req.CandidatePairs) == 0 {
		return nil, fmt.Errorf("invalid custom-room score request")
	}
	for key, list := range req.MusicListMap {
		for idx := range list {
			if raw, ok := list[idx]["music_cover"].(string); ok {
				list[idx]["music_cover"] = normalizeScoreCoverPath(raw)
			}
		}
		req.MusicListMap[key] = list
	}
	return &req, nil
}

func (c *Controller) RenderCustomRoomScore(req drawing.CustomRoomScoreRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildCustomRoomScoreRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateCustomRoomScore(payload)
}

func (c *Controller) BuildMusicMetaRequest(req []drawing.MusicMetaRequest) ([]drawing.MusicMetaRequest, error) {
	if len(req) == 0 {
		return nil, fmt.Errorf("music meta request is empty")
	}
	for idx := range req {
		req[idx].MusicCoverPath = normalizeScoreCoverPath(req[idx].MusicCoverPath)
	}
	return req, nil
}

func (c *Controller) RenderMusicMeta(req []drawing.MusicMetaRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildMusicMetaRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMusicMeta(payload)
}

func (c *Controller) BuildMusicBoardRequest(req drawing.MusicBoardRequest) (*drawing.MusicBoardRequest, error) {
	if len(req.Items) == 0 {
		return nil, fmt.Errorf("music board request has no items")
	}
	for idx := range req.Items {
		req.Items[idx].MusicCoverPath = normalizeScoreCoverPath(req.Items[idx].MusicCoverPath)
	}
	return &req, nil
}

func (c *Controller) RenderMusicBoard(req drawing.MusicBoardRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildMusicBoardRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMusicBoard(payload)
}

func normalizeScoreCoverPath(raw string) string {
	path := filepath.ToSlash(strings.TrimSpace(raw))
	if path == "" {
		return path
	}
	const legacyPrefix = "jacket/"
	const modernPrefix = "music/jacket/"

	if strings.HasPrefix(path, legacyPrefix) {
		rest := strings.TrimPrefix(path, legacyPrefix)
		parts := strings.Split(rest, "/")
		if len(parts) >= 2 {
			dir := strings.TrimSuffix(parts[0], "_rip")
			file := parts[1]
			if dir != "" && file != "" {
				return modernPrefix + dir + "/" + file
			}
		}
		return modernPrefix + rest
	}
	if strings.HasPrefix(path, modernPrefix) {
		return path
	}
	return path
}
