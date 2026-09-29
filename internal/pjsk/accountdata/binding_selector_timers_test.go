package accountdata

import (
	"context"
	"haruki-cloud/internal/observability/commandtrace"
	"testing"
)

func TestBindingSelectorTimerIncludesErrorsAndCancellation(t *testing.T) {
	service, client := openAccountCoverageService(t, "selector_timers", accountCoverageValidator{profiles: map[string]string{"jp": "JP"}})
	t.Cleanup(func() { _ = client.Close() })
	if _, err := service.Bind(t.Context(), "qq", "42", "1001"); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"success", "missing", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == "canceled" {
				cancel()
			}
			ctx, trace := commandtrace.WithTrace(ctx)
			selector := "u1"
			if mode == "missing" {
				selector = "u99"
			}
			_, binding, err := service.ResolveUserBindingBySelector(ctx, "qq", "42", "jp", selector)
			if mode == "success" && (err != nil || binding == nil) {
				t.Fatalf("binding=%+v err=%v", binding, err)
			}
			if mode != "success" && err == nil {
				t.Fatal("expected selector failure")
			}
			operations := trace.Snapshot().Operations
			if len(operations) != 1 || operations[0].Name != "binding.selector_resolve" || operations[0].Count != 1 {
				t.Fatalf("selector statistics: %+v", operations)
			}
		})
	}
}
