package parser

import (
	"fmt"
	"strconv"
	"strings"
)

func NewCommandParser() *CommandParser {
	return &CommandParser{}
}

// Parse 解析指令字符串
func (p *CommandParser) Parse(args string) (*EventCommand, error) {
	args = strings.TrimSpace(args)
	cmd := &EventCommand{Original: args}

	if args == "" {
		cmd.Type = CmdTypeEventQuerySelf
		return cmd, nil
	}

	if strings.HasPrefix(args, "bind ") {
		cmd.Type = CmdTypeBind
		cmd.TargetID = strings.TrimSpace(strings.TrimPrefix(args, "bind "))
		if !isNumeric(cmd.TargetID) || len(cmd.TargetID) < 10 {
			return nil, fmt.Errorf("无效的游戏ID: %s", cmd.TargetID)
		}
		return cmd, nil
	}
	if args == "unbind" {
		cmd.Type = CmdTypeUnbind
		return cmd, nil
	}

	if strings.HasPrefix(args, "@") {
		cmd.Type = CmdTypeEventQueryAt
		cmd.TargetID = strings.TrimPrefix(args, "@")
		if !isNumeric(cmd.TargetID) {
			return nil, fmt.Errorf("无效的用户ID: %s", cmd.TargetID)
		}
		return cmd, nil
	}

	if strings.Contains(args, "-") && !strings.HasPrefix(args, "-") {
		parts := strings.Split(args, "-")
		if len(parts) == 2 {
			start, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
			end, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err1 == nil && err2 == nil {
				if start > end {
					return nil, fmt.Errorf("起始排名不能大于结束排名")
				}
				cmd.Type = CmdTypeEventQueryRankRange
				cmd.Param1 = start
				cmd.Param2 = end
				return cmd, nil
			}
		}
	}

	fields := strings.Fields(args)
	if len(fields) > 1 {
		var ranks []int
		for _, f := range fields {
			if r, err := strconv.Atoi(f); err == nil {
				ranks = append(ranks, r)
			} else {
				return nil, fmt.Errorf("无法解析的排名参数: %s", f)
			}
		}
		cmd.Type = CmdTypeEventQueryMultiRank
		cmd.MultiArgs = ranks
		return cmd, nil
	}

	if isNumeric(args) {
		val, _ := strconv.Atoi(args)
		if len(args) >= 10 {
			cmd.Type = CmdTypeEventQueryUID
			cmd.TargetID = args
		} else {
			cmd.Type = CmdTypeEventQueryRank
			cmd.Param1 = val
		}
		return cmd, nil
	}

	return nil, fmt.Errorf("无法识别的指令格式: %s", args)
}
