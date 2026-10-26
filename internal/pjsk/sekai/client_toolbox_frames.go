package sekai

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"haruki-cloud/internal/core/upstreamerr"
	json "haruki-cloud/internal/jsonutil"
)

type SponsorFrameParts struct {
	Base        string `json:"base"`
	CenterTop   string `json:"centertop"`
	LeftTop     string `json:"lefttop"`
	RightTop    string `json:"righttop"`
	LeftBottom  string `json:"leftbottom"`
	RightBottom string `json:"rightbottom"`
}

type SponsorFrame struct {
	ID         string            `json:"id"`
	Horizontal SponsorFrameParts `json:"horizontal"`
	Vertical   SponsorFrameParts `json:"vertical"`
}

// GetSponsorFrameContext asks Toolbox for the verified game owner's selected
// decoration. The endpoint returns no owner identity or sponsorship details.
// Older Toolbox deployments may return 404; that means no custom frame.
func (c *HarukiToolboxClient) GetSponsorFrameContext(ctx context.Context, server, uid string) (*SponsorFrame, error) {
	r, err := c.internalRequest(ctx)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(c.config.BaseURL, "/") + "/api/private/game-frame/" + url.PathEscape(server) + "/" + url.PathEscape(uid)
	resp, err := r.Get(endpoint)
	if err != nil {
		return nil, sanitizeNetworkError(upstreamerr.ServiceToolbox, "toolbox frame request failed", err)
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, &ToolboxAPIError{StatusCode: resp.StatusCode(), Message: "toolbox frame request failed"}
	}
	var result struct {
		Frame *SponsorFrame `json:"frame"`
	}
	if len(resp.Body()) > 16*1024 {
		return nil, fmt.Errorf("toolbox frame response too large")
	}
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("invalid toolbox frame response: %w", err)
	}
	return result.Frame, nil
}
