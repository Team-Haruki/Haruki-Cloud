package common

// ThumbnailOptions controls how a card thumbnail is rendered.
type ThumbnailOptions struct {
	AfterTraining    bool
	TrainedArt       bool
	ThumbnailPath    string
	RareImgPath      string
	TrainRank        *int
	TrainRankImgPath *string
	Level            *int
	BirthdayIconPath *string
	CustomText       *string
	CardLevel        map[string]any
	IsPcard          bool
}
