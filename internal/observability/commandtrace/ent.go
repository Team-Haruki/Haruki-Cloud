package commandtrace

import (
	"context"

	"entgo.io/ent"
)

// EntQueryInterceptor records every executed Ent query under one
// low-cardinality operation name. Query details deliberately stay out of the
// metric name.
func EntQueryInterceptor() ent.Interceptor {
	return ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			finish := MeasureOperation(ctx, "db.query")
			defer finish()
			return next.Query(ctx, query)
		})
	})
}

// EntMutationHook records every executed Ent create/update/delete operation.
func EntMutationHook() ent.Hook {
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			finish := MeasureOperation(ctx, "db.mutate")
			defer finish()
			return next.Mutate(ctx, mutation)
		})
	}
}
