package sekai

import (
	"errors"
	"fmt"
	"haruki-cloud/internal/pjsk/handler"
)

func (sekaiHandlers) LiveHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{"/pjsk live", "/虚拟live", "/pjsk vlive", "/vlive"},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			// TODO: 迁移最近7天内的 vlive 过滤逻辑
			// TODO: 无活动时返回"当前没有虚拟Live"
			// TODO: 迁移 compose_vlive_list_image 回图逻辑
			if ctx.region == "" {
				return nil, errors.New("无可用区服")
			}
			return nil, fmt.Errorf("TODO: 虚拟Live查询未实现，region=%s", string(ctx.region))
		},
	}
}
