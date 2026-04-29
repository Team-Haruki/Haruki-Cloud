package handler

import (
	"strings"
	"testing"
)

func resetCommandRegistryForTest() {
	treeMutex.Lock()
	defer treeMutex.Unlock()
	commandHandlerTree = handlerTreeNode{}
	commandLookupRegistry = map[string]*lookupEntry{}
	commandMatchRegistry = nil
	botRouteRegistry = map[string]*botRouteEntry{}
	maxDepth = 0
}

func TestMatchCommandPrefixDoesNotCrossMessageSeparatorWithoutCommandSeparator(t *testing.T) {
	cases := []struct {
		name        string
		message     string
		command     string
		wantOK      bool
		wantArgText string
	}{
		{
			name:    "event saki is not events",
			message: "/event saki",
			command: "/events",
			wantOK:  false,
		},
		{
			name:        "events remains events",
			message:     "/events saki",
			command:     "/events",
			wantOK:      true,
			wantArgText: "saki",
		},
		{
			name:        "command separator accepts message separator",
			message:     "/pjsk event saki",
			command:     "/pjsk event",
			wantOK:      true,
			wantArgText: "saki",
		},
		{
			name:        "command separator can be omitted",
			message:     "/pjskevent saki",
			command:     "/pjsk event",
			wantOK:      true,
			wantArgText: "saki",
		},
		{
			name:        "dash command accepts space",
			message:     "/event list 1",
			command:     "/event-list",
			wantOK:      true,
			wantArgText: "1",
		},
		{
			name:    "plain command does not accept inserted separator",
			message: "/event list 1",
			command: "/eventlist",
			wantOK:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prefixLength, ok := MatchCommandPrefix(tc.message, tc.command)
			if ok != tc.wantOK {
				t.Fatalf("MatchCommandPrefix() ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			gotArgText := strings.TrimSpace(string([]rune(tc.message)[prefixLength:]))
			if gotArgText != tc.wantArgText {
				t.Fatalf("arg text = %q, want %q", gotArgText, tc.wantArgText)
			}
		})
	}
}

func TestMatchCommandHandlerTreatsMessageSeparatorAsArgumentBoundary(t *testing.T) {
	resetCommandRegistryForTest()
	defer resetCommandRegistryForTest()

	eventHandler := &CommandHandlerBase{
		Commands: []string{"/event"},
		Path:     "event",
		Priority: DefaultPriority,
	}
	eventsHandler := &CommandHandlerBase{
		Commands: []string{"/events"},
		Path:     "event/list",
		Priority: DefaultPriority,
	}
	RegisterCommandHandler("test", eventHandler)
	RegisterCommandHandler("test", eventsHandler)

	matched := MatchCommandHandler("/event saki")
	if matched.Command != "/event" {
		t.Fatalf("matched command = %q, want /event", matched.Command)
	}
	if got := strings.TrimSpace(string(matched.ArgText)); got != "saki" {
		t.Fatalf("arg text = %q, want saki", got)
	}

	matched = MatchCommandHandler("/events saki")
	if matched.Command != "/events" {
		t.Fatalf("matched command = %q, want /events", matched.Command)
	}
	if got := strings.TrimSpace(string(matched.ArgText)); got != "saki" {
		t.Fatalf("arg text = %q, want saki", got)
	}
}
