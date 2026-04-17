package handler

import (
	"fmt"

	pjskalias "haruki-cloud/internal/pjsk/alias"
	"haruki-cloud/internal/pjsk/onebot11"
)

func executeAlias(rc *RequestContext) (onebot11.Message, error) {
	if rc.App == nil || rc.App.Aliases == nil {
		return nil, fmt.Errorf("别名服务未就绪，请稍后再试")
	}
	data, err := pjskalias.ExecuteCommand(rc.Ctx, rc.App.Aliases, rc.Cmd.Mode, rc.Cmd.Params)
	if err != nil {
		return nil, err
	}
	return onebot11.Message{onebot11.Text(string(data))}, nil
}
