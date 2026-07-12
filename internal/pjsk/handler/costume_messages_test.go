package handler

import (
	"errors"
	"testing"
)

func TestNormalizeCostume3DError(t *testing.T) {
	tests := map[string]string{
		"3d preview service is not configured":                                      "当前 Cloud 未开启3D功能",
		"3d preview engine is not configured for region en":                         "EN服暂未配置3D渲染服务",
		"3d combo role not found; specify a matching unit or different part ids":    "这些3D部件不属于同一个角色，请检查区服、角色组合和部件ID",
		"3d combo outfit not usable: outfit=934 character3d=1 color=4":              "该服装没有对应的角色模型或颜色版本，请检查服装ID、角色ID和颜色ID",
		"3d combo accessory not usable: accessory=11 character3d=2 color=1":         "该饰品不属于或不适用于这个角色模型及颜色，请检查饰品ID、角色ID和颜色ID",
		"3d combo hair not usable: hair=9 character3d=23":                           "该角色没有这个发型ID，请先用 /发型列表 角色ID 查询",
		"3d combo character3d id is duplicated: 23":                                 "3D角色数据存在重复，请稍后再试",
		"3d combo body part not usable for unit=light_sound: 33001":                 "服装ID 33001 不能用于当前角色或组合",
		"3d combo hair part not usable for unit=light_sound: 33021":                 "发型ID 33021 不能用于当前角色或组合",
		"3d combo head/accessory part not usable for unit=light_sound: 53129":       "饰品ID 53129 不能用于当前角色或组合",
		"3d combo head/hair combination is blocked: unit=light_sound head=1 hair=2": "该饰品与发型不兼容，请更换其中一个部件",
		"3d preview capture is busy":                                                "3D渲染任务繁忙，请稍后再试",
	}
	for input, want := range tests {
		if got := normalizeCostume3DError(errors.New(input)); got.Error() != want {
			t.Errorf("normalizeCostume3DError(%q) = %q, want %q", input, got, want)
		}
	}
}
