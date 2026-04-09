package honor

import (
	"log/slog"

	"haruki-cloud/utils/drawing"
)

func logHonorRequestTrace(query Query, req *drawing.HonorRequest) {
	if req == nil {
		slog.Info("honor trace", "honor_id", query.HonorID, "payload_nil", true)
		return
	}

	slog.Info(
		"honor trace",
		"honor_id", query.HonorID,
		"region", query.Region.String(),
		"is_main", query.IsMain,
		"honor_level", query.HonorLevel,
		"bonds_honor_word_id", query.BondsHonorWordID,
		"honor_type", stringPtrTraceValue(req.HonorType),
		"group_type", stringPtrTraceValue(req.GroupType),
		"honor_rarity", stringPtrTraceValue(req.HonorRarity),
		"honor_img_path", stringPtrTraceValue(req.HonorImgPath),
		"rank_img_path", stringPtrTraceValue(req.RankImgPath),
		"frame_img_path", stringPtrTraceValue(req.FrameImgPath),
		"frame_degree_level_img_path", stringPtrTraceValue(req.FrameDegreeLevelImgPath),
		"scroll_img_path", stringPtrTraceValue(req.ScrollImgPath),
		"word_img_path", stringPtrTraceValue(req.WordImgPath),
		"bonds_bg_path", stringPtrTraceValue(req.BondsBgPath),
		"bonds_bg_path2", stringPtrTraceValue(req.BondsBgPath2),
		"mask_img_path", stringPtrTraceValue(req.MaskImgPath),
		"lv_img_path", stringPtrTraceValue(req.LvImgPath),
		"lv6_img_path", stringPtrTraceValue(req.Lv6ImgPath),
	)
}

func stringPtrTraceValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
