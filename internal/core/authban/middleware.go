package authban

import (
	"strconv"
	"strings"
	"time"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/middleware/secure"

	"github.com/gofiber/fiber/v3"
)

// localOutcome carries the login handler's verdict to LoginMiddleware.
const localOutcome = "authban_outcome"

type outcome struct {
	failed bool
	reason Reason
}

// MarkFailure records that the login handler rejected the request for
// reason. Rejections that are not the client's fault (server errors, rate
// limits, banned owners) are simply not marked.
func MarkFailure(c fiber.Ctx, reason Reason) {
	c.Locals(localOutcome, outcome{failed: true, reason: reason})
}

// MarkSuccess records a successful login.
func MarkSuccess(c fiber.Ctx) {
	c.Locals(localOutcome, outcome{})
}

// bannedMessage is the plaintext body of a 429 sent before the Noise
// handshake (catalog account.api.auth_banned). It never echoes the address.
var bannedMessage = i18n.T("account.api.auth_banned")

func reject(c fiber.Ctx, retryAfter time.Duration) error {
	c.Set(fiber.HeaderRetryAfter, strconv.FormatInt(ceilSeconds(retryAfter), 10))
	return c.Status(fiber.StatusTooManyRequests).SendString(bannedMessage)
}

// LoginMiddleware guards the AuthV3 login route. Mount it before the Noise
// middleware: a banned address is refused before any handshake, database or
// bcrypt work. After the chain it counts a failed handshake or a failure the
// handler marked, and remembers a successful bot. botIDParam names the route
// parameter that holds the bot id.
func (g *Guard) LoginMiddleware(botIDParam string) fiber.Handler {
	return func(c fiber.Ctx) error {
		if g == nil {
			return c.Next()
		}
		ctx := c.Context()
		ip := c.IP()
		botID := c.Params(botIDParam)
		if d := g.Check(ctx, ip, botID); d.Banned {
			return reject(c, d.RetryAfter)
		}
		err := c.Next()
		if failed, _ := c.Locals(secure.LocalHandshakeFailed).(bool); failed {
			g.RecordFailure(ctx, ip, botID, ReasonHandshake)
			return err
		}
		if o, ok := c.Locals(localOutcome).(outcome); ok {
			if o.failed {
				g.RecordFailure(ctx, ip, botID, o.reason)
			} else {
				g.RecordSuccess(ctx, ip, botID)
			}
		}
		return err
	}
}

// botRoutePrefixes are the bot route families covered by
// BotRoutesMiddleware: /api/<version>/bot/<bot_id>/...
var botRoutePrefixes = []string{"/api/v2/bot/", "/api/v3/bot/"}

// BotRoutesMiddleware refuses every other bot route from a banned address
// (commands, manifest, logout). It is meant for app.Use when
// Config.BlockBotRoutes is set; the login route keeps its own
// LoginMiddleware and is skipped here. Bots known to the address stay
// admitted like on the login route.
func (g *Guard) BotRoutesMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		if g == nil {
			return c.Next()
		}
		botID, ok := botRouteBotID(c.Path())
		if !ok {
			return c.Next()
		}
		if d := g.Check(c.Context(), c.IP(), botID); d.Banned {
			return reject(c, d.RetryAfter)
		}
		return c.Next()
	}
}

// botRouteBotID returns the bot id of a covered bot route, or false for any
// other path and for the AuthV3 login route.
func botRouteBotID(path string) (string, bool) {
	for _, prefix := range botRoutePrefixes {
		rest, found := strings.CutPrefix(path, prefix)
		if !found {
			continue
		}
		botID, tail, _ := strings.Cut(rest, "/")
		if botID == "" {
			return "", false
		}
		if prefix == "/api/v3/bot/" && strings.Trim(tail, "/") == "auth" {
			return "", false
		}
		return botID, true
	}
	return "", false
}
