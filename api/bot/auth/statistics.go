package auth

import (
	"context"

	"haruki-cloud/api"
	"haruki-cloud/database/bot"
	"haruki-cloud/database/bot/requestsranking"
	"haruki-cloud/internal/cluster"

	"github.com/gofiber/fiber/v3"
)

func (h *StatisticsHandler) RecordStatistics(c fiber.Ctx) error {
	botID := fiber.Params[int](c, "botID", 0)
	if botID <= 0 {
		return api.JSONResponse(c, fiber.StatusBadRequest, "缺少 botID")
	}
	if err := RecordRequestStatistics(c.Context(), h.svc.client, botID, telemetryNow()); err != nil {
		return api.InternalError(c)
	}

	return api.JSONResponse(c, fiber.StatusOK, "统计已记录")
}

func (h *StatisticsHandler) updateRequestsRanking(ctx context.Context, botID int) error {
	if h == nil || h.svc == nil {
		return nil
	}
	if cluster.IsReadOnly() {
		return nil
	}
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

	app.Post("/internal/bot/statistics/record/:botID", api.VerifyAPIAuthorization(), h.RecordStatistics)
}
