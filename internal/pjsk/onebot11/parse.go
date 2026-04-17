package onebot11

import (
	"fmt"
	"regexp"
	"strings"
)

var inlineCQPattern = regexp.MustCompile(`(?i)\[cq:[^\]]+]`)

// StripInlineCQTags removes [CQ:...] tags from text, replacing them with a space.
func StripInlineCQTags(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	return inlineCQPattern.ReplaceAllString(text, " ")
}

// ParseMessage normalises a raw OneBot v11 message. The JSON unmarshaler may
// produce Data fields as map[string]any, map[any]any, or map[string]string
// depending on the transport (JSON vs MsgPack). ParseMessage converts all
// variants into typed segment structs so callers never need to type-assert Data.
func ParseMessage(msg Message) Message {
	segs := make([]Segment, 0, len(msg))
	for _, s := range msg {
		data := make(map[string]string)
		switch d := s.Data.(type) {
		case map[string]any:
			for k, v := range d {
				data[k] = fmt.Sprint(v)
			}
		case map[any]any:
			for k, v := range d {
				ks, _ := k.(string)
				vs := fmt.Sprint(v)
				if ks != "" {
					data[ks] = vs
				}
			}
		case map[string]string:
			data = d
		}
		switch s.Type {
		case TYPE_TEXT:
			segs = append(segs, Text(StripInlineCQTags(data[KEY_TEXT])))
		case TYPE_IMAGE:
			segs = append(segs, Image(data[KEY_FILE], data[KEY_URL]))
		case TYPE_AT:
			segs = append(segs, At(data[KEY_QQ]))
		}
	}
	return segs
}
