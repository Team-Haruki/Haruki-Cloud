package event

import (
	"haruki-cloud/internal/i18n"
	"haruki-cloud/utils/usererror"
)

// ErrNoOngoingEvent reports that no event is running now. It is a typed user
// error; compare with errors.Is.
var ErrNoOngoingEvent = usererror.New(usererror.CodeNotFound, i18n.M("event.no_ongoing"))
