package commandtrace

import (
	"context"
	"testing"

	"entgo.io/ent"
)

type testMutation struct {
	ent.Mutation
}

func TestEntInstrumentationRecordsQueriesAndMutations(t *testing.T) {
	ctx, trace := WithTrace(context.Background())
	querier := EntQueryInterceptor().Intercept(ent.QuerierFunc(func(context.Context, ent.Query) (ent.Value, error) {
		return "ok", nil
	}))
	if _, err := querier.Query(ctx, struct{}{}); err != nil {
		t.Fatalf("query: %v", err)
	}

	mutator := EntMutationHook()(ent.MutateFunc(func(context.Context, ent.Mutation) (ent.Value, error) {
		return "ok", nil
	}))
	if _, err := mutator.Mutate(ctx, testMutation{}); err != nil {
		t.Fatalf("mutate: %v", err)
	}

	counts := make(map[string]int)
	for _, operation := range trace.Snapshot().Operations {
		counts[operation.Name] = operation.Count
	}
	if counts["db.query"] != 1 {
		t.Fatalf("db.query count = %d", counts["db.query"])
	}
	if counts["db.mutate"] != 1 {
		t.Fatalf("db.mutate count = %d", counts["db.mutate"])
	}
}
