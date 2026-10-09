package parser

import (
	"strconv"
	"strings"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/utils/usererror"
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
	if args == "unbind" {
		cmd.Type = CmdTypeUnbind
		return cmd, nil
	}
	for _, parse := range []func(string, *EventCommand) (bool, error){
		parseBindCommand,
		parseMentionCommand,
		parseRankRangeCommand,
		parseMultiRankCommand,
		parseNumericCommand,
	} {
		matched, err := parse(args, cmd)
		if err != nil {
			return nil, err
		}
		if matched {
			return cmd, nil
		}
	}
	return nil, usererror.Unrecognized()
}

func parseBindCommand(args string, cmd *EventCommand) (bool, error) {
	if !strings.HasPrefix(args, "bind ") {
		return false, nil
	}
	cmd.Type = CmdTypeBind
	cmd.TargetID = strings.TrimSpace(strings.TrimPrefix(args, "bind "))
	if !isNumeric(cmd.TargetID) || len(cmd.TargetID) < 10 {
		return true, usererror.BadParam(cmd.TargetID, i18n.M("common.param.uid_invalid"))
	}
	return true, nil
}

func parseMentionCommand(args string, cmd *EventCommand) (bool, error) {
	if !strings.HasPrefix(args, "@") {
		return false, nil
	}
	cmd.Type = CmdTypeEventQueryAt
	cmd.TargetID = strings.TrimPrefix(args, "@")
	if !isNumeric(cmd.TargetID) {
		return true, usererror.BadParam("@"+cmd.TargetID, i18n.M("common.param.mention_invalid"))
	}
	return true, nil
}

func parseRankRangeCommand(args string, cmd *EventCommand) (bool, error) {
	if !strings.Contains(args, "-") || strings.HasPrefix(args, "-") {
		return false, nil
	}
	parts := strings.Split(args, "-")
	if len(parts) != 2 {
		return false, nil
	}
	start, startErr := strconv.Atoi(strings.TrimSpace(parts[0]))
	end, endErr := strconv.Atoi(strings.TrimSpace(parts[1]))
	if startErr != nil || endErr != nil {
		return false, nil
	}
	if start > end {
		return true, usererror.Invalid(i18n.M("common.param.rank_range_order"))
	}
	cmd.Type = CmdTypeEventQueryRankRange
	cmd.Param1 = start
	cmd.Param2 = end
	return true, nil
}

func parseMultiRankCommand(args string, cmd *EventCommand) (bool, error) {
	fields := strings.Fields(args)
	if len(fields) <= 1 {
		return false, nil
	}
	ranks := make([]int, 0, len(fields))
	for _, field := range fields {
		rank, err := strconv.Atoi(field)
		if err != nil {
			return true, usererror.BadParam(field, i18n.M("common.param.rank_invalid"))
		}
		ranks = append(ranks, rank)
	}
	cmd.Type = CmdTypeEventQueryMultiRank
	cmd.MultiArgs = ranks
	return true, nil
}

func parseNumericCommand(args string, cmd *EventCommand) (bool, error) {
	if !isNumeric(args) {
		return false, nil
	}
	value, _ := strconv.Atoi(args)
	if len(args) >= 10 {
		cmd.Type = CmdTypeEventQueryUID
		cmd.TargetID = args
	} else {
		cmd.Type = CmdTypeEventQueryRank
		cmd.Param1 = value
	}
	return true, nil
}
