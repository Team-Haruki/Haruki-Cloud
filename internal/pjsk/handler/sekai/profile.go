package sekai

import (
	"fmt"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"strconv"
	"strings"
)

func (sekaiHandlers) ProfileBindHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk bind", "/pjsk id",
				"/绑定", "/pjsk 绑定",
			},
			Disabled: true,
		},
		// TODO: parse_uid_arg=False 的行为目前未迁移
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			// TODO: 迁移查询绑定列表 + 绑定新ID + 跨区冲突检查 + add_player_bind_id 逻辑
			return nil, fmt.Errorf("TODO: 绑定/查询绑定未实现，args=%q", args)
		},
	}
}

func (sekaiHandlers) ProfileUnbindHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk unbind", "/pjsk解绑", "/解绑",
			},
			Disabled: true,
		},
		// TODO: parse_uid_arg=False 的行为目前未迁移
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.ToLower(strings.TrimSpace(ctx.GetArgs()))
			args = strings.ReplaceAll(args, "u", "")
			index, err := strconv.Atoi(args)
			if err != nil {
				return nil, fmt.Errorf("解除第x个账号绑定:\"%s x\"\n发送\"/绑定\"查询已绑定的账号", ctx.originalTriggerCmd)
			}
			// TODO: 迁移 remove_player_bind_id(ctx, qid, index-1)
			return nil, fmt.Errorf("TODO: 解绑未实现，index=%d", index)
		},
	}
}

func (sekaiHandlers) ProfileSetMainHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk set main", "/pjsk主账号", "/设置主账号", "/主账号",
			},
			Disabled: true,
		},
		// TODO: parse_uid_arg=False 的行为目前未迁移
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(strings.ReplaceAll(ctx.GetArgs(), "u", ""))
			index, err := strconv.Atoi(args)
			if err != nil {
				return nil, fmt.Errorf("使用方式:\n设置主账号为你第x个绑定的账号: %s x", ctx.originalTriggerCmd)
			}
			// TODO: 迁移 set_player_main_bind_id(ctx, qid, index-1)
			return nil, fmt.Errorf("TODO: 设置主账号未实现，index=%d", index)
		},
	}
}

func (sekaiHandlers) ProfileSwapBindHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk swap bind", "/pjsk交换绑定",
				"/交换绑定", "/绑定交换", "/交换账号", "/交换账号顺序",
			},
			Disabled: true,
		},
		// TODO: parse_uid_arg=False 的行为目前未迁移
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			parts := strings.Fields(strings.TrimSpace(ctx.GetArgs()))
			if len(parts) != 2 {
				return nil, fmt.Errorf("使用方式:\n%s u1 u2", ctx.originalTriggerCmd)
			}
			// TODO: 迁移 swap_player_bind_id(ctx, qid, index1, index2)
			return nil, fmt.Errorf("TODO: 交换绑定未实现，parts=%v", parts)
		},
	}
}

func (sekaiHandlers) ProfileHideSuiteHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk hide suite", "/pjsk隐藏抓包", "/隐藏抓包",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			// TODO: 迁移 hide_suite_list 写入逻辑
			return nil, fmt.Errorf("TODO: 隐藏抓包信息未实现，user_id=%s", ctx.GetUserId())
		},
	}
}

func (sekaiHandlers) ProfileShowSuiteHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk show suite", "/pjsk显示抓包", "/pjsk展示抓包", "/展示抓包",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			// TODO: 迁移 hide_suite_list 移除逻辑
			return nil, fmt.Errorf("TODO: 展示抓包信息未实现，user_id=%s", ctx.GetUserId())
		},
	}
}

func (sekaiHandlers) ProfileHideIDHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk hide id", "/pjsk隐藏id", "/pjsk隐藏ID", "/隐藏id", "/隐藏ID",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			// TODO: 迁移 hide_id_list 写入逻辑
			return nil, fmt.Errorf("TODO: 隐藏ID信息未实现，user_id=%s", ctx.GetUserId())
		},
	}
}

func (sekaiHandlers) ProfileShowIDHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk show id", "/pjsk显示id", "/pjsk显示ID", "/pjsk展示id", "/pjsk展示ID",
				"/展示id", "/展示ID", "/显示id", "/显示ID",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			// TODO: 迁移 hide_id_list 移除逻辑
			return nil, fmt.Errorf("TODO: 展示ID信息未实现，user_id=%s", ctx.GetUserId())
		},
	}
}

func (sekaiHandlers) ProfileInfoHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk profile", "/个人信息", "/名片", "/pjsk 个人信息", "/pjsk 名片",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return makeResolvedCmd(ctx, parser.ModuleProfile, "profile"), nil
		},
	}
}

func (sekaiHandlers) ProfileRegTimeHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk reg time", "/注册时间", "/pjsk 注册时间", "/查时间",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			// TODO: 迁移 get_player_bind_id + get_register_time 逻辑
			return nil, fmt.Errorf("TODO: 注册时间查询未实现，user_id=%s", ctx.GetUserId())
		},
	}
}

func (sekaiHandlers) ProfileCheckServiceHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk check service", "/pcs", "/pjsk检查服务状态",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			// TODO: 迁移 get_service_status 逻辑
			return nil, fmt.Errorf("TODO: profile服务状态检查未实现")
		},
	}
}

func (sekaiHandlers) ProfileDataModeHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk data mode", "/pjsk抓包模式", "/pjsk抓包获取模式", "/抓包模式",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			// TODO: 迁移 data_modes 查询/切换逻辑
			return nil, fmt.Errorf("TODO: 抓包模式查询/设置未实现，args=%q", args)
		},
	}
}

func (sekaiHandlers) ProfileCheckDataHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk check data", "/pjsk抓包", "/pjsk抓包状态", "/pjsk抓包数据", "/pjsk抓包查询", "/抓包数据", "/抓包状态", "/抓包信息",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			// TODO: 迁移 at用户解析 + get_suite_upload_time + 文本组装逻辑
			return nil, fmt.Errorf("TODO: 抓包状态查询未实现，user_id=%s", ctx.GetUserId())
		},
	}
}

func (sekaiHandlers) ProfileBlacklistAddHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk blacklist add", "/pjsk add blacklist",
				"/pjsk黑名单添加", "/pjsk添加黑名单",
			},
			Disabled: true,
		},
		// TODO: refer 中这里是 CmdHandler（非 SekaiCmdHandler）
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if args == "" {
				return nil, fmt.Errorf("请提供要添加的游戏ID")
			}
			// TODO: 迁移 superuser 校验 + blacklist 写入逻辑
			return nil, fmt.Errorf("TODO: 添加黑名单未实现，uid=%q", args)
		},
	}
}

func (sekaiHandlers) ProfileBlacklistRemoveHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk blacklist remove", "/pjsk blacklist del", "/pjsk remove blacklist", "/pjsk del blacklist",
				"/pjsk黑名单移除", "/pjsk移除黑名单", "/pjsk删除黑名单",
			},
			Disabled: true,
		},
		// TODO: refer 中这里是 CmdHandler（非 SekaiCmdHandler）
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if args == "" {
				return nil, fmt.Errorf("请提供要移除的游戏ID")
			}
			// TODO: 迁移 superuser 校验 + blacklist 移除逻辑
			return nil, fmt.Errorf("TODO: 移除黑名单未实现，uid=%q", args)
		},
	}
}

func (sekaiHandlers) ProfileVerifyHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk verify", "/pjsk验证",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			// TODO: 迁移 block_region + verify_user_game_account(ctx)
			return nil, fmt.Errorf("TODO: 游戏账号验证未实现，user_id=%s", ctx.GetUserId())
		},
	}
}

func (sekaiHandlers) ProfileVerifyListHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk verify list", "/pjsk验证列表", "/pjsk验证状态",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			// TODO: 迁移 get_user_verified_uids + 输出脱敏列表逻辑
			return nil, fmt.Errorf("TODO: 验证列表查询未实现，user_id=%s", ctx.GetUserId())
		},
	}
}

func (sekaiHandlers) ProfileUploadBGHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk upload profile bg", "/pjsk upload profile background",
				"/上传个人信息背景", "/上传个人信息图片", "/上传个人背景", "/上传个人信息",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			force := strings.Contains(args, "force")
			// TODO: 迁移开关校验 + block_region + 图片上传与回写逻辑
			return nil, fmt.Errorf("TODO: 上传个人信息背景未实现，force=%t", force)
		},
	}
}

func (sekaiHandlers) ProfileClearBGHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk clear profile bg", "/pjsk clear profile background",
				"/清空个人信息背景", "/清除个人信息背景", "/清空个人信息图片", "/清除个人信息图片",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			force := strings.Contains(args, "force")
			// TODO: 迁移 block_region + 清理背景逻辑
			return nil, fmt.Errorf("TODO: 清空个人背景未实现，force=%t", force)
		},
	}
}

func (sekaiHandlers) ProfileAdjustBGHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk adjust profile", "/pjsk adjust profile bg", "/pjsk adjust profile background",
				"/调整个人信息背景", "/调整个人信息", "/设置个人信息", "/设置个人信息背景",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			force := strings.Contains(args, "force")
			// TODO: 迁移 block_region + 背景参数调整逻辑
			return nil, fmt.Errorf("TODO: 调整个人背景未实现，args=%q, force=%t", args, force)
		},
	}
}

func (sekaiHandlers) ProfileUserStatHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk user sta", "/用户统计",
			},
			Disabled: true,
		},
		// TODO: refer 中这里是 CmdHandler（非 SekaiCmdHandler）
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			// TODO: 迁移 superuser 校验 + 用户统计逻辑
			return nil, fmt.Errorf("TODO: 用户统计未实现，args=%q", args)
		},
	}
}

func (sekaiHandlers) ProfileBindHistoryHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk bind history", "/pjsk bind his", "/绑定历史", "/绑定记录",
			},
			Priority: 1,
			Disabled: true,
		},
		// TODO: refer 中这里是 CmdHandler（非 SekaiCmdHandler）
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			// TODO: 迁移 superuser 校验 + bind_history 查询逻辑
			return nil, fmt.Errorf("TODO: 绑定历史查询未实现，args=%q", args)
		},
	}
}

func (sekaiHandlers) ProfileCreateGuestHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk create guest", "/pjsk register", "/pjsk注册",
			},
			Disabled: true,
		},
		Regions: []renderregion.Value{
			renderregion.JP,
			renderregion.EN,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			// TODO: 迁移 create_account + 折叠消息回传逻辑
			return nil, fmt.Errorf("TODO: 注册游客账号未实现，region=%s", string(ctx.region))
		},
	}
}
