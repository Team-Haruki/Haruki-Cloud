package handler

import (
	"errors"
	"fmt"
	"strings"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/core/upstreamerr"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/render/cachefill"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/utils/usererror"
)

// Private data kinds, as passed to the binding-aware error builders.
const (
	privateDataSuite   = "suite"
	privateDataMySekai = "mysekai"
)

// unsupportedModeError reports a mode with no executor: a registration bug,
// shown to users as the generic reply.
func unsupportedModeError(module, mode string) error {
	return fmt.Errorf("bridge: unsupported %s mode %q", module, mode)
}

// normalizeBindingLookupError turns a binding lookup failure into a typed
// user error. notBound is the reply when the looked-up user has no binding
// (zero: the requester's own "please bind first" reply).
func normalizeBindingLookupError(err error, notBound i18n.Message) error {
	switch {
	case err == nil:
		return nil
	case isUserFacingError(err):
		return err
	case errors.Is(err, accountdata.ErrNoBinding):
		if notBound.IsZero() || useTempBindingNotice() {
			return bindingRequiredError(err)
		}
		return usererror.Wrap(usererror.CodeNotFound, notBound, err)
	default:
		return bindingServiceUnavailableError(err)
	}
}

func useTempBindingNotice() bool {
	return harukiConfig.Cfg.Profile.IsTemp()
}

// bindingRequiredError is the reply when the requester has no usable binding.
func bindingRequiredError(cause error) error {
	if useTempBindingNotice() {
		return usererror.Wrap(usererror.CodeSetup, i18n.M("binding.temp_environment"), cause)
	}
	return usererror.Wrap(usererror.CodeSetup, i18n.M("binding.required"), cause)
}

func bindingServiceUnavailableError(cause error) error {
	if useTempBindingNotice() {
		return usererror.Wrap(usererror.CodeSetup, i18n.M("binding.temp_environment"), cause)
	}
	return usererror.Unavailable(i18n.FeatureAccount, cause)
}

func normalizePrivateDataKind(kind string) string {
	if strings.EqualFold(strings.TrimSpace(kind), privateDataMySekai) {
		return privateDataMySekai
	}
	return privateDataSuite
}

func privateDataLabel(kind string) i18n.Message {
	if normalizePrivateDataKind(kind) == privateDataMySekai {
		return i18n.M("binding.data_kind.mysekai")
	}
	return i18n.M("binding.data_kind.suite")
}

func toolboxLink() i18n.Message { return i18n.M("binding.toolbox_link") }

// bindingAccountLabel is the display name of binding's account, e.g.
// "[日服(JP)] 123***789"; ok is false when the binding names no account.
func bindingAccountLabel(binding *accountdata.ResolvedBinding) (i18n.Message, bool) {
	if binding == nil || strings.TrimSpace(binding.PJSKUserID) == "" {
		return i18n.Message{}, false
	}
	return i18n.AccountLabel(binding.Server, binding.PJSKUserID, binding.Visible), true
}

// privateDataError is the reply when binding has no usable data of kind:
// hidden by the user, or not uploaded.
func privateDataError(kind string, binding *accountdata.ResolvedBinding) error {
	kind = normalizePrivateDataKind(kind)
	if binding != nil {
		hidden := !binding.SuiteVisible
		if kind == privateDataMySekai {
			hidden = !binding.MySekaiVisible
		}
		if hidden {
			return usererror.Setup(privateDataHiddenMessage(kind, binding))
		}
	}
	return usererror.Setup(privateDataNotFoundMessage(kind, binding))
}

func suiteDataNotFoundError(binding *accountdata.ResolvedBinding) error {
	return privateDataError(privateDataSuite, binding)
}

func mysekaiDataNotFoundError(binding *accountdata.ResolvedBinding) error {
	return privateDataError(privateDataMySekai, binding)
}

func privateDataHiddenMessage(kind string, binding *accountdata.ResolvedBinding) i18n.Message {
	account, ok := bindingAccountLabel(binding)
	if !ok {
		account = i18n.M("binding.current_account")
	}
	if normalizePrivateDataKind(kind) == privateDataMySekai {
		return i18n.M("binding.data.hidden_mysekai", i18n.Data{"Account": account})
	}
	return i18n.M("binding.data.hidden_suite", i18n.Data{"Account": account})
}

func privateDataNotFoundMessage(kind string, binding *accountdata.ResolvedBinding) i18n.Message {
	if account, ok := bindingAccountLabel(binding); ok && strings.TrimSpace(binding.Server) != "" {
		return i18n.M("binding.data.not_found_account", i18n.Data{"Account": account, "Data": privateDataLabel(kind), "ToolboxLink": toolboxLink()})
	}
	return i18n.M("binding.data.not_found", i18n.Data{"Data": privateDataLabel(kind), "ToolboxLink": toolboxLink()})
}

func toolboxAccessDeniedMessage(kind string, binding *accountdata.ResolvedBinding) i18n.Message {
	if account, ok := bindingAccountLabel(binding); ok {
		return i18n.M("binding.toolbox.access_denied_account", i18n.Data{"Account": account, "Data": privateDataLabel(kind), "ToolboxLink": toolboxLink()})
	}
	return i18n.M("binding.toolbox.access_denied", i18n.Data{"Data": privateDataLabel(kind), "ToolboxLink": toolboxLink()})
}

// normalizeToolboxDataFetchError turns a failure to fetch kind data
// ("suite" or "mysekai") of binding from the Toolbox into a typed user
// error naming the data and, when known, the account.
func normalizeToolboxDataFetchError(err error, kind string, binding *accountdata.ResolvedBinding) error {
	if err == nil {
		return nil
	}
	if isUserFacingError(err) {
		return err
	}
	class, ok := upstreamerr.Classify(err)
	if !ok {
		return usererror.Unavailable(i18n.FeatureToolbox, err)
	}
	switch class.Kind {
	case upstreamerr.KindAccountNotBound:
		if useTempBindingNotice() {
			return bindingRequiredError(err)
		}
		return usererror.Wrap(usererror.CodeSetup, i18n.M("binding.toolbox.not_bound", i18n.Data{"Data": privateDataLabel(kind), "ToolboxLink": toolboxLink()}), err)
	case upstreamerr.KindDataNotUploaded:
		return withCause(privateDataError(kind, binding), err)
	case upstreamerr.KindAccessDenied:
		if useTempBindingNotice() {
			return bindingRequiredError(err)
		}
		return usererror.Wrap(usererror.CodeSetup, toolboxAccessDeniedMessage(kind, binding), err)
	case upstreamerr.KindOwnerBanned:
		return usererror.Wrap(usererror.CodeForbidden, i18n.M("binding.toolbox.owner_banned", i18n.Data{"Data": privateDataLabel(kind)}), err)
	}
	return upstreamerr.UserError(err)
}

// withCause attaches cause to a typed user error for the logs.
func withCause(err error, cause error) error {
	if typed, ok := usererror.As(err); ok && typed.Cause == nil {
		return typed.WithCause(cause)
	}
	return err
}

// normalizeSekaiAPIFetchError turns a game data service failure into a
// typed user error; other errors pass through.
func normalizeSekaiAPIFetchError(err error) error {
	if err == nil || isUserFacingError(err) {
		return err
	}
	if typed := upstreamerr.UserError(err); typed != nil {
		return typed
	}
	return err
}

// WrapDomainError turns the well-known domain failures (no binding, account
// storage down, master data cache down, upstream failures) into typed user
// errors, so the transport layer only distinguishes typed errors from
// unexpected ones. Classification is by type (errors.Is / errors.As), never
// by message text.
func WrapDomainError(err error) error {
	switch {
	case err == nil:
		return nil
	case isUserFacingError(err):
		return err
	case errors.Is(err, accountdata.ErrNoBinding):
		return bindingRequiredError(err)
	case errors.Is(err, accountdata.ErrBindingServiceUnavailable):
		return bindingServiceUnavailableError(err)
	case errors.Is(err, cachefill.ErrUnavailable):
		return usererror.Unavailable(i18n.FeatureGameData, err)
	case errors.Is(err, rendersnapshot.ErrNotConfigured):
		return withCause(suiteDataNotFoundError(nil), err)
	case errors.Is(err, rendersnapshot.ErrMySekaiUnavailable):
		return withCause(mysekaiDataNotFoundError(nil), err)
	}
	if class, ok := upstreamerr.Classify(err); ok && class.Service == upstreamerr.ServiceToolbox {
		return normalizeToolboxDataFetchError(err, privateDataSuite, nil)
	}
	if typed := upstreamerr.UserError(err); typed != nil {
		return typed
	}
	return err
}

func stringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
