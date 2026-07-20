package accountdata

import (
	"context"
	"errors"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

// GlobalDefaultBindingScope is the scope key used for the global default binding.
const GlobalDefaultBindingScope = "default"

// ErrBindingServiceUnavailable is returned when the binding service is not properly configured.
var ErrBindingServiceUnavailable = errors.New("pjsk: binding service is not configured")

// AllBindingServers contains all supported PJSK server regions for binding.
var AllBindingServers = []renderregion.Value{
	renderregion.JP,
	renderregion.CN,
	renderregion.TW,
	renderregion.KR,
	renderregion.EN,
}

// IdentityResolver resolves platform user IDs to internal Haruki user IDs.
type IdentityResolver interface {
	ResolveOrCreate(ctx context.Context, platform, userID string) (int, error)
}

// ProfileValidator validates user profiles against the game server.
type ProfileValidator interface {
	GetUserProfile(server, userID string) (*sekaiapi.GetAnotherProfileResponse, error)
}

type contextProfileValidator interface {
	GetUserProfileContext(ctx context.Context, server, userID string) (*sekaiapi.GetAnotherProfileResponse, error)
}

// FastVerificationProvider provides fast verification of game account bindings.
type FastVerificationProvider interface {
	GetToolboxUserFastVerificationGameAccountBindings(platform, platformUserID string) ([]sekaiapi.UserGameBinding, error)
}

type contextFastVerificationProvider interface {
	GetToolboxUserFastVerificationGameAccountBindingsContext(ctx context.Context, platform, platformUserID string) ([]sekaiapi.UserGameBinding, error)
}

// ProfileBGStorage handles storage of custom profile background images.
type ProfileBGStorage interface {
	SaveProfileBackground(ctx context.Context, server string, userID string, imageURL string) (*drawing.ProfileBgSettings, error)
	DeleteProfileBackground(ctx context.Context, settings *drawing.ProfileBgSettings) error
}

// DefaultScope indicates whether a default binding is global or server-specific.
type DefaultScope string

const (
	DefaultScopeGlobal DefaultScope = "global"
	DefaultScopeServer DefaultScope = "server"
)

// BindResult is returned after a successful bind operation.
type BindResult struct {
	Server              string
	UserID              string
	UserName            string
	AlreadyBound        bool
	SetGlobalDefault    bool
	SetServerDefault    bool
	MultipleServerMatch bool
}

// BindingListItem represents a single binding in the user's binding list.
type BindingListItem struct {
	Index           int
	BindingID       int
	DisplayOrder    int
	Server          string
	UserID          string
	Visible         bool
	SuiteVisible    bool
	MySekaiVisible  bool
	Verified        bool
	Bg              *drawing.ProfileBgSettings
	IsGlobalDefault bool
	IsServerDefault bool
}

// UnbindResult is returned after a successful unbind operation.
type UnbindResult struct {
	Removed          BindingListItem
	ReassignedGlobal *BindingListItem
	ReassignedServer *BindingListItem
}

// DefaultBindingResult is returned after setting or clearing a default binding.
type DefaultBindingResult struct {
	Scope   DefaultScope
	Server  string
	Binding BindingListItem
}

// profileProbe holds the result of probing a UID on a specific server.
type profileProbe struct {
	Server   string
	UserID   string
	UserName string
}
