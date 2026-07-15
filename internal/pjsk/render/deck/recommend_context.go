package deck

import "context"

type contextualDeckRecommender interface {
	RecommendContext(context.Context, RecommendRequest) (*RecommendResult, error)
}

type contextualBatchDeckRecommender interface {
	RecommendBatchContext(context.Context, RecommendRequest) ([]remoteBatchRecommendResult, error)
}

func normalizeRecommendContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func recommendWithContext(ctx context.Context, recommender PjskDeckRecommender, req RecommendRequest) (*RecommendResult, error) {
	ctx = normalizeRecommendContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if contextual, ok := recommender.(contextualDeckRecommender); ok {
		return contextual.RecommendContext(ctx, req)
	}
	result, err := recommender.Recommend(req)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	return result, err
}

func recommendBatchWithContext(ctx context.Context, recommender rawBatchChallengeRecommender, req RecommendRequest) ([]remoteBatchRecommendResult, error) {
	ctx = normalizeRecommendContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if contextual, ok := recommender.(contextualBatchDeckRecommender); ok {
		return contextual.RecommendBatchContext(ctx, req)
	}
	result, err := recommender.RecommendBatch(req)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	return result, err
}
