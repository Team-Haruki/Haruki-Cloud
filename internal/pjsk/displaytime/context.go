package displaytime

import "context"

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
