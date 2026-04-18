package requestbuilder

import "encoding/json"

// CommandInput is the minimal command-layer payload needed by requestbuilder
// helpers. It deliberately avoids depending on parser-level command shells so
// the handler pipeline can keep CommandRequest as its only runtime command object.
type CommandInput struct {
	Query  string
	Region string
	Params json.RawMessage
}
