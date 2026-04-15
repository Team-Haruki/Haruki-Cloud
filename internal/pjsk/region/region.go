package region

import "strings"

type Value string

const (
	Unknown Value = ""
	JP      Value = "jp"
	CN      Value = "cn"
	TW      Value = "tw"
	EN      Value = "en"
	KR      Value = "kr"
)

func Normalize(value string) Value {
	return Value(strings.ToLower(strings.TrimSpace(value)))
}

func WithDefault(value Value) Value {
	normalized := Normalize(value.String())
	if normalized.IsZero() {
		return JP
	}
	return normalized
}

func (v Value) String() string {
	return string(v)
}

func (v Value) IsZero() bool {
	return v == Unknown
}
