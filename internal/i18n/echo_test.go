package i18n

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// TestUserInputMessagesHaveEchoFreeForm checks the convention that lets a
// reply leave out user input: every message with a user-input placeholder
// (User…) has an ID+NoEchoSuffix sibling with the same placeholders minus the
// User… ones, and every _no_echo message belongs to such a message.
func TestUserInputMessagesHaveEchoFreeForm(t *testing.T) {
	for _, entry := range Entries(DefaultLocale) {
		if base, ok := strings.CutSuffix(entry.ID, NoEchoSuffix); ok {
			if !HasUserPlaceholder(base) {
				t.Errorf("%s: %s has no base message %s with a user-input placeholder", entry.File, entry.ID, base)
			}
			if slices.ContainsFunc(entry.Placeholders, IsUserPlaceholder) {
				t.Errorf("%s: %s must not have a user-input placeholder: %v", entry.File, entry.ID, entry.Placeholders)
			}
			continue
		}
		if !HasUserPlaceholder(entry.ID) {
			continue
		}
		echoFree, ok := Entry(DefaultLocale, entry.ID+NoEchoSuffix)
		if !ok {
			t.Errorf("%s: %s shows user input but has no %s form", entry.File, entry.ID, entry.ID+NoEchoSuffix)
			continue
		}
		want := slices.DeleteFunc(slices.Clone(entry.Placeholders), IsUserPlaceholder)
		if !slices.Equal(echoFree.Placeholders, want) {
			t.Errorf("%s: %s placeholders %v, want %v (those of %s without User…)", echoFree.File, echoFree.ID, echoFree.Placeholders, want, entry.ID)
		}
	}
}

// emptyQuotePairs are what an echo-free form must never leave behind.
var emptyQuotePairs = []string{"“”", "「」", "\"\"", "：“", "”："}

// TestEchoFreeFormsHideUserInput renders every message that has a user-input
// placeholder with a sentinel value. Without echo the sentinel never appears
// and no empty quotes or dangling colon are left; with echo it is shown.
func TestEchoFreeFormsHideUserInput(t *testing.T) {
	for _, locale := range Locales() {
		for _, entry := range Entries(locale) {
			if !HasUserPlaceholder(entry.ID) {
				continue
			}
			sentinel := "ECHO_SENTINEL_" + entry.ID
			data := Data{}
			for _, name := range entry.Placeholders {
				if IsUserPlaceholder(name) {
					data[name] = UserText(sentinel)
					continue
				}
				data[name] = "‹" + name + "›"
			}
			message := M(entry.ID, data)
			echoed := message.Render(RenderOptions{Locale: locale})
			if !strings.Contains(echoed, sentinel) {
				t.Errorf("%s %s: echo rendering lost the user input: %q", locale, entry.ID, echoed)
			}
			// Nested inside a usage reply as the reply layer builds it.
			for _, reply := range []Message{message, WithUsage(message, "/查曲")} {
				text := reply.Render(RenderOptions{Locale: locale, NoEcho: true})
				if strings.Contains(text, sentinel) {
					t.Errorf("%s %s: echo-free rendering shows the user input: %q", locale, entry.ID, text)
				}
				if text == RequestFailed().In(locale) {
					t.Errorf("%s %s: echo-free rendering fell back to the generic reply", locale, entry.ID)
				}
				for _, pair := range emptyQuotePairs {
					if strings.Contains(text, pair) {
						t.Errorf("%s %s: echo-free rendering leaves %q: %q", locale, entry.ID, pair, text)
					}
				}
				for _, line := range strings.Split(text, "\n") {
					if strings.HasSuffix(strings.TrimSpace(line), "：") {
						t.Errorf("%s %s: echo-free line ends with a colon: %q", locale, entry.ID, line)
					}
				}
			}
		}
	}
}

// A UserText value in a placeholder that is not a user-input one is never
// substituted without echo: the message falls back to the generic reply.
func TestNoEchoNeverSubstitutesUserText(t *testing.T) {
	leaky := M("common.verbatim", Data{"Value": UserText("ECHO_SENTINEL")})
	if got := leaky.Render(RenderOptions{NoEcho: true}); got != RequestFailed().String() {
		t.Fatalf("UserText outside a User… placeholder rendered as %q", got)
	}
	if got := leaky.String(); got != "ECHO_SENTINEL" {
		t.Fatalf("echo rendering = %q", got)
	}
}

func TestBuiltInUserInputHelpers(t *testing.T) {
	const secret = "ECHO_SENTINEL"
	for _, message := range []Message{
		NotFound(M("common.param_name.value"), secret),
		Ambiguous(M("common.param_name.value"), secret),
		BadParam(secret, M("common.param.integer")),
	} {
		if got := message.Render(RenderOptions{NoEcho: true}); strings.Contains(got, secret) {
			t.Errorf("%s without echo = %q", message.ID, got)
		}
		if got := message.String(); !strings.Contains(got, secret) {
			t.Errorf("%s with echo = %q", message.ID, got)
		}
	}
	if got := BadParam(secret, M("common.param.integer")).Render(RenderOptions{NoEcho: true}); got != "参数格式不正确\n"+T("common.param.integer") {
		t.Errorf("bad parameter without echo = %q", got)
	}
}

func TestParamEchoContext(t *testing.T) {
	if ParamEchoFromContext(context.Background()) {
		t.Fatal("parameter echo must be off by default")
	}
	ctx := WithParamEcho(WithLocale(context.Background(), ZhCN), true)
	if opts := RenderOptionsFromContext(ctx); opts.NoEcho || opts.Locale != ZhCN {
		t.Fatalf("options with echo = %+v", opts)
	}
	if opts := RenderOptionsFromContext(WithParamEcho(ctx, false)); !opts.NoEcho {
		t.Fatalf("options without echo = %+v", opts)
	}
}

func TestIsUserPlaceholder(t *testing.T) {
	for name, want := range map[string]bool{"UserQuery": true, "UserUID": true, "User": false, "Username": false, "Query": false, "Users": false} {
		if got := IsUserPlaceholder(name); got != want {
			t.Errorf("IsUserPlaceholder(%q) = %v", name, got)
		}
	}
}
