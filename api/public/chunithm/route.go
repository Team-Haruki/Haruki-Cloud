package chunithm

import (
	"haruki-cloud/database/chunithm/maindb"
	"haruki-cloud/database/chunithm/music"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

func RegisterChunithmRoutes(app fiber.Router, mainClient *maindb.Client, musicClient *music.Client, redisClient *redis.Client) {
	group := app.Group("/api/v2/public/chunithm")
	registerAliasRoutes(group, mainClient, redisClient)
	registerMusicRoutes(group, musicClient, redisClient)
}
