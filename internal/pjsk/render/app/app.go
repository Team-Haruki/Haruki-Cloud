package app

import (
	"strings"
	"time"

	pjskDB "haruki-cloud/database/pjsk"
	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/card"
	"haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/education"
	"haruki-cloud/internal/pjsk/render/event"
	"haruki-cloud/internal/pjsk/render/gacha"
	"haruki-cloud/internal/pjsk/render/honor"
	"haruki-cloud/internal/pjsk/render/misc"
	"haruki-cloud/internal/pjsk/render/music"
	"haruki-cloud/internal/pjsk/render/mysekai"
	"haruki-cloud/internal/pjsk/render/profile"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/score"
	"haruki-cloud/internal/pjsk/render/sk"
	"haruki-cloud/internal/pjsk/render/stamp"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"
)

type Config struct {
	DefaultRegion     renderregion.Value
	DrawingBaseURL    string
	DrawingTimeout    time.Duration
	DrawingRetryCount int
	AssetPrimaryDir   string
	AssetLegacyDirs   []string
	LocalMasterdata   LocalMasterdataConfig
	UserSnapshot      UserSnapshotConfig
	DeckRecommend     DeckRecommendConfig
}

type LocalMasterdataConfig struct {
	Enabled bool
	Dir     string
}

type UserSnapshotConfig struct {
	Provider      string
	UserJSON      string
	MusicMetaJSON string
	MySekaiJSON   string
}

type DeckRecommendConfig struct {
	Enabled        bool
	UseLocalEngine bool
	Timeout        time.Duration
	DefaultAlgs    []string
}

type App struct {
	Sekai    *sekaiDB.Client
	PJSK     *pjskDB.Client
	Drawing  *drawing.HarukiDrawingClient
	Assets   *assets.AssetHelper
	Cards    *card.Controller
	Decks    *deck.Controller
	Edu      *education.Controller
	Events   *event.Controller
	Gachas   *gacha.Controller
	Honors   *honor.Controller
	Misc     *misc.Controller
	MySekai  *mysekai.Controller
	Music    *music.Controller
	Profiles *profile.Controller
	Score    *score.Controller
	SK       *sk.Controller
	Stamps   *stamp.Controller
	Config   Config
}

func New(sekaiClient *sekaiDB.Client, pjskClient *pjskDB.Client, cfg Config) *App {
	cfg.DefaultRegion = renderregion.WithDefault(cfg.DefaultRegion)

	assetHelper := assets.NewAssetHelper(cfg.AssetPrimaryDir, cfg.AssetLegacyDirs)
	var snapshotService *userdata.Service
	if provider := strings.TrimSpace(cfg.UserSnapshot.Provider); provider == "" || strings.EqualFold(provider, "local_file") {
		snapshotService = userdata.NewLocalFileService(sekaiClient, assetHelper, userdata.LocalFileConfig{
			DefaultRegion: cfg.DefaultRegion,
			UserJSON:      cfg.UserSnapshot.UserJSON,
			MusicMetaJSON: cfg.UserSnapshot.MusicMetaJSON,
			MySekaiJSON:   cfg.UserSnapshot.MySekaiJSON,
		})
	}

	var drawingClient *drawing.HarukiDrawingClient
	if strings.TrimSpace(cfg.DrawingBaseURL) != "" {
		var options []drawing.ClientOption
		if cfg.DrawingTimeout > 0 {
			options = append(options, drawing.WithTimeout(cfg.DrawingTimeout))
		}
		if cfg.DrawingRetryCount > 0 {
			options = append(options, drawing.WithRetryCount(cfg.DrawingRetryCount))
		}
		drawingClient = drawing.NewHarukiDrawingClient(cfg.DrawingBaseURL, options...)
	}

	miscController := misc.NewController(drawingClient)
	mysekaiController := (*mysekai.Controller)(nil)
	musicController := (*music.Controller)(nil)
	deckController := deck.NewController(nil, nil, drawingClient, assetHelper, snapshotService, cfg.DefaultRegion)
	educationController := education.NewController(drawingClient, assetHelper, snapshotService, cfg.DefaultRegion)
	scoreController := score.NewController(drawingClient)
	skController := sk.NewController(drawingClient)
	if snapshotService != nil && cfg.LocalMasterdata.Enabled && strings.TrimSpace(cfg.LocalMasterdata.Dir) != "" {
		mysekaiController = mysekai.NewController(drawingClient, snapshotService, cfg.LocalMasterdata.Dir, cfg.DefaultRegion)
	}

	var cardController *card.Controller
	var eventController *event.Controller
	var gachaController *gacha.Controller
	var honorController *honor.Controller
	var profileController *profile.Controller
	var stampController *stamp.Controller
	if sekaiClient != nil {
		cardSource := card.NewCloudSource(sekaiClient, cfg.DefaultRegion)
		eventSource := event.NewCloudSource(sekaiClient, cfg.DefaultRegion)
		deckController = deck.NewController(cardSource, eventSource, drawingClient, assetHelper, snapshotService, cfg.DefaultRegion)
		cardController = card.NewController(cardSource, eventSource, drawingClient, assetHelper)
		educationController.RegisterSource(education.NewCloudSource(sekaiClient, cfg.DefaultRegion))
		eventController = event.NewController(eventSource, drawingClient, assetHelper)
		gachaController = gacha.NewController(gacha.NewCloudSource(sekaiClient, cfg.DefaultRegion), drawingClient, assetHelper)
		honorController = honor.NewController(honor.NewCloudSource(sekaiClient, cfg.DefaultRegion), drawingClient, assetHelper)
		musicController = music.NewController(music.NewCloudSource(sekaiClient, cfg.DefaultRegion), drawingClient, assetHelper, snapshotService)
		profileController = profile.NewController(profile.NewCloudSource(sekaiClient, cfg.DefaultRegion), drawingClient, assetHelper, snapshotService)
		stampController = stamp.NewController(stamp.NewCloudSource(sekaiClient, cfg.DefaultRegion), drawingClient, assetHelper)
	}

	return &App{
		Sekai:    sekaiClient,
		PJSK:     pjskClient,
		Drawing:  drawingClient,
		Assets:   assetHelper,
		Cards:    cardController,
		Decks:    deckController,
		Edu:      educationController,
		Events:   eventController,
		Gachas:   gachaController,
		Honors:   honorController,
		Misc:     miscController,
		MySekai:  mysekaiController,
		Music:    musicController,
		Profiles: profileController,
		Score:    scoreController,
		SK:       skController,
		Stamps:   stampController,
		Config:   cfg,
	}
}

func (a *App) AssetRoots() []string {
	if a == nil || a.Assets == nil {
		return nil
	}
	return a.Assets.Roots()
}
