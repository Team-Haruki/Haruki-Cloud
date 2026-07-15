package handler

import (
	"context"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
)

type CommandTrace = commandtrace.Trace

func WithCommandTrace(ctx context.Context) (context.Context, *CommandTrace) {
	return commandtrace.WithTrace(ctx)
}

func CommandTraceFromContext(ctx context.Context) *CommandTrace {
	return commandtrace.FromContext(ctx)
}

func recordCommandStage(ctx context.Context, stage string, elapsed time.Duration) {
	commandtrace.RecordOperation(ctx, stage, elapsed)
}

func measureCommandOperation(ctx context.Context, operation string) func() {
	return commandtrace.MeasureOperation(ctx, operation)
}

func measurePayloadBuild(ctx context.Context) func() {
	return measureCommandOperation(ctx, "payload.build")
}
