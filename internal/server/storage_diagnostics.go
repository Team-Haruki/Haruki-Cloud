package server

import (
	"context"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/storage"
	"haruki-cloud/utils/logger"
	"time"
)

// Background work has no command trace. Log changed process counters at a
// bounded interval so prewarming, failover and maintenance remain observable.
func startStorageDiagnostics(ctx context.Context, runtime *renderapp.App, log *logger.Logger) {
	if runtime == nil || runtime.Stores.Runtime == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		previous := map[string]storage.IOMetric{}
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				delta := storageMetricDelta(runtime.Stores.Runtime.Stats(), previous)
				if len(delta) > 0 {
					log.InfoContext(ctx, "storage io interval", "interval_seconds", 300, "max_scope", "process_lifetime", "metrics", delta)
				}
			}
		}
	}()
}

func storageMetricDelta(current []storage.IOMetric, previous map[string]storage.IOMetric) []storage.IOMetric {
	var result []storage.IOMetric
	for _, metric := range current {
		old := previous[metric.Name]
		previous[metric.Name] = metric
		if metric.Count == old.Count {
			continue
		}
		metric.Count -= old.Count
		metric.Duration -= old.Duration
		metric.Value -= old.Value
		// Max remains cumulative since startup; the log labels its scope.
		result = append(result, metric)
	}
	return result
}
