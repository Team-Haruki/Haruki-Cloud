package i18n

import (
	"context"
	"regexp"
	"slices"
	"strconv"
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

// numericSentinel is the number every user-input placeholder gets in the
// numeric pass of TestEchoFreeFormsHideUserInput. IDs, ranks, counts, WL
// turns and UIDs from the user's command are user input as much as text.
const numericSentinel = 987654321

// TestEchoFreeFormsHideUserInput renders every message that has a user-input
// placeholder with sentinel values: a text sentinel, then a numeric one passed
// both as UserNumber and as a plain int (a caller that forgot UserNumber).
// Without echo no sentinel appears and no empty quotes or dangling colon are
// left; with echo the sentinel is shown.
func TestEchoFreeFormsHideUserInput(t *testing.T) {
	type pass struct {
		name     string
		sentinel string
		value    func() any
	}
	for _, locale := range Locales() {
		for _, entry := range Entries(locale) {
			if !HasUserPlaceholder(entry.ID) {
				continue
			}
			textSentinel := "ECHO_SENTINEL_" + entry.ID
			numeric := strconv.Itoa(numericSentinel)
			for _, p := range []pass{
				{"text", textSentinel, func() any { return UserText(textSentinel) }},
				{"UserNumber", numeric, func() any { return UserNumber(numericSentinel) }},
				{"plain int", numeric, func() any { return numericSentinel }},
			} {
				checkEchoFreeForm(t, locale, entry, p.name, p.sentinel, p.value)
			}
		}
	}
}

func checkEchoFreeForm(t *testing.T, locale Locale, entry CatalogEntry, pass, sentinel string, value func() any) {
	t.Helper()
	data := Data{}
	for _, name := range entry.Placeholders {
		if IsUserPlaceholder(name) {
			data[name] = value()
			continue
		}
		data[name] = "‹" + name + "›"
	}
	message := M(entry.ID, data)
	echoed := message.Render(RenderOptions{Locale: locale})
	if !strings.Contains(echoed, sentinel) {
		t.Errorf("%s %s (%s): echo rendering lost the user input: %q", locale, entry.ID, pass, echoed)
	}
	// Nested inside a usage reply as the reply layer builds it.
	for _, reply := range []Message{message, WithUsage(message, "/查曲")} {
		text := reply.Render(RenderOptions{Locale: locale, NoEcho: true})
		if strings.Contains(text, sentinel) {
			t.Errorf("%s %s (%s): echo-free rendering shows the user input: %q", locale, entry.ID, pass, text)
		}
		if text == RequestFailed().In(locale) {
			t.Errorf("%s %s (%s): echo-free rendering fell back to the generic reply", locale, entry.ID, pass)
		}
		for _, pair := range emptyQuotePairs {
			if strings.Contains(text, pair) {
				t.Errorf("%s %s (%s): echo-free rendering leaves %q: %q", locale, entry.ID, pass, pair, text)
			}
		}
		for _, line := range strings.Split(text, "\n") {
			if strings.HasSuffix(strings.TrimSpace(line), "：") {
				t.Errorf("%s %s (%s): echo-free line ends with a colon: %q", locale, entry.ID, pass, line)
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
	leakyNumber := M("common.verbatim", Data{"Value": UserNumber(numericSentinel)})
	if got := leakyNumber.Render(RenderOptions{NoEcho: true}); got != RequestFailed().String() {
		t.Fatalf("UserNumber outside a User… placeholder rendered as %q", got)
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

// userWrittenOutsideEchoRule are placeholders whose description says the
// value is user input but which are not User…: the echo rule covers error
// replies and unreviewed alias text, not these success replies, image labels
// and moderation notices.
var userWrittenOutsideEchoRule = map[string]string{
	"account.swap.done.Left":                     "success reply",
	"account.swap.done.Right":                    "success reply",
	"account.swap.done_region.Left":              "success reply",
	"account.swap.done_region.Right":             "success reply",
	"alias.record.rejected.Reason":               "the admin's own reason in a success reply",
	"deck.summary.wl_character.Character":        "success summary",
	"deck.summary.leader.Character":              "success summary",
	"deck.summary.challenge_character.Character": "success summary",
	"moderation.kill.done_permanent.Reason":      "the admin's own reason in a success reply",
	"moderation.kill.done_until.Reason":          "the admin's own reason in a success reply",
	"moderation.banned_reason.Reason":            "ban notice written by an admin",
	"moderation.banned_reason_until.Reason":      "ban notice written by an admin",
	"music.lookup_list.bpm.BPM":                  "image title of a successful lookup",
	"music.image.board.strategy.Strategy":        "image label",
}

var userWrittenDescription = regexp.MustCompile(`用户写|用户输入|管理员输入|用户提交`)

// TestUserWrittenPlaceholdersAreUserInput catches a new message whose
// description says a placeholder holds what the user (or admin) typed, text
// or number, without naming it User…, so it would be echoed without
// parameter echo.
func TestUserWrittenPlaceholdersAreUserInput(t *testing.T) {
	seen := map[string]bool{}
	for _, entry := range Entries(DefaultLocale) {
		_, placeholders, _ := strings.Cut(entry.Description, "占位符：")
		for _, item := range strings.Split(strings.TrimSuffix(placeholders, "。"), "；") {
			name, meaning, ok := strings.Cut(item, "=")
			if !ok || IsUserPlaceholder(name) || !userWrittenDescription.MatchString(meaning) {
				continue
			}
			key := entry.ID + "." + name
			seen[key] = true
			if _, allowed := userWrittenOutsideEchoRule[key]; !allowed {
				t.Errorf("%s: %s is user input (%q); name it User… and add %s%s", entry.File, key, meaning, entry.ID, NoEchoSuffix)
			}
		}
	}
	for key := range userWrittenOutsideEchoRule {
		if !seen[key] {
			t.Errorf("userWrittenOutsideEchoRule: %s no longer matches a catalog placeholder; remove it", key)
		}
	}
}
