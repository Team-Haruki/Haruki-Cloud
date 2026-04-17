package sekai

import (
	"fmt"
	"strings"

	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/common"
)

var (
	profileHorizontalKeywords = []string{"横屏", "横向", "横版"}
	profileVerticalKeywords   = []string{"竖屏", "竖向", "竖版", "纵向"}
)

func extractProfileVerticalArg(args string) (*bool, string) {
	remaining := strings.TrimSpace(args)
	for _, keyword := range profileHorizontalKeywords {
		if strings.Contains(remaining, keyword) {
			return new(false), strings.TrimSpace(strings.Replace(remaining, keyword, "", 1))
		}
	}
	for _, keyword := range profileVerticalKeywords {
		if strings.Contains(remaining, keyword) {
			return new(true), strings.TrimSpace(strings.Replace(remaining, keyword, "", 1))
		}
	}
	return nil, remaining
}

func extractFirstImageURL(ctx HarrukiSekaiHandlerContext) string {
	for _, segment := range ctx.GetMessage() {
		if segment.Type != "image" {
			continue
		}
		imgData, ok := segment.Data.(onebot11.ImageData)
		if !ok {
			continue
		}

		if imageURL := strings.TrimSpace(imgData.Url); imageURL != "" {
			return imageURL
		}
		if fileURL := strings.TrimSpace(imgData.File); strings.HasPrefix(strings.ToLower(fileURL), "http://") || strings.HasPrefix(strings.ToLower(fileURL), "https://") {
			return fileURL
		}
	}
	return ""
}

func parseProfileBGAdjustArgs(args string) (accountdata.ProfileSettingsCommandParams, error) {
	params := accountdata.ProfileSettingsCommandParams{}
	args = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(args, "度", ""), "%", ""))
	if args == "" {
		return params, nil
	}
	params.Vertical, args = extractProfileVerticalArg(args)

	tokens := strings.Fields(args)
	for i := 0; i < len(tokens); i++ {
		token := strings.TrimSpace(tokens[i])
		switch strings.ToLower(token) {
		case "模糊", "blur":
			if i+1 >= len(tokens) {
				return params, onebot11.NewReplayError("使用方式:\n调整个人信息背景 [横屏|竖屏] [模糊 0~10] [透明 0~100]")
			}
			value, err := parseProfileBGInt(tokens[i+1], 0, 10)
			if err != nil {
				return params, err
			}
			params.Blur = &value
			i++
		case "透明", "alpha":
			if i+1 >= len(tokens) {
				return params, onebot11.NewReplayError("使用方式:\n调整个人信息背景 [横屏|竖屏] [模糊 0~10] [透明 0~100]")
			}
			value, err := parseProfileBGInt(tokens[i+1], 0, 100)
			if err != nil {
				return params, err
			}
			params.Alpha = &value
			i++
		default:
			if strings.HasPrefix(token, "模糊") {
				value, err := parseProfileBGInt(strings.TrimPrefix(token, "模糊"), 0, 10)
				if err != nil {
					return params, err
				}
				params.Blur = &value
				continue
			}
			if strings.HasPrefix(token, "透明") {
				value, err := parseProfileBGInt(strings.TrimPrefix(token, "透明"), 0, 100)
				if err != nil {
					return params, err
				}
				params.Alpha = &value
				continue
			}
			return params, fmt.Errorf("无法识别的个人信息背景参数: %s", token)
		}
	}
	return params, nil
}

func parseProfileBGInt(raw string, minValue, maxValue int) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("请提供正确的数值")
	}
	n := 0
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("请提供正确的数值")
		}
		n = n*10 + int(ch-'0')
	}
	if n < minValue || n > maxValue {
		return 0, fmt.Errorf("数值超出范围，需要在 %d~%d 之间", minValue, maxValue)
	}
	return n, nil
}

func resolveProfileBGSelector(ctx HarrukiSekaiHandlerContext) (string, error) {
	uidArg := strings.TrimSpace(ctx.UIDArg())
	if uidArg == "" {
		return "", nil
	}
	if isBindingSelector(uidArg) {
		return uidArg, nil
	}
	return "", onebot11.NewReplayError("此设置仅支持操作自己的账号\n使用方式：%s [u序号] ...", ctx.originalTriggerCmd)
}

func (sekaiHandlers) ProfileUploadBGHandle() HarukiSekaiCommandHandler {
	return HarukiSekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk upload profile bg", "/pjsk upload profile background",
				"/上传个人信息背景", "/上传个人信息图片", "/上传个人背景", "/上传个人信息",
			},
			Path: "profile/bg/upload",
		},
		ParseUIDArg: common.BoolPtr(true),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*parser.ResolvedCommand, error) {
			imageURL := extractFirstImageURL(ctx)
			if imageURL == "" {
				return nil, onebot11.NewReplayError("请在命令中附带一张个人信息背景图片")
			}
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			params := newProfileSettingsParams(ctx, selector)
			params.ImageURL = imageURL
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeBGUpload, params), nil
		},
	}
}

func (sekaiHandlers) ProfileClearBGHandle() HarukiSekaiCommandHandler {
	return HarukiSekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk clear profile bg", "/pjsk clear profile background",
				"/清空个人信息背景", "/清除个人信息背景", "/清空个人信息图片", "/清除个人信息图片",
			},
			Path: "profile/bg/clear",
		},
		ParseUIDArg: common.BoolPtr(true),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*parser.ResolvedCommand, error) {
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeBGClear, newProfileSettingsParams(ctx, selector)), nil
		},
	}
}

func (sekaiHandlers) ProfileAdjustBGHandle() HarukiSekaiCommandHandler {
	return HarukiSekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk adjust profile", "/pjsk adjust profile bg", "/pjsk adjust profile background",
				"/调整个人信息背景", "/调整个人信息", "/设置个人信息", "/设置个人信息背景",
			},
			Path: "profile/bg/adjust",
		},
		ParseUIDArg: common.BoolPtr(true),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*parser.ResolvedCommand, error) {
			selector, err := resolveProfileBGSelector(ctx)
			if err != nil {
				return nil, err
			}
			params := newProfileSettingsParams(ctx, selector)
			adjustParams, err := parseProfileBGAdjustArgs(ctx.GetArgs())
			if err != nil {
				return nil, err
			}
			params.Blur = adjustParams.Blur
			params.Alpha = adjustParams.Alpha
			params.Vertical = adjustParams.Vertical
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeBGAdjust, params), nil
		},
	}
}
