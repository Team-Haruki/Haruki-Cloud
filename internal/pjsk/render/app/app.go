package app

import (
	"strings"
	"time"

	pjskDB "haruki-cloud/database/pjsk"
	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/event"
	"haruki-cloud/internal/pjsk/render/gacha"
	"haruki-cloud/internal/pjsk/render/honor"
	"haruki-cloud/internal/pjsk/render/misc"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/score"
	"haruki-cloud/internal/pjsk/render/stamp"
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
	Sekai   *sekaiDB.Client
	PJSK    *pjskDB.Client
	Drawing *drawing.HarukiDrawingClient
	Assets  *assets.AssetHelper
	Events  *event.Controller
	Gachas  *gacha.Controller
	Honors  *honor.Controller
	Misc    *misc.Controller
	Score   *score.Controller
	Stamps  *stamp.Controller
	Config  Config
}

func New(sekaiClient *sekaiDB.Client, pjskClient *pjskDB.Client, cfg Config) *App {
	cfg.DefaultRegion = renderregion.WithDefault(cfg.DefaultRegion)

	assetHelper := assets.NewAssetHelper(cfg.AssetPrimaryDir, cfg.AssetLegacyDirs)

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
	scoreController := score.NewController(drawingClient)

	var eventController *event.Controller
	var gachaController *gacha.Controller
	var honorController *honor.Controller
	var stampController *stamp.Controller
	if sekaiClient != nil {
		eventController = event.NewController(event.NewCloudSource(sekaiClient, cfg.DefaultRegion), drawingClient, assetHelper)
		gachaController = gacha.NewController(gacha.NewCloudSource(sekaiClient, cfg.DefaultRegion), drawingClient, assetHelper)
		honorController = honor.NewController(honor.NewCloudSource(sekaiClient, cfg.DefaultRegion), drawingClient, assetHelper)
		stampController = stamp.NewController(stamp.NewCloudSource(sekaiClient, cfg.DefaultRegion), drawingClient, assetHelper)
	}

	return &App{
		Sekai:   sekaiClient,
		PJSK:    pjskClient,
		Drawing: drawingClient,
		Assets:  assetHelper,
		Events:  eventController,
		Gachas:  gachaController,
		Honors:  honorController,
		Misc:    miscController,
		Score:   scoreController,
		Stamps:  stampController,
		Config:  cfg,
	}
}

func (a *App) AssetRoots() []string {
	if a == nil || a.Assets == nil {
		return nil
	}
	return a.Assets.Roots()
}
