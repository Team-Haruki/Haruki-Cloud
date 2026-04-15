package handler

import (
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	"haruki-cloud/internal/pjsk/drawing"

	accountdata "haruki-cloud/internal/pjsk/userdata"
)

// userQueryParams mirrors sekai.UserQueryParams for bridge-side decoding.
type userQueryParams struct {
	Mode            string `json:"mode"`
	Platform        string `json:"platform"`
	PlatformUserID  string `json:"platform_user_id"`
	AtUserID        string `json:"at_user_id"`
	PJSKUserID      string `json:"pjsk_user_id"`
	Selector        string `json:"selector,omitempty"`
	ProfileVertical *bool  `json:"profile_vertical,omitempty"`
}

type resolvedGameTarget struct {
	HarukiUserID int
	PJSKUserID   string
	Visible      bool
	BgSettings   *drawing.ProfileBgSettings
	Binding      *accountdata.ResolvedBinding
}

type mySekaiRenderContext struct {
	Controller *rendermysekai.Controller
	Profile    *drawing.ProfileCardRequest
	Region     string
}
