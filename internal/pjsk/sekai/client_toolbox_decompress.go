package sekai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"haruki-cloud/internal/observability/commandtrace"

	"github.com/go-resty/resty/v2"
	"github.com/klauspost/compress/zstd"
)

const (
	toolboxIdleDecoders          = 2
	toolboxRetainedDecoderWindow = 32 << 20
	// Match zstd's existing defaults for uncommon large-window frames.
	toolboxMaxDecoderWindow = 512 << 20
	toolboxMaxDecoderMemory = 64 << 30
	// DecodeAll keeps its fast block copies when the destination has a little
	// spare capacity past the frame content size.
	toolboxDecodeAllSlack = 64
)

// Each stream exclusively borrows a decoder. Only a bounded number of idle,
// small-window decoders survive requests; busy requests do not wait for a slot.
type toolboxDecoderPool struct {
	mu     sync.Mutex
	idle   []*zstd.Decoder
	closed bool
}

func newToolboxDecoder(window uint64) (*zstd.Decoder, error) {
	return zstd.NewReader(nil,
		zstd.WithDecoderMaxWindow(window),
		zstd.WithDecoderMaxMemory(toolboxMaxDecoderMemory),
		zstd.WithDecodeBuffersBelow(0),
		// DecodeAll may only fill the presized destination; see
		// decodeToolboxZstdFrame. Streaming reads are unaffected.
		zstd.WithDecodeAllCapLimit(true),
	)
}

func (p *toolboxDecoderPool) acquire() (*zstd.Decoder, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, fmt.Errorf("toolbox: decoder pool is closed")
	}
	if n := len(p.idle); n > 0 {
		decoder := p.idle[n-1]
		p.idle[n-1] = nil
		p.idle = p.idle[:n-1]
		p.mu.Unlock()
		return decoder, nil
	}
	p.mu.Unlock()
	return newToolboxDecoder(toolboxRetainedDecoderWindow)
}

func (p *toolboxDecoderPool) release(decoder *zstd.Decoder) {
	// Stop any unfinished stream before ownership can pass to another request,
	// including after errors/cancellation.
	if err := decoder.Reset(nil); err != nil {
		decoder.Close()
		return
	}
	p.mu.Lock()
	if !p.closed && len(p.idle) < toolboxIdleDecoders {
		p.idle = append(p.idle, decoder)
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()
	decoder.Close()
}

// Close releases idle decoder buffers. In-flight decodes retain exclusive
// ownership and close their decoder on release. It is safe to call repeatedly.
func (c *HarukiToolboxClient) Close() error {
	if c == nil {
		return nil
	}
	c.decoders.mu.Lock()
	c.decoders.closed = true
	idle := c.decoders.idle
	c.decoders.idle = nil
	c.decoders.mu.Unlock()
	for _, decoder := range idle {
		decoder.Close()
	}
	return nil
}

func (c *HarukiToolboxClient) decompressContext(ctx context.Context, resp *resty.Response) ([]byte, error) {
	return c.decompressContextLimit(ctx, resp, toolboxMaxDecompressedResponseBytes)
}

func (c *HarukiToolboxClient) decompressContextLimit(ctx context.Context, resp *resty.Response, limit int64) ([]byte, error) {
	if c == nil {
		return nil, ErrClientNotConfigured
	}
	if ctx == nil {
		ctx = context.TODO()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("toolbox: response is nil")
	}
	if limit <= 0 || limit > toolboxMaxDecompressedResponseBytes {
		limit = toolboxMaxDecompressedResponseBytes
	}
	body := resp.Body()
	if resp.Header().Get("Content-Encoding") != "zstd" {
		if int64(len(body)) > limit {
			return nil, fmt.Errorf("toolbox: response exceeds %d-byte limit", limit)
		}
		return body, nil
	}
	finish := commandtrace.MeasureOperation(ctx, "toolbox.decompress")
	defer finish()

	decoder, err := c.decoders.acquire()
	if err != nil {
		return nil, fmt.Errorf("toolbox: zstd reader init failed: %w", err)
	}
	out, ok := decodeToolboxZstdFrame(decoder, body, limit)
	if !ok {
		out, err = readToolboxZstd(ctx, decoder, body, limit)
	}
	c.decoders.release(decoder)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if errors.Is(err, zstd.ErrWindowSizeExceeded) || errors.Is(err, zstd.ErrDecoderSizeExceeded) {
		// Preserve support for all previously accepted windows without retaining
		// their potentially large buffers for the client's entire lifetime.
		decoder, err = newToolboxDecoder(toolboxMaxDecoderWindow)
		if err != nil {
			return nil, fmt.Errorf("toolbox: zstd reader init failed: %w", err)
		}
		out, err = readToolboxZstd(ctx, decoder, body, limit)
		decoder.Close()
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("toolbox: zstd decompression failed: %w", err)
	}
	return out, nil
}

// decodeToolboxZstdFrame decodes a body whose frame header declares its
// content size (Toolbox stores bodies written by EncodeAll) in one DecodeAll
// call into a buffer of that size, instead of growing a buffer through
// io.ReadAll. The cap limit bounds the output, any other length (a second
// frame or a wrong size) is rejected, and every failure reports ok=false so the
// streaming path decodes the body again and returns its usual result or error.
//
// DecodeAll leaves its block decoders pointing into their input until the
// decoder is used again, so it decodes a scratch copy that is zeroed
// afterwards: an idle pooled decoder keeps neither the response nor its bytes.
// The frame is decoded without cancellation checks; it is bounded by limit.
func decodeToolboxZstdFrame(decoder *zstd.Decoder, body []byte, limit int64) ([]byte, bool) {
	var header zstd.Header
	if err := header.Decode(body); err != nil || !header.HasFCS || header.FrameContentSize == 0 || header.FrameContentSize > uint64(limit) {
		return nil, false
	}
	input := bytes.Clone(body)
	out, err := decoder.DecodeAll(input, make([]byte, 0, header.FrameContentSize+toolboxDecodeAllSlack))
	clear(input)
	if err != nil || uint64(len(out)) != header.FrameContentSize {
		return nil, false
	}
	return out, true
}

func readToolboxZstd(ctx context.Context, decoder *zstd.Decoder, body []byte, limit int64) ([]byte, error) {
	input := &toolboxContextReader{ctx: ctx, reader: bytes.NewReader(body)}
	defer func() {
		// Reset drains asynchronous work, but zstd may retain the input wrapper
		// in its frame state. Detach it only after readers have stopped so the
		// idle decoder cannot keep a response or request context alive.
		_ = decoder.Reset(nil)
		input.ctx = nil
		input.reader = nil
	}()
	if err := decoder.Reset(input); err != nil {
		return nil, err
	}
	reader := toolboxContextReader{ctx: ctx, reader: decoder}
	out, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(out)) > limit {
		return nil, fmt.Errorf("toolbox: decompressed response exceeds %d-byte limit", limit)
	}
	return out, nil
}

type toolboxContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r toolboxContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	// Bound work between cancellation checks even as io.ReadAll grows its buffer.
	if len(p) > 32<<10 {
		p = p[:32<<10]
	}
	return r.reader.Read(p)
}
