// Package onebot11 provides typed representations of the three most common
// OneBot v11 message segment types used in bot API responses.
package onebot11

// Segment is a single OneBot v11 message segment.
// The JSON representation is {"type": "<type>", "data": {...}}.
type Segment struct {
	Type string `json:"type" msgpack:"type"`
	Data any    `json:"data" msgpack:"data"`
}

// TextData is the data payload for a "text" segment.
type TextData struct {
	Text string `json:"text" msgpack:"text"`
}

// ImageData is the data payload for an "image" segment.
type ImageData struct {
	File string `json:"file" msgpack:"file"`
}

// AtData is the data payload for an "at" segment.
type AtData struct {
	QQ string `json:"qq" msgpack:"qq"`
}

// Text returns a text message segment.
func Text(text string) Segment {
	return Segment{Type: "text", Data: TextData{Text: text}}
}

// Image returns an image message segment with the given URL or file path.
func Image(file string) Segment {
	return Segment{Type: "image", Data: ImageData{File: file}}
}

// At returns an at (@mention) message segment targeting the given user ID.
func At(userID string) Segment {
	return Segment{Type: "at", Data: AtData{QQ: userID}}
}

// Message is a slice of Segments, representing a full OneBot v11 message.
type Message []Segment
