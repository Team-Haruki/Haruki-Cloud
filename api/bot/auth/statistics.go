package auth

import (
	"context"
	"time"

	"haruki-cloud/api"
	"haruki-cloud/database/bot"
	"haruki-cloud/database/bot/dailyrequests"
	"haruki-cloud/database/bot/hourlyrequests"
	"haruki-cloud/database/bot/requestsranking"

	"github.com/gofiber/fiber/v3"
)

func (h *StatisticsHandler) RecordStatistics(c fiber.Ctx) error {
	botID := fiber.Params[int](c, "botID", 0)
	if botID <= 0 {
		return api.JSONResponse(c, fiber.StatusBadRequest, "botID required")
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return api.InternalError(c)
	}
	now := time.Now().In(loc)
	ctx := c.Context()
	for _, update := range []func(context.Context) error{
		func(ctx context.Context) error { return h.updateRequestsRanking(ctx, botID) },
		func(ctx context.Context) error { return h.updateHourlyRequests(ctx, now) },
		func(ctx context.Context) error { return h.updateDailyRequests(ctx, now, loc) },
	} {
		if err := update(ctx); err != nil {
			return api.InternalError(c)
		}
	}

	return api.JSONResponse(c, fiber.StatusOK, "Statistics recorded")
}

func (h *StatisticsHandler) updateRequestsRanking(ctx context.Context, botID int) error {
	return incrementStatisticCounter(
		func() error {
			_, err := h.svc.client.RequestsRanking.
				Create().
				SetBotID(botID).
				SetCounts(1).
				Save(ctx)
			return err
		},
		func() (int, error) {
			return h.svc.client.RequestsRanking.
				Update().
				Where(requestsranking.BotIDEQ(botID)).
				AddCounts(1).
				Save(ctx)
		},
	)
}

func (h *StatisticsHandler) updateHourlyRequests(ctx context.Context, now time.Time) error {
	hourKey := now.Truncate(time.Hour)
	return incrementStatisticCounter(
		func() error {
			_, err := h.svc.client.HourlyRequests.
				Create().
				SetHourKey(hourKey).
				SetCount(1).
				Save(ctx)
			return err
		},
		func() (int, error) {
			return h.svc.client.HourlyRequests.
				Update().
				Where(hourlyrequests.HourKeyEQ(hourKey)).
				AddCount(1).
				Save(ctx)
		},
	)
}

func (h *StatisticsHandler) updateDailyRequests(ctx context.Context, now time.Time, loc *time.Location) error {
	dateKey := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	return incrementStatisticCounter(
		func() error {
			_, err := h.svc.client.DailyRequests.
				Create().
				SetDateKey(dateKey).
				SetCount(1).
				Save(ctx)
			return err
		},
		func() (int, error) {
			return h.svc.client.DailyRequests.
				Update().
				Where(dailyrequests.DateKeyEQ(dateKey)).
				AddCount(1).
				Save(ctx)
		},
	)
}

func incrementStatisticCounter(create func() error, update func() (int, error)) error {
	updated, err := update()
	if err != nil {
		return err
	}
	if updated > 0 {
		return nil
	}

	if err := create(); err != nil {
		if !bot.IsConstraintError(err) {
			return err
		}
		_, err = update()
		return err
	}
	return nil
}

func registerStatisticsRoutes(app *fiber.App, client *bot.Client) {
	svc := NewStatisticsService(client)
	h := NewStatisticsHandler(svc)

	app.Post("/bot/statistics/record/:botID", api.VerifyAPIAuthorization(), h.RecordStatistics)
}
