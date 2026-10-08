package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"haruki-cloud/database/bot"
	"haruki-cloud/database/bot/dailyrequests"
	"haruki-cloud/database/bot/hourlyrequests"
	"haruki-cloud/database/bot/requestsranking"
	"haruki-cloud/internal/cluster"
)

type CommandLogEntry struct {
	Platform string
	PID      string
	GID      string
	UID      string
	Command  string
}

var (
	telemetryLocationOnce sync.Once
	telemetryLocation     *time.Location
	telemetryLocationErr  error
)

func RecordRequestStatistics(ctx context.Context, client *bot.Client, botID int, now time.Time) error {
	if cluster.IsReadOnly() {
		return nil
	}
	if client == nil || botID <= 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	for _, update := range []func() error{
		func() error {
			return incrementStatisticCounter(
				func() error {
					_, err := client.RequestsRanking.
						Create().
						SetBotID(botID).
						SetCounts(1).
						Save(ctx)
					return err
				},
				func() (int, error) {
					return client.RequestsRanking.
						Update().
						Where(requestsranking.BotIDEQ(botID)).
						AddCounts(1).
						Save(ctx)
				},
			)
		},
		func() error {
			hourKey := now.Truncate(time.Hour)
			return incrementStatisticCounter(
				func() error {
					_, err := client.HourlyRequests.
						Create().
						SetHourKey(hourKey).
						SetCount(1).
						Save(ctx)
					return err
				},
				func() (int, error) {
					return client.HourlyRequests.
						Update().
						Where(hourlyrequests.HourKeyEQ(hourKey)).
						AddCount(1).
						Save(ctx)
				},
			)
		},
		func() error {
			loc := now.Location()
			dateKey := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
			return incrementStatisticCounter(
				func() error {
					_, err := client.DailyRequests.
						Create().
						SetDateKey(dateKey).
						SetCount(1).
						Save(ctx)
					return err
				},
				func() (int, error) {
					return client.DailyRequests.
						Update().
						Where(dailyrequests.DateKeyEQ(dateKey)).
						AddCount(1).
						Save(ctx)
				},
			)
		},
	} {
		if err := update(); err != nil {
			return err
		}
	}
	return nil
}

func RecordCommandLog(ctx context.Context, client *bot.Client, entry CommandLogEntry, now time.Time) error {
	if cluster.IsReadOnly() {
		return nil
	}
	if client == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	_, err := client.CommandLog.
		Create().
		SetPlatform(strings.TrimSpace(entry.Platform)).
		SetPid(strings.TrimSpace(entry.PID)).
		SetGid(strings.TrimSpace(entry.GID)).
		SetUID(strings.TrimSpace(entry.UID)).
		SetCommand(strings.TrimSpace(entry.Command)).
		SetCreatedAt(now).
		Save(ctx)
	return err
}

// RecordCommandTelemetry writes the three request counters and the command
// log row in one transaction (one commit instead of four). If the
// transaction fails before its commit (for example a concurrent insert of a
// new hour/day row aborts it on PostgreSQL), nothing was written and the
// statements are retried one by one, where the counters handle that race.
func RecordCommandTelemetry(ctx context.Context, client *bot.Client, botID int, entry CommandLogEntry) error {
	if client == nil {
		return nil
	}
	now := telemetryNow()
	if cluster.IsReadOnly() {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	committed, err := recordCommandTelemetryTx(ctx, client, botID, entry, now)
	if committed || ctx.Err() != nil {
		return err
	}
	var errs []error
	if err := RecordRequestStatistics(ctx, client, botID, now); err != nil {
		errs = append(errs, err)
	}
	if err := RecordCommandLog(ctx, client, entry, now); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// recordCommandTelemetryTx reports committed=true once Commit was attempted:
// a failed commit is not retried statement by statement, because the
// counters may already have been applied.
func recordCommandTelemetryTx(ctx context.Context, client *bot.Client, botID int, entry CommandLogEntry, now time.Time) (committed bool, err error) {
	tx, err := client.Tx(ctx)
	if err != nil {
		return false, err
	}
	txClient := tx.Client()
	if err := RecordRequestStatistics(ctx, txClient, botID, now); err != nil {
		return false, errors.Join(err, tx.Rollback())
	}
	if err := RecordCommandLog(ctx, txClient, entry, now); err != nil {
		return false, errors.Join(err, tx.Rollback())
	}
	return true, tx.Commit()
}

func telemetryNow() time.Time {
	telemetryLocationOnce.Do(func() {
		telemetryLocation, telemetryLocationErr = time.LoadLocation("Asia/Shanghai")
	})
	if telemetryLocationErr != nil || telemetryLocation == nil {
		return time.Now()
	}
	return time.Now().In(telemetryLocation)
}
