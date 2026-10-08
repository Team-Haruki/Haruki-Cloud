package config

import "haruki-cloud/internal/core/dbpool"

// Default MaxOpen per pool. Every pool also defaults MaxIdle to MaxOpen,
// ConnMaxLifetime to 30m and ConnMaxIdleTime to 5m (dbpool.Defaults).
//
// With five regions the worst case is sekai 16 + 5x3 mysekai + 5x1 provider
// = 36, pjsk 10, users 8, bot 10, censor 4, chunithm 2x4 and the image cache
// index 8 (pjsk_render.image_cache.pg_max_open): 84 connections per Cloud
// process, well under PostgreSQL max_connections 200 that it shares with the
// tracker, master ingest and the Drawing image-cache writers.
const (
	DefaultSekaiPoolMaxOpen         = 16
	DefaultSekaiMySekaiPoolMaxOpen  = 3
	DefaultSekaiProviderPoolMaxOpen = 1
	DefaultPJSKPoolMaxOpen          = 10
	DefaultUsersPoolMaxOpen         = 8
	DefaultBotPoolMaxOpen           = 10
	DefaultCensorPoolMaxOpen        = 4
	DefaultChunithmPoolMaxOpen      = 4
)

// DBPool returns pjsk.pool with defaults applied.
func (c PJSKConfig) DBPool() dbpool.Config {
	return c.Pool.WithDefaults(dbpool.Defaults(DefaultPJSKPoolMaxOpen))
}

// DBPool returns sekai.pool with defaults applied.
func (c SekaiConfig) DBPool() dbpool.Config {
	return c.Pool.WithDefaults(dbpool.Defaults(DefaultSekaiPoolMaxOpen))
}

// MySekaiDBPool returns sekai.mysekai_pool with defaults applied.
func (c SekaiConfig) MySekaiDBPool() dbpool.Config {
	return c.MySekaiPool.WithDefaults(dbpool.Defaults(DefaultSekaiMySekaiPoolMaxOpen))
}

// ProviderDBPool returns sekai.provider_pool with defaults applied.
func (c SekaiConfig) ProviderDBPool() dbpool.Config {
	return c.ProviderPool.WithDefaults(dbpool.Defaults(DefaultSekaiProviderPoolMaxOpen))
}

// DBPool returns users_db.pool with defaults applied.
func (c UsersDBConfig) DBPool() dbpool.Config {
	return c.Pool.WithDefaults(dbpool.Defaults(DefaultUsersPoolMaxOpen))
}

// DBPool returns haruki_bot.pool with defaults applied.
func (c HarukiBotDBConfig) DBPool() dbpool.Config {
	return c.Pool.WithDefaults(dbpool.Defaults(DefaultBotPoolMaxOpen))
}

// DBPool returns censor.censor_db_pool with defaults applied.
func (c CensorConfig) DBPool() dbpool.Config {
	return c.CensorDBPool.WithDefaults(dbpool.Defaults(DefaultCensorPoolMaxOpen))
}

// MusicDBPoolConfig returns chunithm.music_db_pool with defaults applied.
func (c ChunithmConfig) MusicDBPoolConfig() dbpool.Config {
	return c.MusicDBPool.WithDefaults(dbpool.Defaults(DefaultChunithmPoolMaxOpen))
}

// BindingDBPoolConfig returns chunithm.binding_db_pool with defaults applied.
func (c ChunithmConfig) BindingDBPoolConfig() dbpool.Config {
	return c.BindingDBPool.WithDefaults(dbpool.Defaults(DefaultChunithmPoolMaxOpen))
}
