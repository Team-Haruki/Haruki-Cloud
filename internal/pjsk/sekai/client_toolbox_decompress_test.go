package sekai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"weak"

	"github.com/go-resty/resty/v2"
	"github.com/klauspost/compress/zstd"
)

func toolboxCompressedResponse(body []byte) *resty.Response {
	return (&resty.Response{RawResponse: &http.Response{Header: http.Header{"Content-Encoding": []string{"zstd"}}}}).SetBody(body)
}

func toolboxEncode(t *testing.T, plain []byte) []byte {
	t.Helper()
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	return encoder.EncodeAll(plain, nil)
}

func toolboxEncodeStream(t *testing.T, plain []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	encoder, err := zstd.NewWriter(&out, zstd.WithEncoderConcurrency(1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encoder.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// A raw-block frame without content size makes the output limit depend on
// streaming reads, even when its window descriptor advertises a large window.
func toolboxUnknownSizeFrame(plain []byte, windowLog byte) []byte {
	header := uint32(len(plain))<<3 | 1
	out := []byte{0x28, 0xb5, 0x2f, 0xfd, 0, (windowLog - 10) << 3, byte(header), byte(header >> 8), byte(header >> 16)}
	return append(out, plain...)
}

func TestToolboxDecoderReuseAfterErrors(t *testing.T) {
	client := NewToolboxClient(nil)
	defer client.Close()
	plain := bytes.Repeat([]byte("toolbox snapshot"), 1024)
	valid := toolboxCompressedResponse(toolboxEncode(t, plain))
	if got, err := client.decompressContext(context.Background(), valid); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("initial decode: size=%d err=%v", len(got), err)
	}
	first := client.decoders.idle[0]
	cases := []struct {
		name      string
		body      []byte
		limit     int64
		errorText string
	}{
		{"corrupt", []byte("not zstd"), toolboxMaxDecompressedResponseBytes, "zstd"},
		{"truncated", valid.Body()[:len(valid.Body())-3], toolboxMaxDecompressedResponseBytes, "zstd"},
		{"known size exceeds limit", valid.Body(), 16, "exceeds"},
		{"unknown size exceeds limit", toolboxUnknownSizeFrame(plain, 15), 16, "exceeds"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := client.decompressContextLimit(context.Background(), toolboxCompressedResponse(tc.body), tc.limit); err == nil || !strings.Contains(err.Error(), tc.errorText) || got != nil {
				t.Fatalf("failure size=%d err=%v", len(got), err)
			}
			if len(client.decoders.idle) != 1 || client.decoders.idle[0] != first {
				t.Fatal("failed stream did not reset the borrowed decoder")
			}
			if got, err := client.decompressContext(context.Background(), valid); err != nil || !bytes.Equal(got, plain) {
				t.Fatalf("recovery size=%d err=%v", len(got), err)
			}
		})
	}
}

func TestToolboxDecoderFramesAndWindowLimits(t *testing.T) {
	plain := []byte("bounded streaming toolbox data")
	normal := toolboxUnknownSizeFrame(plain, 10)
	large := toolboxUnknownSizeFrame(plain, 26)     //64 MiB: accepted before pooling.
	oversized := toolboxUnknownSizeFrame(plain, 30) //1 GiB: rejected before pooling.
	for _, tc := range []struct {
		name       string
		body, want []byte
		limit      int64
		errorText  string
	}{
		{"unknown size", normal, plain, 1024, ""},
		{"unknown size large window", large, plain, 1024, ""},
		{"multiple frames", append(append([]byte{}, normal...), normal...), append(append([]byte{}, plain...), plain...), 1024, ""},
		{"later frame large window", append(append([]byte{}, normal...), large...), append(append([]byte{}, plain...), plain...), 1024, ""},
		{"multiple frames output cap", append(append([]byte{}, normal...), normal...), nil, int64(len(plain) + 1), "exceeds"},
		{"fallback output cap", large, nil, 8, "exceeds"},
		{"original window cap", oversized, nil, 1024, "window"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := NewToolboxClient(nil)
			defer client.Close()
			got, err := client.decompressContextLimit(context.Background(), toolboxCompressedResponse(tc.body), tc.limit)
			if tc.errorText != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errorText) || got != nil {
					t.Fatalf("size=%d err=%v", len(got), err)
				}
			} else if err != nil || !bytes.Equal(got, tc.want) {
				t.Fatalf("size=%d err=%v", len(got), err)
			}
			if len(client.decoders.idle) != 1 {
				t.Fatalf("idle decoders=%d", len(client.decoders.idle))
			}
			// Large windows must never increase the pooled decoder's configured cap.
			decoder, _ := client.decoders.acquire()
			_, err = readToolboxZstd(context.Background(), decoder, large, 1024)
			client.decoders.release(decoder)
			if !errors.Is(err, zstd.ErrWindowSizeExceeded) && !errors.Is(err, zstd.ErrDecoderSizeExceeded) {
				t.Fatalf("pooled decoder accepted large window: %v", err)
			}
		})
	}
}

func TestToolboxDecoderConcurrentAndBounded(t *testing.T) {
	client := NewToolboxClient(nil)
	defer client.Close()
	var wg sync.WaitGroup
	for worker := range 16 {
		plain := bytes.Repeat([]byte(fmt.Sprintf("worker %02d: distinct snapshot\n", worker)), 2048)
		resp := toolboxCompressedResponse(toolboxEncode(t, plain))
		wg.Go(func() {
			for range 10 {
				out, err := client.decompressContext(context.Background(), resp)
				if err != nil || !bytes.Equal(out, plain) {
					t.Errorf("concurrent size=%d err=%v", len(out), err)
					return
				}
			}
		})
	}
	wg.Wait()
	if n := len(client.decoders.idle); n == 0 || n > toolboxIdleDecoders {
		t.Fatalf("idle count=%d", n)
	}
}

type toolboxCancelAfterChecks struct {
	context.Context
	cancel    context.CancelFunc
	remaining atomic.Int32
}

func (c *toolboxCancelAfterChecks) Err() error {
	if c.remaining.Add(-1) == 0 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestToolboxDecoderCancellationAndReuse(t *testing.T) {
	client := NewToolboxClient(nil)
	defer client.Close()
	plain := bytes.Repeat([]byte("cancel this stream without decoding all data"), 1<<16)
	// A streamed frame has no content size, so it takes the cancellable
	// streaming path rather than the one-shot DecodeAll.
	resp := toolboxCompressedResponse(toolboxEncodeStream(t, plain))
	decoder, err := client.decoders.acquire()
	if err != nil {
		t.Fatal(err)
	}
	client.decoders.release(decoder)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	checked := &toolboxCancelAfterChecks{Context: ctx, cancel: cancel}
	checked.remaining.Store(10)
	if out, err := client.decompressContext(checked, resp); !errors.Is(err, context.Canceled) || out != nil {
		t.Fatalf("canceled size=%d err=%v", len(out), err)
	}
	if len(client.decoders.idle) != 1 || client.decoders.idle[0] != decoder {
		t.Fatal("canceled stream did not release its decoder")
	}
	if out, err := client.decompressContext(context.Background(), resp); err != nil || !bytes.Equal(out, plain) {
		t.Fatalf("reuse after cancellation size=%d err=%v", len(out), err)
	}
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if _, err := client.decompressContext(canceled, resp); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled error=%v", err)
	}
}

func TestToolboxDecoderCloseWithInFlightOwnership(t *testing.T) {
	client := NewToolboxClient(nil)
	first, err := client.decoders.acquire()
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.decoders.acquire()
	if err != nil {
		t.Fatal(err)
	}
	client.decoders.release(second)
	var wg sync.WaitGroup
	wg.Go(func() { _ = client.Close() })
	wg.Go(func() { client.decoders.release(first) })
	wg.Go(func() { _ = client.Close() })
	wg.Wait()
	if len(client.decoders.idle) != 0 {
		t.Fatal("closed pool retains decoder")
	}
	if _, err := client.decoders.acquire(); err == nil {
		t.Fatal("closed pool allowed acquire")
	}
	if err := first.Reset(nil); !errors.Is(err, zstd.ErrDecoderClosed) {
		t.Fatalf("in-flight decoder not closed: %v", err)
	}
	if err := second.Reset(nil); !errors.Is(err, zstd.ErrDecoderClosed) {
		t.Fatalf("idle decoder not closed: %v", err)
	}
	var absent *HarukiToolboxClient
	if err := absent.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := absent.decompressContext(context.Background(), nil); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("nil client err=%v", err)
	}
}

func TestToolboxDecoderPlainAndNilResponse(t *testing.T) {
	client := NewToolboxClient(nil)
	defer client.Close()
	if _, err := client.decompressContext(context.Background(), nil); err == nil {
		t.Fatal("nil response accepted")
	}
	resp := (&resty.Response{RawResponse: &http.Response{Header: make(http.Header)}}).SetBody([]byte("plain data"))
	if out, err := client.decompressContext(context.TODO(), resp); err != nil || string(out) != "plain data" {
		t.Fatalf("plain body size=%d err=%v", len(out), err)
	}
	if _, err := client.decompressContextLimit(context.Background(), resp, 4); err == nil {
		t.Fatal("plain response cap not enforced")
	}
}

type toolboxPauseContext struct {
	context.Context
	checks  atomic.Int32
	entered chan struct{}
	resume  chan struct{}
}

func (c *toolboxPauseContext) Err() error {
	if c.checks.Add(1) == 3 {
		close(c.entered)
		<-c.resume
	}
	return c.Context.Err()
}

func TestToolboxDecoderCloseDuringDecode(t *testing.T) {
	client := NewToolboxClient(nil)
	defer client.Close()
	plain := bytes.Repeat([]byte("active stream"), 1024)
	resp := toolboxCompressedResponse(toolboxEncode(t, plain))
	ctx := &toolboxPauseContext{Context: context.Background(), entered: make(chan struct{}), resume: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		defer close(done)
		out, err := client.decompressContext(ctx, resp)
		if err != nil || !bytes.Equal(out, plain) {
			t.Errorf("in-flight decode interrupted by Close: size=%d err=%v", len(out), err)
		}
	}()
	<-ctx.entered
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	close(ctx.resume)
	<-done
	if len(client.decoders.idle) != 0 {
		t.Fatal("active stream repopulated closed pool")
	}
}

func toolboxDecodeWeakReferences(t *testing.T, client *HarukiToolboxClient, compressed []byte, limit int64, cancelAfter int32) (weak.Pointer[byte], weak.Pointer[toolboxCancelAfterChecks]) {
	t.Helper()
	// Give this invocation sole ownership so no fixture reference keeps its
	// response or request context alive after the decoder goes idle.
	body := bytes.Clone(compressed)
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &toolboxCancelAfterChecks{Context: base, cancel: cancel}
	ctx.remaining.Store(cancelAfter)
	bodyRef, ctxRef := weak.Make(&body[0]), weak.Make(ctx)
	_, _ = client.decompressContextLimit(ctx, toolboxCompressedResponse(body), limit)
	return bodyRef, ctxRef
}

func TestToolboxIdleDecoderReleasesRequestReferences(t *testing.T) {
	valid := toolboxEncode(t, bytes.Repeat([]byte("private request reference lifetime"), 1<<15))
	for _, tc := range []struct {
		name        string
		body        []byte
		limit       int64
		cancelAfter int32
	}{
		{"success", valid, toolboxMaxDecompressedResponseBytes, 0},
		{"limit", valid, 16, 0},
		{"corrupt", []byte("invalid zstd frame with private data"), 1024, 0},
		{"cancel", valid, toolboxMaxDecompressedResponseBytes, 10},
		{"large window fallback", toolboxUnknownSizeFrame([]byte("private large-window response"), 26), 1024, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := NewToolboxClient(nil)
			defer client.Close()
			body, ctx := toolboxDecodeWeakReferences(t, client, tc.body, tc.limit, tc.cancelAfter)
			runtime.GC()
			if body.Value() != nil {
				t.Error("idle decoder retains compressed response")
			}
			if ctx.Value() != nil {
				t.Error("idle decoder retains request context")
			}
			runtime.KeepAlive(client)
		})
	}
}

func TestToolboxDecodeAllFastPath(t *testing.T) {
	plain := bytes.Repeat([]byte(`{"userCards":[{"cardId":1,"level":60}]},`), 4096)
	sized := toolboxEncode(t, plain)
	streamed := toolboxEncodeStream(t, plain)

	var header zstd.Header
	if err := header.Decode(sized); err != nil || !header.HasFCS {
		t.Fatalf("EncodeAll frame header = %+v, %v", header, err)
	}
	if err := header.Decode(streamed); err != nil || header.HasFCS {
		t.Fatalf("streamed frame header = %+v, %v", header, err)
	}

	client := NewToolboxClient(nil)
	defer client.Close()
	decoder, err := client.decoders.acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer client.decoders.release(decoder)

	out, ok := decodeToolboxZstdFrame(decoder, sized, toolboxMaxDecompressedResponseBytes)
	if !ok || !bytes.Equal(out, plain) || cap(out) != len(plain)+toolboxDecodeAllSlack {
		t.Fatalf("fast path ok=%t size=%d cap=%d", ok, len(out), cap(out))
	}
	original := bytes.Clone(sized)
	if _, ok := decodeToolboxZstdFrame(decoder, sized, toolboxMaxDecompressedResponseBytes); !ok || !bytes.Equal(sized, original) {
		t.Fatal("fast path must leave the response body untouched")
	}
	twoFrames := append(bytes.Clone(sized), sized...)
	for name, body := range map[string][]byte{
		"no content size": streamed,
		"over limit":      sized,
		"second frame":    twoFrames,
		"truncated":       sized[:len(sized)-4],
		"not zstd":        []byte("plain"),
	} {
		limit := int64(toolboxMaxDecompressedResponseBytes)
		if name == "over limit" {
			limit = int64(len(plain) - 1)
		}
		if out, ok := decodeToolboxZstdFrame(decoder, body, limit); ok || out != nil {
			t.Fatalf("%s: fast path answered (size=%d)", name, len(out))
		}
	}
	// Bodies the fast path declines still decode, or fail, on the streaming path.
	if got, err := client.decompressContext(context.Background(), toolboxCompressedResponse(twoFrames)); err != nil || !bytes.Equal(got, append(bytes.Clone(plain), plain...)) {
		t.Fatalf("two frames size=%d err=%v", len(got), err)
	}
	if got, err := client.decompressContext(context.Background(), toolboxCompressedResponse(streamed)); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("streamed size=%d err=%v", len(got), err)
	}
}
