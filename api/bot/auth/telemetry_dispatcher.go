package auth

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"haruki-cloud/database/bot"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"
)

const (
	commandTelemetryQueueCapacity = 4096
	commandTelemetryWriteTimeout  = 10 * time.Second
	commandTelemetryFlushTimeout  = 30 * time.Second
)

var commandTelemetryLogger = logger.NewLoggerWithGlobalWriter("CommandTelemetry", "INFO")

type commandTelemetryJob struct {
	ctx        context.Context
	botID      int
	entry      CommandLogEntry
	enqueuedAt time.Time
}

// CommandTelemetryDispatcher removes best-effort analytics writes from the
// client response path while keeping a bounded queue and a serial DB writer.
// The single worker intentionally avoids SQLite lock contention in local/test
// deployments; production databases still benefit because client latency no
// longer includes the four telemetry statements.
type CommandTelemetryDispatcher struct {
	client *bot.Client
	jobs   chan commandTelemetryJob
	done   chan struct{}

	mu        sync.RWMutex
	closed    bool
	closeOnce sync.Once
}

func NewCommandTelemetryDispatcher(client *bot.Client) *CommandTelemetryDispatcher {
	if client == nil {
		return nil
	}
	return newCommandTelemetryDispatcher(client, commandTelemetryQueueCapacity)
}

func newCommandTelemetryDispatcher(client *bot.Client, capacity int) *CommandTelemetryDispatcher {
	if client == nil {
		return nil
	}
	if capacity <= 0 {
		capacity = 1
	}
	dispatcher := &CommandTelemetryDispatcher{
		client: client,
		jobs:   make(chan commandTelemetryJob, capacity),
		done:   make(chan struct{}),
	}
	go dispatcher.run()
	return dispatcher
}

// Enqueue returns false when the dispatcher is closed or its bounded queue is
// full. It never blocks the command response path.
func (d *CommandTelemetryDispatcher) Enqueue(ctx context.Context, botID int, entry CommandLogEntry) bool {
	if d == nil || d.client == nil {
		return true
	}
	job := commandTelemetryJob{
		ctx:        logger.DetachedContext(ctx),
		botID:      botID,
		entry:      boundedCommandLogEntry(entry),
		enqueuedAt: time.Now(),
	}

	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return false
	}
	select {
	case d.jobs <- job:
		return true
	default:
		return false
	}
}

func boundedCommandLogEntry(entry CommandLogEntry) CommandLogEntry {
	return CommandLogEntry{
		Platform: boundedTelemetryField(entry.Platform, 32),
		PID:      boundedTelemetryField(entry.PID, 64),
		GID:      boundedTelemetryField(entry.GID, 128),
		UID:      boundedTelemetryField(entry.UID, 128),
		Command:  boundedTelemetryField(entry.Command, 128),
	}
}

func boundedTelemetryField(value string, maxBytes int) string {
	value = strings.TrimSpace(value)
	if maxBytes <= 0 {
		return ""
	}
	if len(value) > maxBytes {
		value = value[:maxBytes]
		for len(value) > 0 && !utf8.ValidString(value) {
			value = value[:len(value)-1]
		}
	}
	// Detach short fields from the potentially large request-body allocation.
	return strings.Clone(value)
}

func (d *CommandTelemetryDispatcher) run() {
	defer close(d.done)
	for job := range d.jobs {
		startedAt := time.Now()
		writeCtx, cancel := context.WithTimeout(job.ctx, commandTelemetryWriteTimeout)
		err := RecordCommandTelemetry(writeCtx, d.client, job.botID, job.entry)
		cancel()

		attrs := []any{
			"event", "bot_command_telemetry",
			"command", job.entry.Command,
			"queue_delay_ms", commandtrace.Milliseconds(startedAt.Sub(job.enqueuedAt)),
			"write_duration_ms", commandtrace.Milliseconds(time.Since(startedAt)),
		}
		if err != nil {
			attrs = append(attrs,
				"outcome", "error",
				"error_type", fmt.Sprintf("%T", err),
			)
			commandTelemetryLogger.WarnContext(job.ctx, "bot command telemetry completed", attrs...)
			continue
		}
		attrs = append(attrs, "outcome", "ok")
		commandTelemetryLogger.InfoContext(job.ctx, "bot command telemetry completed", attrs...)
	}
}

// Close stops accepting work and gives queued telemetry a bounded interval to
// flush. A database outage must not hold process shutdown for queue_size × the
// per-write timeout.
func (d *CommandTelemetryDispatcher) Close() {
	if d == nil {
		return
	}
	d.closeOnce.Do(func() {
		d.mu.Lock()
		d.closed = true
		close(d.jobs)
		d.mu.Unlock()
	})
	timer := time.NewTimer(commandTelemetryFlushTimeout)
	defer timer.Stop()
	select {
	case <-d.done:
	case <-timer.C:
		commandTelemetryLogger.Warn("bot command telemetry flush timed out",
			"event", "bot_command_telemetry_flush",
			"queued_jobs", len(d.jobs),
		)
	}
}
