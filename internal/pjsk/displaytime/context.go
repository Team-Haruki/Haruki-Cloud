package displaytime

import (
	"context"
	"time"
)

type requestTimeZoneKey struct{}

func WithRequestTimeZone(ctx context.Context, value string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, requestTimeZoneKey{}, NormalizeTimeZone(value))
}

func RequestTimeZoneFromContext(ctx context.Context) string {
	if ctx == nil {
		return DefaultTimeZone
	}
	value, _ := ctx.Value(requestTimeZoneKey{}).(string)
	return NormalizeTimeZone(value)
}

// RequestLocation is the requester's time zone as a location, for
// i18n.FormatUserTime. It is Asia/Shanghai when none was resolved.
func RequestLocation(ctx context.Context) *time.Location {
	loc, _ := LoadLocation(RequestTimeZoneFromContext(ctx))
	return loc
}
