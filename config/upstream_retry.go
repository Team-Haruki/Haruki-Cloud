package config

import "time"

// UpstreamRetryConfig is the retry block of sekai_api, toolbox and tracker.
// Zero fields take the defaults below; a negative max_retries or budget
// disables retries or the budget.
type UpstreamRetryConfig struct {
	// MaxRetries is the number of extra attempts after the first one.
	MaxRetries int `yaml:"max_retries"`
	// Wait is the first backoff; later waits grow up to MaxWait.
	Wait    time.Duration `yaml:"wait"`
	MaxWait time.Duration `yaml:"max_wait"`
	// Budget is the time after the first attempt started past which no
	// retry is started, so only fast failures are retried and the worst
	// case stays near Budget plus one attempt timeout.
	Budget time.Duration `yaml:"budget"`
}

// Retries are for transient failures of idempotent GETs only: 502/503 or a
// refused/reset connection, never a client timeout. One retry within 3 s of
// the first attempt keeps the worst case near 3 s + one timeout instead of
// five attempts (about 32 s for SekaiAPI/Toolbox, 107 s for the tracker).
const (
	DefaultUpstreamRetryMaxRetries = 1
	DefaultUpstreamRetryWait       = 200 * time.Millisecond
	DefaultUpstreamRetryMaxWait    = time.Second
	DefaultUpstreamRetryBudget     = 3 * time.Second
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
	if c.Budget == 0 {
		c.Budget = DefaultUpstreamRetryBudget
	}
	return c
}

func envRetry(prefix string, dst *UpstreamRetryConfig) {
	envInt(prefix+"_RETRY_MAX_RETRIES", &dst.MaxRetries)
	envDuration(prefix+"_RETRY_WAIT", &dst.Wait)
	envDuration(prefix+"_RETRY_MAX_WAIT", &dst.MaxWait)
	envDuration(prefix+"_RETRY_BUDGET", &dst.Budget)
}
