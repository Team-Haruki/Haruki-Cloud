package handler

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	corehandler "haruki-cloud/internal/handler"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
)

// commandHelpMessage answers "<command> -help": the route's help document,
// completed with the generated region and alias sections, rendered as an
// image, or sent as plain text when the render service cannot be used.
func commandHelpMessage(ctx context.Context, resolved *CommandRequest, app *renderapp.App) (onebot11.Message, error) {
	finishBuild := measurePayloadBuild(ctx)
	defer finishBuild()
	locale := i18n.LocaleFromContext(ctx)
	markdown, err := commandHelpMarkdownIn(locale, resolved)
	if err != nil {
		return nil, err
	}
	if app == nil || app.Drawing == nil {
		return commandHelpTextMessage(locale, markdown), nil
	}
	request := &drawing.CommandHelpRenderRequest{
		Path:     commandHelpRequestPath(resolved),
		Title:    commandHelpTitle(locale, markdown),
		Markdown: markdown,
	}
	finishBuild()
	image, err := app.Drawing.WithContext(ctx).GenerateCommandHelpImage(request)
	if err != nil {
		return commandHelpTextMessage(locale, markdown), nil
	}
	if app.ImageCache != nil || (image.Ref() != nil && app.ImageHosts.Len() > 0) {
		message, err := renderedImageMessage(ctx, image, app)
		if err != nil {
			return commandHelpTextMessage(locale, markdown), nil
		}
		return message, nil
	}
	data, err := image.Bytes(ctx)
	if err != nil {
		return commandHelpTextMessage(locale, markdown), nil
	}
	return inlineImageMessage(ctx, data), nil
}

// commandHelpTextMessage is the text fallback of a help image: the same
// document with its Markdown markup removed, so chat users never see "#",
// "`" or "**".
func commandHelpTextMessage(locale i18n.Locale, markdown string) onebot11.Message {
	return onebot11.Message{onebot11.Text(commandHelpPlainText(locale, markdown))}
}

// commandHelpMarkdown is the complete help document of a command in the
// default locale.
func commandHelpMarkdown(resolved *CommandRequest) (string, error) {
	return commandHelpMarkdownIn(i18n.DefaultLocale, resolved)
}

// commandHelpMarkdownIn returns the route's help document followed by the
// generated sections. Every registered route has its own document
// (TestEveryRouteHasHelpDoc); the generic document only answers a request
// without a route.
func commandHelpMarkdownIn(locale i18n.Locale, resolved *CommandRequest) (string, error) {
	path := commandHelpRequestPath(resolved)
	for _, key := range commandHelpLookupKeys(path) {
		md, ok, err := i18n.HelpDoc(locale, key)
		if err != nil {
			return "", err
		}
		if ok {
			return withGeneratedHelpSections(locale, md, path), nil
		}
	}
	return "", fmt.Errorf("command help markdown not found: path=%s", path)
}

func commandHelpRequestPath(resolved *CommandRequest) string {
	if resolved == nil {
		return ""
	}
	path := strings.Trim(strings.TrimSpace(resolved.CommandPath), "/")
	trigger := normalizeCommandHelpTrigger(resolved.TriggerCommand)
	if path == "mysekai/talk-list" && isMysekaiBlueprintHelpTrigger(trigger) {
		return mysekaiBlueprintHelpPath
	}
	return path
}

func normalizeCommandHelpTrigger(trigger string) string {
	trigger = strings.TrimSpace(trigger)
	if trigger == "" {
		return ""
	}
	lower := strings.ToLower(trigger)
	for _, region := range i18n.RegionCodes {
		prefix := "/" + region
		if !strings.HasPrefix(lower, prefix) {
			continue
		}
		if len(trigger) == len(prefix) {
			continue
		}
		return "/" + strings.TrimPrefix(trigger[len(prefix):], "/")
	}
	return trigger
}

// mysekaiBlueprintHelpTriggers are the mysekai/talk-list spellings that ask
// for the blueprint list; their help is the blueprint document.
var mysekaiBlueprintHelpTriggers = []string{
	"/msb", "/mysekai blueprint", "/mysekai 蓝图", "/pjsk mysekai blueprint", "/烤森蓝图", //copylint:ignore command triggers
}

func isMysekaiBlueprintHelpTrigger(trigger string) bool {
	return slices.Contains(mysekaiBlueprintHelpTriggers, trigger)
}

// mysekaiBlueprintHelpPath is the help document of the blueprint spellings of
// mysekai/talk-list; it is not a route of its own.
const mysekaiBlueprintHelpPath = "mysekai/blueprint"

// commandHelpRoutePath is the registered route a help document describes.
func commandHelpRoutePath(path string) string {
	if path == mysekaiBlueprintHelpPath {
		return "mysekai/talk-list"
	}
	return path
}

// commandHelpShowsAlias reports whether the help document of path lists the
// route spelling alias: the blueprint document owns the blueprint spellings
// and the talk-list document the others.
func commandHelpShowsAlias(path, alias string) bool {
	switch path {
	case mysekaiBlueprintHelpPath:
		return isMysekaiBlueprintHelpTrigger(alias)
	case "mysekai/talk-list":
		return !isMysekaiBlueprintHelpTrigger(alias)
	default:
		return true
	}
}

// commandHelpLookupKeys lists the help documents tried for a route: its own
// document, or the generic one for a request without a route.
func commandHelpLookupKeys(path string) []string {
	if key := commandHelpDocKey(path); key != "" {
		return []string{key}
	}
	return []string{commandHelpGenericKey}
}

const commandHelpGenericKey = "generic"

func commandHelpDocKey(path string) string {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return ""
	}
	return strings.ReplaceAll(path, "/", "_")
}

func readCommandHelpMarkdown(key string) (string, bool, error) {
	key = commandHelpDocKey(key)
	if key == "" {
		return "", false, nil
	}
	return i18n.HelpDoc(i18n.DefaultLocale, key)
}

// commandHelpTitle is the document's first heading, or the generic help
// title for a document without one.
func commandHelpTitle(locale i18n.Locale, markdown string) string {
	for _, line := range strings.Split(markdown, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			if title := strings.TrimSpace(strings.TrimLeft(line, "#")); title != "" {
				return title
			}
		}
	}
	return i18n.M("usage.help.title").In(locale)
}

// withGeneratedHelpSections appends the sections every route document shares
// and that are therefore never written by hand: which regions the command
// serves (once, instead of a copied sentence per document) and the aliases
// the document does not already show.
func withGeneratedHelpSections(locale i18n.Locale, markdown, path string) string {
	sections := []string{strings.TrimSpace(markdown)}
	if region := commandHelpRegionSection(locale, markdown, path); region != "" {
		sections = append(sections, region)
	}
	if aliases := commandHelpAliasSection(locale, markdown, path); aliases != "" {
		sections = append(sections, aliases)
	}
	return strings.Join(sections, "\n\n")
}

// regionAgnosticHelpRoutes are routes whose result does not depend on a
// region (account-wide settings, alias review, moderation). Their region
// prefix is accepted but meaningless, so their help leaves it out.
var regionAgnosticHelpRoutes = []string{
	"admin/",
	"alias/",
	"profile/arrest-difficulty",
	"profile/bind",
	"profile/chart-style",
	"profile/modular/",
	"profile/timezone",
}

func isRegionAgnosticHelpRoute(path string) bool {
	for _, entry := range regionAgnosticHelpRoutes {
		if path == strings.TrimSuffix(entry, "/") || (strings.HasSuffix(entry, "/") && strings.HasPrefix(path, entry)) {
			return true
		}
	}
	return false
}

// commandHelpRegionSection explains the region prefix for path, generated
// from the regions its handler accepts.
func commandHelpRegionSection(locale i18n.Locale, markdown, path string) string {
	if path == "" || isRegionAgnosticHelpRoute(path) {
		return ""
	}
	primary, handler := commandHelpPrimaryHandler(markdown, path)
	if handler == nil || len(handler.Regions) == 0 {
		return ""
	}
	heading := "## " + i18n.M("usage.help.section_region").In(locale)
	if len(handler.Regions) == 1 {
		only := i18n.M("usage.help.region_only", i18n.Data{"Region": i18n.RegionLabel(string(handler.Regions[0]))})
		return heading + "\n- " + only.In(locale)
	}
	codes := make([]string, 0, len(handler.Regions))
	for _, code := range i18n.RegionCodes {
		if slices.Contains(handler.Regions, renderregion.Value(code)) {
			codes = append(codes, "`"+code+"`")
		}
	}
	example := "`/" + string(handler.Regions[0]) + strings.TrimPrefix(primary, "/") + "`"
	prefix := i18n.M("usage.help.region_prefix", i18n.Data{"Codes": strings.Join(codes, " "), "Example": example})
	fallback := i18n.M("usage.help.region_default", i18n.Data{"Region": i18n.RegionLabel(string(handler.Regions[0]))})
	return heading + "\n- " + prefix.In(locale) + "\n- " + fallback.In(locale)
}

// commandHelpPrimaryHandler returns the route's primary command and its
// handler: the first command of the route the document shows, else the
// route's shortest Chinese spelling.
func commandHelpPrimaryHandler(markdown, path string) (string, *HarukiSekaiCommandHandler) {
	candidates := make([]string, 0)
	for _, span := range helpDocCodeSpans(markdown) {
		if command := helpDocSpanCommand(span); command != "" {
			candidates = append(candidates, command)
		}
	}
	aliases := commandHelpAliases(path)
	for _, alias := range aliases {
		if strings.ContainsFunc(alias, isHanRune) {
			candidates = append(candidates, alias)
		}
	}
	candidates = append(candidates, aliases...)
	for _, command := range candidates {
		matched, ok := corehandler.LookupCommandHandler(command)
		if !ok {
			continue
		}
		handler, ok := matched.Handler.(*HarukiSekaiCommandHandler)
		if ok && strings.Trim(handler.Path, "/") == commandHelpRoutePath(path) {
			return command, handler
		}
	}
	return "", nil
}

func commandHelpAliasSection(locale i18n.Locale, markdown, path string) string {
	aliases := missingCommandHelpAliases(markdown, path)
	if len(aliases) == 0 {
		return ""
	}
	lines := make([]string, 0, (len(aliases)+3)/4)
	for len(aliases) > 0 {
		n := min(len(aliases), 4)
		items := make([]string, 0, n)
		for _, alias := range aliases[:n] {
			items = append(items, "`"+alias+"`")
		}
		lines = append(lines, "- "+strings.Join(items, " "))
		aliases = aliases[n:]
	}
	return "## " + i18n.M("usage.help.section_aliases").In(locale) + "\n" + strings.Join(lines, "\n")
}

func missingCommandHelpAliases(markdown string, path string) []string {
	aliases := commandHelpAliases(path)
	if len(aliases) == 0 {
		return nil
	}
	shown := map[string]bool{}
	for _, span := range helpDocCodeSpans(markdown) {
		if command := helpDocSpanCommand(span); command != "" {
			shown[strings.ToLower(command)] = true
		}
	}
	missing := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		if !shown[strings.ToLower(alias)] {
			missing = append(missing, alias)
		}
	}
	return missing
}

// commandHelpAliases lists the spellings of a route as users type them,
// without region prefixes and without the variants an extra prefix such as
// "wl" produces (the document explains those once), shortest first.
func commandHelpAliases(path string) []string {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return nil
	}
	routePath := commandHelpRoutePath(path)
	seen := map[string]struct{}{}
	var prefixes []string
	for _, route := range corehandler.ListBotRoutes() {
		if strings.Trim(strings.TrimSpace(route.Path), "/") != routePath {
			continue
		}
		for _, command := range route.Commands {
			if prefixes == nil {
				prefixes = commandHelpPrefixArgs(command)
			}
			command = normalizeCommandHelpAlias(command)
			if command == "" || !commandHelpShowsAlias(path, command) {
				continue
			}
			seen[command] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	aliases := make([]string, 0, len(seen))
	for alias := range seen {
		if isPrefixArgVariant(alias, prefixes, seen) {
			continue
		}
		aliases = append(aliases, alias)
	}
	slices.SortFunc(aliases, func(a, b string) int {
		if len([]rune(a)) != len([]rune(b)) {
			return len([]rune(a)) - len([]rune(b))
		}
		if c := strings.Compare(strings.ToLower(a), strings.ToLower(b)); c != 0 {
			return c
		}
		return -strings.Compare(a, b)
	})
	// Spellings that differ only in letter case match the same input; keep
	// one (the lower-case one, sorted first above).
	return slices.CompactFunc(aliases, strings.EqualFold)
}

func commandHelpPrefixArgs(command string) []string {
	matched, ok := corehandler.LookupCommandHandler(command)
	if !ok {
		return []string{}
	}
	handler, ok := matched.Handler.(*HarukiSekaiCommandHandler)
	if !ok {
		return []string{}
	}
	prefixes := make([]string, 0, len(handler.PrefixArgs))
	for _, prefix := range handler.PrefixArgs {
		if prefix = strings.TrimSpace(prefix); prefix != "" {
			prefixes = append(prefixes, prefix)
		}
	}
	return prefixes
}

// isPrefixArgVariant reports whether alias is another alias with one of the
// handler's extra prefixes (e.g. "/wlsk线" for "/sk线").
func isPrefixArgVariant(alias string, prefixes []string, all map[string]struct{}) bool {
	for _, prefix := range prefixes {
		rest, ok := strings.CutPrefix(alias, "/"+prefix)
		if !ok || rest == "" {
			continue
		}
		if _, exists := all["/"+strings.TrimPrefix(rest, "/")]; exists {
			return true
		}
	}
	return false
}

func normalizeCommandHelpAlias(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	lower := strings.ToLower(command)
	for _, region := range i18n.RegionCodes {
		prefix := "/" + region
		if !strings.HasPrefix(lower, prefix) || len(command) == len(prefix) {
			continue
		}
		return "/" + strings.TrimPrefix(command[len(prefix):], "/")
	}
	return command
}

var helpDocCodeSpan = regexp.MustCompile("`([^`\n]+)`")

// helpDocCodeSpans returns the text of every inline code span.
func helpDocCodeSpans(markdown string) []string {
	matches := helpDocCodeSpan.FindAllStringSubmatch(markdown, -1)
	spans := make([]string, 0, len(matches))
	for _, match := range matches {
		spans = append(spans, strings.TrimSpace(match[1]))
	}
	return spans
}

// helpDocSpanCommand returns the registered command a code span starts with
// ("/pjsk card" for "`/pjsk card 1`"), without its region prefix, or "" when
// the span is not a command. Arguments written directly after a command may
// not start with a Chinese character: "/歌曲表" is not "/歌曲" plus "表".
func helpDocSpanCommand(span string) string {
	if !strings.HasPrefix(span, "/") || span == "/" {
		return ""
	}
	matched := corehandler.MatchCommandHandler(span)
	if matched.Handler == nil {
		return ""
	}
	if len(matched.ArgText) > 0 && isHanRune(matched.ArgText[0]) {
		return ""
	}
	// The span's own spelling: registered commands match without regard to
	// letter case, and the help should repeat what the document wrote.
	written := strings.TrimSpace(string([]rune(span)[:matched.PrefixLength]))
	return normalizeCommandHelpAlias(written)
}

func isHanRune(r rune) bool {
	return unicode.Is(unicode.Han, r)
}

var (
	plainHelpStrong = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	plainHelpCode   = regexp.MustCompile("`([^`]*)`")
)

// commandHelpPlainText turns a help document into chat text: headings become
// "标题：" lines, list markers become "·", and code fences, inline code and
// emphasis markers are dropped.
func commandHelpPlainText(locale i18n.Locale, markdown string) string {
	lines := strings.Split(strings.TrimSpace(markdown), "\n")
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		line = strings.TrimRight(line, " \t")
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "```"):
			continue
		case strings.HasPrefix(trimmed, "#"):
			heading := plainHelpInline(strings.TrimSpace(strings.TrimLeft(trimmed, "#")))
			if i == 0 {
				out = append(out, heading)
				continue
			}
			out = append(out, i18n.M("usage.help.plain_heading", i18n.Data{"Heading": heading}).In(locale))
		case strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* "):
			out = append(out, "· "+plainHelpInline(strings.TrimSpace(trimmed[2:])))
		default:
			out = append(out, plainHelpInline(line))
		}
	}
	text := strings.Join(out, "\n")
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(text)
}

func plainHelpInline(text string) string {
	text = plainHelpStrong.ReplaceAllString(text, "$1")
	return plainHelpCode.ReplaceAllString(text, "$1")
}
