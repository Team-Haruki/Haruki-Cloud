package auth

import (
	"strings"

	"haruki-cloud/api"
	"haruki-cloud/internal/core/authban"

	"github.com/gofiber/fiber/v3"
)

// IPBanAdminPath is the internal admin endpoint of the login IP ban.
const IPBanAdminPath = "/internal/bot/auth-bans"

// errInvalidBanSubject is the internal API's answer to a bad ?ip= value.
const errInvalidBanSubject = "ip must be an IP address or an IPv6 /64"

// ipBanAdminResponse wraps a listing.
type ipBanAdminResponse struct {
	Bans []authban.Ban `json:"bans"`
}

// unbanResponse answers DELETE.
type unbanResponse struct {
	IP        string `json:"ip"`
	WasBanned bool   `json:"was_banned"`
}

// registerIPBanAdminRoutes mounts the operator endpoints behind the internal
// API authorization:
//
//	GET    /internal/bot/auth-bans          active bans
//	GET    /internal/bot/auth-bans?ip=<ip>  everything stored for one address
//	DELETE /internal/bot/auth-bans?ip=<ip>  lift the ban and clear its counters
func registerIPBanAdminRoutes(app *fiber.App, guard *authban.Guard) {
	if guard == nil {
		return
	}
	h := ipBanAdminHandler{guard: guard}
	group := app.Group(IPBanAdminPath, api.VerifyAPIAuthorization())
	group.Get("", h.get)
	group.Delete("", h.unban)
}

type ipBanAdminHandler struct {
	guard *authban.Guard
}

func (h ipBanAdminHandler) get(c fiber.Ctx) error {
	ip := strings.TrimSpace(c.Query("ip"))
	if ip == "" {
		bans, err := h.guard.List(c.Context())
		if err != nil {
			return api.InternalError(c)
		}
		return api.JSONResponse(c, fiber.StatusOK, api.ResponseOK, ipBanAdminResponse{Bans: bans})
	}
	if _, err := authban.NormalizeSubject(ip); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, errInvalidBanSubject)
	}
	status, err := h.guard.Inspect(c.Context(), ip)
	if err != nil {
		return api.InternalError(c)
	}
	return api.JSONResponse(c, fiber.StatusOK, api.ResponseOK, status)
}

func (h ipBanAdminHandler) unban(c fiber.Ctx) error {
	ip := strings.TrimSpace(c.Query("ip"))
	if _, err := authban.NormalizeSubject(ip); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, errInvalidBanSubject)
	}
	subject, wasBanned, err := h.guard.Unban(c.Context(), ip)
	if err != nil {
		return api.InternalError(c)
	}
	return api.JSONResponse(c, fiber.StatusOK, api.ResponseOK, unbanResponse{IP: subject, WasBanned: wasBanned})
}
