package app

import (
	"context"
	"strings"
	"time"

	pjskDB "haruki-cloud/database/pjsk"
	sekaiDB "haruki-cloud/database/sekai"
	pjskalias "haruki-cloud/internal/pjsk/alias"
	"haruki-cloud/internal/pjsk/meta"
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
	"haruki-cloud/internal/pjsk/render/vlive"
	accountdata "haruki-cloud/internal/pjsk/userdata"
	"haruki-cloud/utils/censor"
	"haruki-cloud/utils/drawing"
	"haruki-cloud/utils/imagecache"
	sekaiutil "haruki-cloud/utils/sekai"
)

type Config struct {
	DefaultRegion     renderregion.Value
	DrawingBaseURL    string
	DrawingTimeout    time.Duration
	DrawingRetryCount int
	DrawingCache      drawing.RenderCacheConfig
	ImageCacheURI     string
	ImageCacheDir     string
	ImageCachePGURL   string // PostgreSQL DSN for image cache deduplication (optional)
	CensorService     *censor.Service
	AssetPrimaryDir   string
	AssetLegacyDirs   []string
	LocalMasterdata   LocalMasterdataConfig
	UserSnapshot      UserSnapshotConfig
	MetaLoader        *meta.Loader
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
	Sekai      *sekaiDB.Client
	PJSK       *pjskDB.Client
	Drawing    *drawing.HarukiDrawingClient
	Assets     *assets.AssetHelper
	MetaLoader *meta.Loader
	Cards      *card.Controller
	Decks      *deck.Controller
	Edu        *education.Controller
	Events     *event.Controller
	Gachas     *gacha.Controller
	Honors     *honor.Controller
	Misc       *misc.Controller
	MySekai    *mysekai.Controller
	Music      *music.Controller
	Aliases    *pjskalias.Service
	Profiles   *profile.Controller
	Score      *score.Controller
	SK         *sk.Controller
	Stamps     *stamp.Controller
	VLive      *vlive.Controller
	Bindings   *accountdata.BindingService
	BanChecker *accountdata.BanService
	ImageCache *imagecache.Client
	Censor     *censor.Service
	Config     Config
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
		drawingClient.SetRenderCache(drawing.NewRenderCacheClient(cfg.DrawingCache))
	}

	miscController := misc.NewController(drawingClient)
	mysekaiController := (*mysekai.Controller)(nil)
	musicController := (*music.Controller)(nil)
	deckController := deck.NewController(nil, nil, drawingClient, assetHelper, snapshotService, cfg.DefaultRegion)
	educationController := education.NewController(drawingClient, assetHelper, snapshotService, cfg.DefaultRegion)
	scoreController := score.NewController(drawingClient)
	skController := sk.NewController(drawingClient)
	skController.SetTrackerIntegration(sekaiutil.GetTrackerClient(), nil, assetHelper)
	if snapshotService != nil {
		mysekaiController = mysekai.NewController(drawingClient, snapshotService, cfg.LocalMasterdata.Dir, cfg.DefaultRegion)
	}

	var cardController *card.Controller
	var eventController *event.Controller
	var gachaController *gacha.Controller
	var honorController *honor.Controller
	var profileController *profile.Controller
	var stampController *stamp.Controller
	var vliveController *vlive.Controller
	if sekaiClient != nil {
		cardSource := card.NewCloudSource(sekaiClient, cfg.DefaultRegion)
		eventSource := event.NewCloudSource(sekaiClient, cfg.DefaultRegion)
		skController.SetTrackerIntegration(sekaiutil.GetTrackerClient(), eventSource, assetHelper)
		deckController = deck.NewController(cardSource, eventSource, drawingClient, assetHelper, snapshotService, cfg.DefaultRegion)
		cardController = card.NewController(cardSource, eventSource, drawingClient, assetHelper)
		educationController.RegisterSource(education.NewCloudSource(sekaiClient, cfg.DefaultRegion))
		eventController = event.NewController(eventSource, drawingClient, assetHelper)
		gachaController = gacha.NewController(gacha.NewCloudSource(sekaiClient, cfg.DefaultRegion), drawingClient, assetHelper)
		honorController = honor.NewController(honor.NewCloudSource(sekaiClient, cfg.DefaultRegion), drawingClient, assetHelper)
		musicController = music.NewController(music.NewCloudSource(sekaiClient, cfg.DefaultRegion), drawingClient, assetHelper, snapshotService, cfg.MetaLoader)
		profileController = profile.NewController(profile.NewCloudSource(sekaiClient, cfg.DefaultRegion), drawingClient, assetHelper, snapshotService)
		stampController = stamp.NewController(stamp.NewCloudSource(sekaiClient, cfg.DefaultRegion), drawingClient, assetHelper)
		vliveController = vlive.NewController(vlive.NewCloudSource(sekaiClient, cfg.DefaultRegion), cfg.DefaultRegion)
	}

	var imgStore *imagecache.PGStore
	if cfg.ImageCachePGURL != "" {
		if store, err := imagecache.NewPGStore(cfg.ImageCachePGURL); err == nil {
			_ = store.Init(context.Background())
			imgStore = store
		}
	}

	if cfg.CensorService != nil {
		skController.SetCensor(cfg.CensorService)
		if profileController != nil {
			profileController.SetCensor(cfg.CensorService)
		}
	}

	return &App{
		Sekai:      sekaiClient,
		PJSK:       pjskClient,
		Drawing:    drawingClient,
		Assets:     assetHelper,
		MetaLoader: cfg.MetaLoader,
		Cards:      cardController,
		Decks:      deckController,
		Edu:        educationController,
		Events:     eventController,
		Gachas:     gachaController,
		Honors:     honorController,
		Misc:       miscController,
		MySekai:    mysekaiController,
		Music:      musicController,
		Profiles:   profileController,
		Score:      scoreController,
		SK:         skController,
		Stamps:     stampController,
		VLive:      vliveController,
		ImageCache: imagecache.NewWithStore(cfg.ImageCacheURI, cfg.ImageCacheDir, imgStore),
		Censor:     cfg.CensorService,
		Config:     cfg,
	}
}

func (a *App) AssetRoots() []string {
	if a == nil || a.Assets == nil {
		return nil
	}
	return a.Assets.Roots()
}
