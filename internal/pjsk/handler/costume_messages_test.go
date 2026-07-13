package handler

import (
	"errors"
	"testing"

	rendercostume "haruki-cloud/internal/pjsk/render/costume"
)

func TestNormalizeCostume3DError(t *testing.T) {
	tests := map[string]string{
		"3d preview service is not configured":                                                          "当前 Cloud 未开启3D功能",
		"3d preview engine is not configured for region en":                                             "EN服暂未配置3D渲染服务",
		"3d combo role not found; specify a matching unit or different part ids":                        "这些3D部件不属于同一个角色，请检查区服、角色组合和部件ID",
		"3d combo outfit not usable: outfit=934 character3d=1 color=4":                                  "该服装没有对应的角色模型或颜色版本，请检查服装ID、角色ID和颜色ID",
		"3d combo accessory not usable: accessory=11 character3d=2 color=1":                             "该饰品不属于或不适用于这个角色模型及颜色，请检查饰品ID、角色ID和颜色ID",
		"3d combo accessory legacy id: accessory=2003 character3d=2 ids=[2003001 2003017]":              "旧版饰品短ID已拆分为独立饰品（ID：2003001、2003017），请填写完整饰品ID；也可用 /饰品列表 角色ID 查询",
		"3d combo accessory raw id is ambiguous: raw=797009 ids=[797001 797002]":                        "这个原始饰品同时对应多个独立饰品，请改用完整饰品ID",
		"3d preview accessory raw id is ambiguous: raw=797009 ids=[797001 797002]":                      "这个原始饰品同时对应多个独立饰品，请改用完整饰品ID",
		"3d combo hair not usable: hair=9 character3d=23":                                               "该角色没有这个发型ID，请先用 /发型列表 角色ID 查询",
		"3d combo character3d id is duplicated: 23":                                                     "3D角色数据存在重复，请稍后再试",
		"3d combo body part not usable for unit=light_sound: 33001":                                     "服装ID 33001 不能用于当前角色或组合",
		"3d combo hair part not usable for unit=light_sound: 33021":                                     "发型ID 33021 不能用于当前角色或组合",
		"3d combo head/accessory part not usable for unit=light_sound: 53129":                           "饰品ID 53129 不能用于当前角色或组合",
		"3d combo head/hair combination is blocked: unit=light_sound head=1 hair=2":                     "该饰品与发型不兼容，请更换其中一个部件",
		"3d preview registry accessory identity is invalid: accessoryId 797001 maps to sources a and b": "读取3D部件数据失败，请稍后再试",
		"3d preview capture is busy":                                                                    "3D渲染任务繁忙，请稍后再试",
	}
	for input, want := range tests {
		if got := normalizeCostume3DError(errors.New(input)); got.Error() != want {
			t.Errorf("normalizeCostume3DError(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeTypedLegacyAccessoryIDError(t *testing.T) {
	err := &rendercostume.LegacyAccessoryIDError{
		LegacyID:      2003,
		Character3DID: 2,
		AccessoryIDs:  []int{2003001, 2003017},
	}
	want := "旧版饰品短ID已拆分为独立饰品（ID：2003001、2003017），请填写完整饰品ID；也可用 /饰品列表 角色ID 查询"
	if got := normalizeCostume3DError(err).Error(); got != want {
		t.Fatalf("normalize typed legacy error = %q, want %q", got, want)
	}
}
