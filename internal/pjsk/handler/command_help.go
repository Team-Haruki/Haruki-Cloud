package handler

import (
	"context"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"strings"

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
		return nil, fmt.Errorf("drawing client is not configured")
	}
	path := commandHelpRequestPath(resolved)
	image, err := app.Drawing.WithContext(ctx).GenerateCommandHelp(&drawing.CommandHelpRenderRequest{
		Path:     path,
		Title:    commandHelpTitle(resolved, markdown),
		Markdown: markdown,
	})
	if err != nil {
		return nil, err
	}
	if app.ImageCache != nil {
		url, err := app.ImageCache.StoreAndGetURL(ctx, image, BotModulePJSK)
		if err != nil {
			return nil, err
		}
		return onebot11.Message{onebot11.Image(url, "")}, nil
	}
	return onebot11.Message{
		onebot11.Image("base64://"+base64.StdEncoding.EncodeToString(image), ""),
	}, nil
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
			return md, nil
		}
	}
	if md := strings.TrimSpace(helper); md != "" {
		return fallbackCommandHelpMarkdown(trigger, path, md), nil
	}
	md, ok, err := readCommandHelpMarkdown("generic")
	if err != nil {
		return "", err
	}
	if ok {
		return md, nil
	}
	return "", fmt.Errorf("command help markdown not found: path=%s", path)
}

func commandHelpRequestPath(resolved *CommandRequest) string {
	if resolved == nil {
		return ""
	}
	return strings.Trim(strings.TrimSpace(resolved.CommandPath), "/")
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
