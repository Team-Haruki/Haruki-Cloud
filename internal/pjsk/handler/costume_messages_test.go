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
		"3d combo matches multiple roles; specify unit or another part id":          "这些部件可匹配多个角色版本，请补充角色组合（例如 ln、mmj、vbs、ws、n25）",
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
