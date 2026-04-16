// Package onebot11 provides typed representations of the three most common
// OneBot v11 message segment types used in bot API responses.
package onebot11

const (
	TYPE_TEXT  = "text"
	TYPE_IMAGE = "image"
	TYPE_AT    = "at"
)

const (
	KEY_TEXT = "text"
	KEY_FILE = "file"
	KEY_URL  = "url"
	KEY_QQ   = "qq"
)

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
	Url  string `json:"url" msgpack:"url"`
}

// AtData is the data payload for an "at" segment.
type AtData struct {
	QQ string `json:"qq" msgpack:"qq"`
}

// Text returns a text message segment.
func Text(text string) Segment {
	return Segment{Type: TYPE_TEXT, Data: TextData{Text: text}}
}

// Image returns an image message segment with the given URL or file path.
func Image(file string, url string) Segment {
	return Segment{Type: TYPE_IMAGE, Data: ImageData{File: file, Url: url}}
}

// At returns an at (@mention) message segment targeting the given user ID.
func At(userID string) Segment {
	return Segment{Type: TYPE_AT, Data: AtData{QQ: userID}}
}

// Message is a slice of Segments, representing a full OneBot v11 message.
type Message []Segment
