//go:build !cgo || !pjsk_deck_cgo

package deck

import "fmt"

type stubLocalEngineProvider struct{}

func newLocalEngineProvider(RecommendConfig) localEngineProvider {
	return stubLocalEngineProvider{}
}

func (stubLocalEngineProvider) Get(region string) (DeckRecommender, error) {
	return nil, fmt.Errorf("deck local engine is not available in this build; rebuild with CGO_ENABLED=1 and -tags pjsk_deck_cgo")
}
