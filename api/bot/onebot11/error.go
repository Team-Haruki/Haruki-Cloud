package onebot11

import "fmt"

type ReplayError string

func (r ReplayError) Error() string {
	return string(r)
}
func NewReplayError(format string, a ...any) ReplayError {
	return ReplayError(fmt.Sprintf(format, a...))
}
