package commandtrace

import (
	"context"
	"testing"
)

func BenchmarkMeasureOperationWithoutTrace(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		finish := MeasureOperation(ctx, "snapshot.decode")
		finish()
	}
}

func BenchmarkMeasureOperationWithTrace(b *testing.B) {
	ctx, _ := WithTrace(context.Background())
	b.ReportAllocs()
	for b.Loop() {
		finish := MeasureOperation(ctx, "snapshot.decode")
		finish()
	}
}

func BenchmarkCommandOperationsSummary(b *testing.B) {
	names := []string{
		"request.body_decode", "request.message_parse", "command.context_build",
		"command.match", "command.parse", "runtime.ban_check", "binding.resolve",
		"snapshot.private_data", "toolbox.http", "toolbox.decompress",
		"snapshot.decode", "asset.store_get", "drawing.encode", "drawing.http",
		"drawing.decode", "image.result_url",
	}
	b.ReportAllocs()
	for b.Loop() {
		ctx, trace := WithTrace(context.Background())
		for range 5 {
			for _, name := range names {
				finish := MeasureOperation(ctx, name)
				finish()
			}
		}
		_ = trace.Snapshot().OperationValue()
	}
}
