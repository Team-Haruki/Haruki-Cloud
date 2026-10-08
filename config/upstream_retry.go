package config

import "time"

// UpstreamRetryConfig is the retry block of sekai_api, toolbox and tracker.
// Zero fields take the defaults below; a negative max_retries disables
// retries and a zero or negative budget leaves retries unbounded in time.
type UpstreamRetryConfig struct {
	// MaxRetries is the number of extra attempts after the first one.
	MaxRetries int `yaml:"max_retries"`
	// Wait is the first backoff; later waits grow up to MaxWait.
	Wait    time.Duration `yaml:"wait"`
	MaxWait time.Duration `yaml:"max_wait"`
	// Budget, when positive, is the time after the first attempt started
	// past which no retry is started. Off by default.
	Budget time.Duration `yaml:"budget"`
}

// The defaults are the retry behaviour Cloud ran with through 3.7.18: up to
// four retries on any 5xx answer or transport failure (client timeouts
// included), 1 s first backoff, 2 s maximum, no budget. 3.8.0 cut this to one
// retry of 502/503 and refused/reset connections within 3 s, which turned
// transient SekaiAPI and Toolbox 500/504s and timeouts into command errors.
const (
	DefaultUpstreamRetryMaxRetries = 4
	DefaultUpstreamRetryWait       = time.Second
	DefaultUpstreamRetryMaxWait    = 2 * time.Second
)

// WithDefaults fills the zero fields.
func (c UpstreamRetryConfig) WithDefaults() UpstreamRetryConfig {
	if c.MaxRetries == 0 {
		c.MaxRetries = DefaultUpstreamRetryMaxRetries
	}
	if c.Wait == 0 {
		c.Wait = DefaultUpstreamRetryWait
	}
	if c.MaxWait == 0 {
		c.MaxWait = DefaultUpstreamRetryMaxWait
	}
	if c.MaxWait < c.Wait {
		c.MaxWait = c.Wait
	}
	return c
}

func envRetry(prefix string, dst *UpstreamRetryConfig) {
	envInt(prefix+"_RETRY_MAX_RETRIES", &dst.MaxRetries)
	envDuration(prefix+"_RETRY_WAIT", &dst.Wait)
	envDuration(prefix+"_RETRY_MAX_WAIT", &dst.MaxWait)
	envDuration(prefix+"_RETRY_BUDGET", &dst.Budget)
}
