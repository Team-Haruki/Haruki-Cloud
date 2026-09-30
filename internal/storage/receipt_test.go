package storage_test

import (
	"context"
	"testing"
	"time"

	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

type receiptingStore struct {
	storage.Store
	node string
}

func (s receiptingStore) Put(ctx context.Context, key storage.Key, data []byte, options storage.PutOptions) error {
	if err := s.Store.Put(ctx, key, data, options); err != nil {
		return err
	}
	storage.ReportWriteReceipt(ctx, storage.WriteReceipt{WriterNode: s.node, WrittenAt: time.Now().UTC()})
	return nil
}

func TestDualStoreReceiptComesOnlyFromPrimary(t *testing.T) {
	primary := receiptingStore{Store: storagetest.NewMemory(), node: "primary"}
	mirror := receiptingStore{Store: storagetest.NewMemory(), node: "mirror"}
	store := storage.NewDual(primary, mirror, storage.DualWrite, nil)
	var receipts []storage.WriteReceipt
	ctx := storage.WithWriteReceipt(context.Background(), func(receipt storage.WriteReceipt) { receipts = append(receipts, receipt) })
	if err := store.Put(ctx, "image", []byte("bytes"), storage.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 1 || receipts[0].WriterNode != "primary" {
		t.Fatalf("receipts=%+v", receipts)
	}
}

func TestLocalStoreReceiptIsUnnamedAndOnlyForSuccessfulWrite(t *testing.T) {
	store, err := storage.NewLocal(t.TempDir(), 10)
	if err != nil {
		t.Fatal(err)
	}
	var receipts []storage.WriteReceipt
	ctx := storage.WithWriteReceipt(context.Background(), func(receipt storage.WriteReceipt) { receipts = append(receipts, receipt) })
	if err := store.Put(ctx, "image", []byte("bytes"), storage.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 1 || receipts[0].WriterNode != "" || receipts[0].WrittenAt.IsZero() {
		t.Fatalf("receipts=%+v", receipts)
	}
	if err := store.Put(ctx, "bad", make([]byte, 11), storage.PutOptions{}); err == nil {
		t.Fatal("oversize write succeeded")
	}
	if len(receipts) != 1 {
		t.Fatal("failed local Put emitted receipt")
	}
}

func TestEndpointNamesUseExplicitResolvedPositions(t *testing.T) {
	cfg := storage.ProviderConfig{Scheme: "s3", Bucket: "bucket", Endpoints: []string{"http://first", "http://second"}, EndpointNames: []string{" vm105 ", ""}}
	resolved, err := storage.Resolve(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.EndpointNames) != 2 || resolved.EndpointNames[0] != "vm105" || resolved.EndpointNames[1] != "" {
		t.Fatalf("names=%v", resolved.EndpointNames)
	}
	for _, names := range [][]string{{"one"}, {"https://must-not-infer", "other"}} {
		cfg.EndpointNames = names
		if _, err := storage.Resolve(cfg); err == nil {
			t.Fatalf("invalid mapping accepted: %v", names)
		}
	}
	cfg.EndpointNames = nil
	resolved, err = storage.Resolve(cfg)
	if err != nil || len(resolved.EndpointNames) != 0 {
		t.Fatalf("unconfigured mapping=%v error=%v", resolved.EndpointNames, err)
	}
}
