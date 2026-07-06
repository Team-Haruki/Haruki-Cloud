package handler

import (
	"context"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	corehandler "haruki-cloud/internal/handler"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	renderapp "haruki-cloud/internal/pjsk/render/app"
)

//go:embed helpdocs/*.md
var commandHelpDocs embed.FS

func commandHelpMessage(ctx context.Context, resolved *CommandRequest, app *renderapp.App) (onebot11.Message, error) {
	markdown, err := commandHelpMarkdown(resolved)
	if err != nil {
		return nil, err
	}
	if app == nil || app.Drawing == nil {
		return commandHelpTextMessage(markdown), nil
	}
	path := commandHelpRequestPath(resolved)
	image, err := app.Drawing.WithContext(ctx).GenerateCommandHelp(&drawing.CommandHelpRenderRequest{
		Path:     path,
		Title:    commandHelpTitle(resolved, markdown),
		Markdown: markdown,
	})
	if err != nil {
		return commandHelpTextMessage(markdown), nil
	}
	if app.ImageCache != nil {
		url, err := app.ImageCache.StoreAndGetURL(ctx, image, BotModulePJSK)
		if err != nil {
			return commandHelpTextMessage(markdown), nil
		}
		return onebot11.Message{onebot11.Image(url, "")}, nil
	}
	return onebot11.Message{
		onebot11.Image("base64://"+base64.StdEncoding.EncodeToString(image), ""),
	}, nil
}

func commandHelpTextMessage(markdown string) onebot11.Message {
	return onebot11.Message{onebot11.Text(strings.TrimSpace(markdown))}
}

func commandHelpMarkdown(resolved *CommandRequest) (string, error) {
	helper := ""
	trigger := ""
	if resolved != nil {
		helper = strings.TrimSpace(resolved.HelpText)
		trigger = strings.TrimSpace(resolved.TriggerCommand)
	}
	path := commandHelpRequestPath(resolved)

	for _, key := range commandHelpLookupKeys(path) {
		md, ok, err := readCommandHelpMarkdown(key)
		if err != nil {
			return "", err
		}
		if ok {
			return withCommandHelpAliasSection(md, path), nil
		}
	}
	if md := strings.TrimSpace(helper); md != "" {
		return withCommandHelpAliasSection(fallbackCommandHelpMarkdown(trigger, path, md), path), nil
	}
	md, ok, err := readCommandHelpMarkdown("generic")
	if err != nil {
		return "", err
	}
	if ok {
		return withCommandHelpAliasSection(md, path), nil
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
		return "mysekai/blueprint"
	}
	return path
}

func normalizeCommandHelpTrigger(trigger string) string {
	trigger = strings.TrimSpace(trigger)
	if trigger == "" {
		return ""
	}
	lower := strings.ToLower(trigger)
	for _, region := range []string{"jp", "en", "cn", "tw", "kr"} {
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

func isMysekaiBlueprintHelpTrigger(trigger string) bool {
	switch trigger {
	case "/msb", "/mysekai blueprint", "/mysekai 蓝图", "/pjsk mysekai blueprint":
		return true
	default:
		return false
	}
}

func commandHelpLookupKeys(path string) []string {
	keys := make([]string, 0, 3)
	add := func(key string) {
		key = commandHelpDocKey(key)
		if key == "" {
			return
		}
		for _, existing := range keys {
			if existing == key {
				return
			}
		}
		keys = append(keys, key)
	}
	add(path)
	add(commandHelpFamily(path))
	return keys
}

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
	data, err := commandHelpDocs.ReadFile("helpdocs/" + key + ".md")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	return strings.TrimSpace(string(data)), true, nil
}

func commandHelpFamily(path string) string {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return "generic"
	}
	if idx := strings.IndexByte(path, '/'); idx >= 0 {
		return path[:idx]
	}
	return path
}

func commandHelpTitle(resolved *CommandRequest, markdown string) string {
	for _, line := range strings.Split(markdown, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			title := strings.TrimSpace(strings.TrimLeft(line, "#"))
			if title != "" {
				return title
			}
		}
	}
	if resolved != nil {
		if title := strings.TrimSpace(resolved.TriggerCommand); title != "" {
			return title
		}
		if title := strings.TrimSpace(resolved.CommandPath); title != "" {
			return title
		}
	}
	return "指令帮助"
}

func fallbackCommandHelpMarkdown(trigger, path, helper string) string {
	title := strings.TrimSpace(trigger)
	if title == "" {
		title = strings.TrimSpace(path)
	}
	if title == "" {
		title = "指令帮助"
	}
	return fmt.Sprintf("# %s\n\n%s", title, helper)
}

func withCommandHelpAliasSection(markdown string, path string) string {
	markdown = strings.TrimSpace(markdown)
	aliases := missingCommandHelpAliases(markdown, path)
	if len(aliases) == 0 {
		return markdown
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
	return strings.TrimSpace(markdown + "\n\n## 指令别名\n" + strings.Join(lines, "\n"))
}

func missingCommandHelpAliases(markdown string, path string) []string {
	aliases := commandHelpAliases(path)
	if len(aliases) == 0 {
		return nil
	}
	missing := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		if strings.Contains(markdown, alias) {
			continue
		}
		missing = append(missing, alias)
	}
	return missing
}

func commandHelpAliases(path string) []string {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return nil
	}
	seen := map[string]struct{}{}
	for _, route := range corehandler.ListBotRoutes() {
		if strings.Trim(strings.TrimSpace(route.Path), "/") != path {
			continue
		}
		for _, command := range route.Commands {
			command = normalizeCommandHelpAlias(command)
			if command == "" {
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
		aliases = append(aliases, alias)
	}
	slices.SortFunc(aliases, func(a, b string) int {
		if len([]rune(a)) == len([]rune(b)) {
			return strings.Compare(a, b)
		}
		return len([]rune(a)) - len([]rune(b))
	})
	return aliases
}

func normalizeCommandHelpAlias(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	lower := strings.ToLower(command)
	for _, region := range []string{"jp", "en", "cn", "tw", "kr"} {
		prefix := "/" + region
		if !strings.HasPrefix(lower, prefix) || len(command) == len(prefix) {
			continue
		}
		return "/" + strings.TrimPrefix(command[len(prefix):], "/")
	}
	return command
}
