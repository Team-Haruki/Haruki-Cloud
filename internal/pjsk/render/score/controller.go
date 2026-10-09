package score

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/utils/usererror"
)

type Controller struct {
	drawing    *drawing.HarukiDrawingClient
	requestCtx context.Context
}

func NewController(drawingClient *drawing.HarukiDrawingClient) *Controller {
	return &Controller{drawing: drawingClient}
}

func (c *Controller) WithContext(ctx context.Context) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	clone.requestCtx = ctx
	clone.drawing = c.drawing.WithContext(ctx)
	return &clone
}

func (c *Controller) BuildScoreControlRequest(req drawing.ScoreControlRequest) (*drawing.ScoreControlRequest, error) {
	if req.MusicID <= 0 || req.TargetPoint <= 0 {
		return nil, usererror.Misuse(i18n.M("score.control.invalid"))
	}
	req.MusicCoverPath = normalizeScoreCoverPath(req.MusicCoverPath)
	return &req, nil
}

func (c *Controller) RenderScoreControl(req drawing.ScoreControlRequest) ([]byte, error) {
	image, err := c.RenderScoreControlImage(req)
	if err != nil {
		return nil, err
	}
	return image.Bytes(c.requestCtx)
}

func (c *Controller) RenderScoreControlImage(req drawing.ScoreControlRequest) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, drawing.ErrNotConfigured
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, payloadBuildStage)
	payload, err := c.BuildScoreControlRequest(req)
	finishBuild()
	if err != nil {
		return drawing.ImageResult{}, err
	}
	return c.drawing.GenerateScoreControlImage(payload)
}

func (c *Controller) BuildCustomRoomScoreRequest(req drawing.CustomRoomScoreRequest) (*drawing.CustomRoomScoreRequest, error) {
	if req.TargetPoint <= 0 || len(req.CandidatePairs) == 0 {
		return nil, usererror.Misuse(i18n.M("score.custom_room.usage"))
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
	image, err := c.RenderCustomRoomScoreImage(req)
	if err != nil {
		return nil, err
	}
	return image.Bytes(c.requestCtx)
}

func (c *Controller) RenderCustomRoomScoreImage(req drawing.CustomRoomScoreRequest) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, drawing.ErrNotConfigured
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, payloadBuildStage)
	payload, err := c.BuildCustomRoomScoreRequest(req)
	finishBuild()
	if err != nil {
		return drawing.ImageResult{}, err
	}
	return c.drawing.GenerateCustomRoomScoreImage(payload)
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
	image, err := c.RenderMusicMetaImage(req)
	if err != nil {
		return nil, err
	}
	return image.Bytes(c.requestCtx)
}

func (c *Controller) RenderMusicMetaImage(req []drawing.MusicMetaRequest) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, drawing.ErrNotConfigured
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, payloadBuildStage)
	payload, err := c.BuildMusicMetaRequest(req)
	finishBuild()
	if err != nil {
		return drawing.ImageResult{}, err
	}
	return c.drawing.GenerateMusicMetaImage(payload)
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
	image, err := c.RenderMusicBoardImage(req)
	if err != nil {
		return nil, err
	}
	return image.Bytes(c.requestCtx)
}

func (c *Controller) RenderMusicBoardImage(req drawing.MusicBoardRequest) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, drawing.ErrNotConfigured
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, payloadBuildStage)
	payload, err := c.BuildMusicBoardRequest(req)
	finishBuild()
	if err != nil {
		return drawing.ImageResult{}, err
	}
	return c.drawing.GenerateMusicBoardImage(payload)
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
