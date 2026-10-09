package onebot11

import "haruki-cloud/internal/i18n"

// LocalizedTextData is the data of a text segment built from a catalog
// message that is rendered when the reply is delivered (LocalizedText). Text
// holds the message rendered without user input, so a segment that is
// encoded without Render never shows it.
type LocalizedTextData struct {
	Text    string `json:"text" msgpack:"text"`
	message i18n.Message
}

// LocalizedText returns a text segment whose text is msg, rendered for the
// receiving bot client by Message.Render. Command results use it for a
// success reply that repeats user input the parameter echo rule covers
// (unreviewed alias text): one shared command result can then be delivered
// with or without echo.
func LocalizedText(msg i18n.Message) Segment {
	return Segment{Type: TypeText, Data: LocalizedTextData{
		Text:    msg.Render(i18n.RenderOptions{NoEcho: true}),
		message: msg,
	}}
}

// HasLocalizedText reports whether m has a segment built by LocalizedText.
func (m Message) HasLocalizedText() bool {
	for _, segment := range m {
		if _, ok := segment.Data.(LocalizedTextData); ok {
			return true
		}
	}
	return false
}

// Render returns m with every LocalizedText segment rendered with opts as a
// plain text segment. Other segments are kept; m itself is not changed.
func (m Message) Render(opts i18n.RenderOptions) Message {
	if !m.HasLocalizedText() {
		return m
	}
	rendered := make(Message, len(m))
	for i, segment := range m {
		if data, ok := segment.Data.(LocalizedTextData); ok {
			segment = Text(data.message.Render(opts))
		}
		rendered[i] = segment
	}
	return rendered
}
