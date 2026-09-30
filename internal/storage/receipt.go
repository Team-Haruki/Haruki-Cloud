package storage

import (
	"context"
	"time"
)

// WriteReceipt identifies the endpoint that completed this Put. WriterNode is
// supplied explicitly by configuration, never inferred from an endpoint URL.
// An empty node preserves compatibility with local or unmapped stores.
type WriteReceipt struct {
	WriterNode string
	WrittenAt  time.Time
}

type writeReceiptKey struct{}

// WithWriteReceipt observes a successful Put on this call's context. The
// callback runs synchronously before Put returns; callers concurrently reusing
// the context must make their callback concurrency-safe. A nil callback clears
// an inherited receipt (used for a DualStore's best-effort mirror write).
func WithWriteReceipt(ctx context.Context, callback func(WriteReceipt)) context.Context {
	return context.WithValue(ctx, writeReceiptKey{}, callback)
}

// ReportWriteReceipt is called by backends only after a successful Put. It does
// not retain the receipt or any per-request data on a shared store.
func ReportWriteReceipt(ctx context.Context, receipt WriteReceipt) {
	if callback, _ := ctx.Value(writeReceiptKey{}).(func(WriteReceipt)); callback != nil {
		callback(receipt)
	}
}
