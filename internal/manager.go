package koolo

import (
	"fmt"
	"log/slog"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/hectorgimenez/koolo/cmd/koolo/log"
	"github.com/hectorgimenez/koolo/internal/action"
	"github.com/hectorgimenez/koolo/internal/character"
	"github.com/hectorgimenez/koolo/internal/config"
	"github.com/hectorgimenez/koolo/internal/container"
	"github.com/hectorgimenez/koolo/internal/event"
	"github.com/hectorgimenez/koolo/internal/game"
	"github.com/hectorgimenez/koolo/internal/health"
	"github.com/hectorgimenez/koolo/internal/pather"
	"github.com/hectorgimenez/koolo/internal/run"
	"github.com/hectorgimenez/koolo/internal/town"
	"github.com/hectorgimenez/koolo/internal/ui"
	"github.com/lxn/win"
)

type SupervisorManager struct {
	logger        *slog.Logger
	supervisors   map[string]Supervisor
	eventListener *event.Listener
}

func NewSupervisorManager(logger *slog.Logger, eventListener *event.Listener) *SupervisorManager {
	return &SupervisorManager{
		logger:        logger,
		supervisors:   make(map[string]Supervisor),
		eventListener: eventListener,
	}
}

func (mng *SupervisorManager) AvailableSupervisors() []string {
	availableSupervisors := make([]string, 0)
	for name := range config.Characters {
		if name != "template" {
			availableSupervisors = append(availableSupervisors, name)
		}
	}

	return availableSupervisors
}

func (mng *SupervisorManager) Start(supervisorName string) error {
	// Reload config to get the latest local changes before starting the supervisor
	err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	supervisorLogger, err := log.NewLogger(config.Koolo.Debug.Log, config.Koolo.LogSaveDirectory, supervisorName)
	if err != nil {
		return err
	}

	supervisor, err := mng.buildSupervisor(supervisorName, supervisorLogger)
	if err != nil {
		return err
	}
	mng.supervisors[supervisorName] = supervisor

	if config.Koolo.GameWindowArrangement {
		go func() {
			// When the game starts, its doing some weird stuff like repositioning and resizing window automatically
			// we need to wait until this is done in order to reposition, or it will be overridden
			time.Sleep(time.Second * 5)
			mng.rearrangeWindows()
		}()
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				mng.logger.Error(
					"fatal error detected, forcing shutdown",
					slog.String("supervisor", supervisorName),
					slog.Any("error", r),
					slog.String("stacktrace", string(debug.Stack())),
				)
				mng.Stop(supervisorName)
			}
		}()

		err = supervisor.Start()
		if err != nil {
			mng.logger.Error(fmt.Sprintf("error running supervisor %s: %s", supervisorName, err.Error()))
		}
	}()

	return nil
}

func (mng *SupervisorManager) StopAll() {
	for _, s := range mng.supervisors {
		s.Stop()
	}
}

func (mng *SupervisorManager) Stop(supervisor string) {
	s, found := mng.supervisors[supervisor]
	if found {
		s.Stop()
		delete(mng.supervisors, supervisor)
	}
}

func (mng *SupervisorManager) TogglePause(supervisor string) {
	s, found := mng.supervisors[supervisor]
	if found {
		s.TogglePause()
	}
}

func (mng *SupervisorManager) Status(characterName string) Stats {
	for name, supervisor := range mng.supervisors {
		if name == characterName {
			return supervisor.Stats()
		}
	}

	return Stats{}
}

func (mng *SupervisorManager) GetData(characterName string) game.Data {
	for name, supervisor := range mng.supervisors {
		if name == characterName {
			return supervisor.GetData()
		}
	}

	return game.Data{}
}

func (mng *SupervisorManager) buildSupervisor(supervisorName string, logger *slog.Logger) (Supervisor, error) {
	cfg, found := config.Characters[supervisorName]
	if !found {
		return nil, fmt.Errorf("character %s not found", supervisorName)
	}

	pid, hwnd, err := game.StartGame(cfg.Username, cfg.Password, cfg.AuthMethod, cfg.Realm, cfg.CommandLineArgs, config.Koolo.UseCustomSettings)
	if err != nil {
		return nil, fmt.Errorf("error starting game: %w", err)
	}

	gr, err := game.NewGameReader(cfg, supervisorName, pid, hwnd, logger)
	if err != nil {
		return nil, fmt.Errorf("error creating game reader: %w", err)
	}

	gi, err := game.InjectorInit(logger, gr.GetPID())
	if err != nil {
		return nil, fmt.Errorf("error creating game injector: %w", err)
	}

	hidM := game.NewHID(gr, gi)
	bm := health.NewBeltManager(logger, hidM, cfg, supervisorName)
	gm := game.NewGameManager(gr, hidM, supervisorName)
	hm := health.NewHealthManager(logger, bm, gm, cfg)
	pf := pather.NewPathFinder(gr, hidM, cfg)
	c := container.Container{
		Supervisor:    supervisorName,
		Logger:        logger,
		Reader:        gr,
		HID:           hidM,
		Injector:      gi,
		Manager:       gm,
		PathFinder:    pf,
		CharacterCfg:  cfg,
		EventListener: mng.eventListener,
		UIManager:     ui.NewManager(gr),
	}
	sm := town.NewShopManager(logger, bm, c)

	char, err := character.BuildCharacter(logger, c)
	if err != nil {
		return nil, fmt.Errorf("error creating character: %w", err)
	}

	ab := action.NewBuilder(c, sm, bm, char)
	bot := NewBot(logger, hm, ab, c, supervisorName)
	runFactory := run.NewFactory(logger, ab, char, bm, c)

	statsHandler := NewStatsHandler(supervisorName, logger)
	mng.eventListener.Register(statsHandler.Handle)

	if config.Characters[supervisorName].Companion.Enabled {
		return NewCompanionSupervisor(supervisorName, bot, runFactory, statsHandler, c)
	}

	return NewSinglePlayerSupervisor(supervisorName, bot, runFactory, statsHandler, c)
}

func (mng *SupervisorManager) rearrangeWindows() {
	width := win.GetSystemMetrics(0)
	height := win.GetSystemMetrics(1)
	var windowBorderX int32 = 2   // left + right window border is 2px
	var windowBorderY int32 = 40  // upper window border is usually 40px
	var windowOffsetX int32 = -10 // offset horizontal window placement by -10 pixel
	maxColumns := width / (1280 + windowBorderX)
	maxRows := height / (720 + windowBorderY)

	mng.logger.Debug(
		"Arranging windows",
		slog.String("displaywidth", strconv.FormatInt(int64(width), 10)),
		slog.String("displayheight", strconv.FormatInt(int64(height), 10)),
		slog.String("max columns", strconv.FormatInt(int64(maxColumns+1), 10)), // +1 as we are counting from 0
		slog.String("max rows", strconv.FormatInt(int64(maxRows+1), 10)),
	)

	var column, row int32
	for _, sp := range mng.supervisors {
		// reminder that columns are vertical (they go up and down) and rows are horizontal (they go left and right)
		if column > maxColumns {
			column = 0
			row++
		}

		if row <= maxRows {
			sp.SetWindowPosition(int(column*(1280+windowBorderX)+windowOffsetX), int(row*(720+windowBorderY)))
			mng.logger.Debug(
				"Window Positions",
				slog.String("supervisor", sp.Name()),
				slog.String("column", strconv.FormatInt(int64(column), 10)),
				slog.String("row", strconv.FormatInt(int64(row), 10)),
				slog.String("position", strconv.FormatInt(int64(column*(1280+windowBorderX)+windowOffsetX), 10)+"x"+strconv.FormatInt(int64(row*(720+windowBorderY)), 10)),
			)
			column++
		} else {
			mng.logger.Debug("Window position of supervisor " + sp.Name() + " was not changed, no free space for it")
		}
	}
}
