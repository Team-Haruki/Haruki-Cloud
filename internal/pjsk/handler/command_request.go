package handler

import (
	"encoding/json"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
)

type commandExecutor func(*ExecutionRuntime) (onebot11.Message, error)

// CommandRequest is the handler-layer runtime command object. It replaces the
// old resolver-era command object in the PJSK command pipeline.
type CommandRequest struct {
	Module         parser.TargetModule
	Mode           string
	Query          string
	Region         string
	RegionExplicit bool
	Params         json.RawMessage
	IsHelp         bool
	IsVerbose      bool
	IsPreview      bool
	// IsForce asks for fresh renders (--force / 强制刷新); honoured subject to
	// the per-user cooldown in force_render.go.
	IsForce           bool
	CommandPath       string
	TriggerCommand    string
	HelpText          string
	RequesterPlatform string
	RequesterUserID   string
	RequesterGroupID  string
	RequesterBotID    string

	executor commandExecutor
}
