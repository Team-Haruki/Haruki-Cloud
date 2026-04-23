package handler

import "haruki-cloud/internal/pjsk/requestbuilder"

func toRequestBuilderCommandInput(cmd *CommandRequest) *requestbuilder.CommandInput {
	if cmd == nil {
		return nil
	}
	return &requestbuilder.CommandInput{
		Query:  cmd.Query,
		Region: cmd.Region,
		Params: cmd.Params,
	}
}
