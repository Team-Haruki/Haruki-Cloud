package handler

import (
	"context"
	"fmt"
	"strings"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/displaytime"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
)

// ExecutionRuntime captures the request-scoped runtime prepared before a PJSK
// command enters its domain-specific executor.
type ExecutionRuntime struct {
	Context  context.Context
	Resolved *CommandRequest
	App      *renderapp.App
	Request  *RequestContext
}

// PrepareExecutionRuntime centralizes the request-level preflight shared by the
// current command execution pipeline. Future direct command execution can
// reuse the same requester-ban, region, timezone, and RequestContext
// preparation logic.
//
// When a short-circuit response should be returned immediately (for example the
// requester is banned), shortCircuit is non-nil and runtime is nil.
func PrepareExecutionRuntime(ctx context.Context, resolved *CommandRequest, app *renderapp.App) (runtime *ExecutionRuntime, shortCircuit onebot11.Message, err error) {
	if resolved == nil {
		return nil, nil, fmt.Errorf("bridge: nil resolved command")
	}
	if app == nil {
		return nil, nil, fmt.Errorf("bridge: nil render app")
	}

	if platform := strings.TrimSpace(resolved.RequesterPlatform); platform != "" {
		if userID := strings.TrimSpace(resolved.RequesterUserID); userID != "" {
			if err := app.BanChecker.CheckBan(ctx, platform, userID, resolved.Module); err != nil {
				return nil, onebot11.Message{onebot11.Text(err.Error())}, nil
			}
		}
	}

	resolved.Region = resolveRegionFromDefaultBinding(ctx, resolved, app)
	ctx = displaytime.WithRequestTimeZone(
		ctx,
		resolveRequesterHarukiUserTimeZone(ctx, app, resolved.RequesterPlatform, resolved.RequesterUserID),
	)
	ctx = rendersnapshot.WithRequestCache(ctx)

	return &ExecutionRuntime{
		Context:  ctx,
		Resolved: resolved,
		App:      app,
		Request:  NewRequestContext(ctx, resolved, app),
	}, nil, nil
}
