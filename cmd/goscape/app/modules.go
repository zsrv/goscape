package app

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/zsrv/goscape/modules/account"
	"github.com/zsrv/goscape/modules/friends"
	"github.com/zsrv/goscape/modules/hiscore"
	"github.com/zsrv/goscape/modules/login"
	"github.com/zsrv/goscape/modules/ondemand"
	"github.com/zsrv/goscape/modules/world"
	"github.com/zsrv/goscape/pkg/dskit/modules"
	"github.com/zsrv/goscape/pkg/dskit/server"
	"github.com/zsrv/goscape/pkg/dskit/services"
	"github.com/zsrv/goscape/pkg/gamedb"
	"github.com/zsrv/goscape/pkg/tapper"
	"github.com/zsrv/goscape/pkg/util/log"
	"github.com/zsrv/goscape/pkg/world/connhandler"
)

// The various modules that make up goscape.

const (
	// Individual targets

	OnDemand string = "ondemand"
	Friends  string = "friends"
	Login    string = "login"
	World    string = "world"
	Database string = "database"
	Account  string = "account"
	Hiscore  string = "hiscore"

	// Composite targets

	SingleBinary string = "all"
)

func (g *App) initOnDemand() (services.Service, error) {
	if !g.cfg.OnDemand.Enable {
		// TODO: still makes module appear to be running, move the check elsewhere?
		return services.NewIdleService(nil, nil), nil
	}

	logLevel := slog.Level(g.cfg.LogLevel)
	if g.cfg.OnDemand.Server.LogLevel != nil {
		logLevel = *g.cfg.OnDemand.Server.LogLevel
	}

	logger, err := log.NewLogger(logLevel, g.cfg.LogFormat, os.Stdout, log.WithSourceFormat(g.cfg.LogSource))
	if err != nil {
		g.logger.Error("failed to create logger", "module", "ondemand", "err", err)
		os.Exit(1)
	}
	logger = logger.With("component", "ondemand")

	g.cfg.OnDemand.Server.Log = logger

	server.DisableSignalHandling(&g.cfg.OnDemand.Server)
	serv, err := server.New(g.cfg.OnDemand.Server)
	if err != nil {
		return nil, err
	}

	// The dskit DAG declares OnDemand: {Common, World}, so g.world has been
	// initialised by the time initOnDemand runs (when World is enabled). When
	// the world module is disabled, worldConn stays nil and the / WS route
	// falls back to RootHandler only.
	var worldConn connhandler.ConnHandler
	var worldSrv *world.Server
	if g.world != nil && g.world.Server != nil {
		worldConn = g.world.Server
		worldSrv = g.world.Server
	}

	a, err := ondemand.New(g.cfg.OnDemand, logger, serv, worldConn)
	if err != nil {
		// server.New already bound the HTTP listener; ondemand.New failing
		// here (bad cache path, bad source-IP regex) would otherwise leak the
		// socket, since no service ever runs to Shutdown it (arch-29.8
		// follow-up).
		_ = serv.Close()
		return nil, fmt.Errorf("failed to create ondemand: %w", err)
	}
	g.ondemand = a

	// When the WS bridge is enabled and a world connection handler is wired,
	// WebSocketHandler owns GET / and falls back to RootHandler for
	// non-Upgrade requests (preserving the existing static dispatch chain).
	if g.cfg.OnDemand.WebSocket.Enable && worldConn != nil {
		g.ondemand.Server.HTTP.HandleFunc("GET /", g.ondemand.WebSocketHandler)
	} else {
		g.ondemand.Server.HTTP.HandleFunc("GET /", g.ondemand.RootHandler)
	}

	// arch-29.6: /healthz + /debug/status. worldSrv is nil when the world
	// module is disabled (standalone ondemand) — snap then reports
	// hasWorld=false and /healthz degrades to a plain process-up 200. Two
	// mirrored HealthSnapshot structs (world's and ondemand's) avoid a
	// modules/ondemand → modules/world import; this adapter is the only
	// place that converts between them.
	ondemand.RegisterHealthRoutes(g.ondemand.Server.HTTP, func() (ondemand.HealthSnapshot, bool) {
		if worldSrv == nil {
			return ondemand.HealthSnapshot{}, false
		}
		s := worldSrv.HealthSnapshot()
		return ondemand.HealthSnapshot{
			LastTick:        s.LastTick,
			CurrentTick:     s.CurrentTick,
			PlayersOnline:   s.PlayersOnline,
			LastCycleMillis: s.LastCycleMillis,
		}, true
	}, g.cfg.OnDemand.DebugStatusEnabled)

	servicesToWaitFor := func() []services.Service {
		return []services.Service{}
	}

	//return g.ondemand, nil
	return server.NewServerService(serv, servicesToWaitFor), nil
	//return g.ondemand.Service, nil
}

func (g *App) initLogin() (services.Service, error) {
	if !g.cfg.Login.Enable {
		// TODO: still makes module appear to be running, move the check elsewhere?
		return services.NewIdleService(nil, nil), nil
	}

	logLevel := slog.Level(g.cfg.LogLevel)
	if g.cfg.Login.LogLevel != nil {
		logLevel = slog.Level(*g.cfg.Login.LogLevel)
	}
	logger, err := log.NewLogger(logLevel, g.cfg.LogFormat, os.Stdout, log.WithSourceFormat(g.cfg.LogSource))
	if err != nil {
		g.logger.Error("failed to create logger", "module", "login", "err", err)
		os.Exit(1)
	}
	logger = logger.With("component", "login")

	l, err := login.New(g.cfg.Login, g.cfg.Database, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create login: %w", err)
	}
	g.login = l

	return g.login, nil
}

func (g *App) initFriends() (services.Service, error) {
	if !g.cfg.Friends.Enable {
		// TODO: still makes module appear to be running, move the check elsewhere?
		return services.NewIdleService(nil, nil), nil
	}

	logLevel := slog.Level(g.cfg.LogLevel)
	if g.cfg.Friends.LogLevel != nil {
		logLevel = slog.Level(*g.cfg.Friends.LogLevel)
	}
	logger, err := log.NewLogger(logLevel, g.cfg.LogFormat, os.Stdout, log.WithSourceFormat(g.cfg.LogSource))
	if err != nil {
		g.logger.Error("failed to create logger", "module", "friends", "err", err)
		os.Exit(1)
	}
	logger = logger.With("component", "friends")

	f, err := friends.New(g.cfg.Friends, g.cfg.Database, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create friends: %w", err)
	}
	g.friends = f

	return g.friends, nil
}

func (g *App) initAccount() (services.Service, error) {
	if !g.cfg.Account.Enable {
		// TODO: still makes module appear to be running, move the check elsewhere?
		return services.NewIdleService(nil, nil), nil
	}

	logLevel := slog.Level(g.cfg.LogLevel)
	if g.cfg.Account.LogLevel != nil {
		logLevel = slog.Level(*g.cfg.Account.LogLevel)
	}
	logger, err := log.NewLogger(logLevel, g.cfg.LogFormat, os.Stdout, log.WithSourceFormat(g.cfg.LogSource))
	if err != nil {
		g.logger.Error("failed to create logger", "module", "account", "err", err)
		os.Exit(1)
	}
	logger = logger.With("component", "account")

	a, err := account.New(g.cfg.Account, g.cfg.Database, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}
	g.account = a

	return g.account, nil
}

func (g *App) initHiscore() (services.Service, error) {
	if !g.cfg.Hiscore.Enable {
		// TODO: still makes module appear to be running, move the check elsewhere?
		return services.NewIdleService(nil, nil), nil
	}

	logLevel := slog.Level(g.cfg.LogLevel)
	if g.cfg.Hiscore.Server.LogLevel != nil {
		logLevel = *g.cfg.Hiscore.Server.LogLevel
	}
	logger, err := log.NewLogger(logLevel, g.cfg.LogFormat, os.Stdout, log.WithSourceFormat(g.cfg.LogSource))
	if err != nil {
		g.logger.Error("failed to create logger", "module", "hiscore", "err", err)
		os.Exit(1)
	}
	logger = logger.With("component", "hiscore")

	g.cfg.Hiscore.Server.Log = logger

	server.DisableSignalHandling(&g.cfg.Hiscore.Server)
	serv, err := server.New(g.cfg.Hiscore.Server)
	if err != nil {
		return nil, err
	}

	h, err := hiscore.New(g.cfg.Hiscore, g.cfg.Database, logger, serv)
	if err != nil {
		// server.New already bound the HTTP listener; hiscore.New failing
		// here would otherwise leak the socket, since no service ever
		// runs to Shutdown it (same posture as initOnDemand).
		_ = serv.Close()
		return nil, fmt.Errorf("failed to create hiscore: %w", err)
	}
	g.hiscore = h

	return hiscore.NewHiscoreService(h, serv), nil
}

// initDatabase is the migration anchor: it brings the central-database
// schema up to date before any DB-using module starts (login, friends,
// account, and hiscore all depend on it in the graph). It holds no runtime
// connection — login, friends, account, and hiscore each open their own
// pool (independent-clients model, pkg/gamedb doc).
func (g *App) initDatabase() (services.Service, error) {
	if !g.cfg.Login.Enable && !g.cfg.Friends.Enable && !g.cfg.Account.Enable && !g.cfg.Hiscore.Enable {
		// No DB consumer in this target — contribute no service.
		g.logger.Info("module disabled", "module", "database")
		return nil, nil
	}

	logLevel := slog.Level(g.cfg.LogLevel)
	logger, err := log.NewLogger(logLevel, g.cfg.LogFormat, os.Stdout, log.WithSourceFormat(g.cfg.LogSource))
	if err != nil {
		g.logger.Error("failed to create logger", "module", "database", "err", err)
		os.Exit(1)
	}
	logger = logger.With("component", "database")

	return gamedb.NewMigratorService(g.cfg.Database, logger), nil
}

func (g *App) initWorld() (services.Service, error) {
	if !g.cfg.World.Enable {
		// TODO: still makes module appear to be running, move the check elsewhere?
		return services.NewIdleService(nil, nil), nil
	}

	logLevel := slog.Level(g.cfg.LogLevel)
	if g.cfg.World.LogLevel != nil {
		logLevel = slog.Level(*g.cfg.World.LogLevel)
	}

	logger, err := log.NewLogger(logLevel, g.cfg.LogFormat, os.Stdout, log.WithSourceFormat(g.cfg.LogSource))
	if err != nil {
		g.logger.Error("failed to create logger", "module", "world", "err", err)
		os.Exit(1)
	}

	world.DisableSignalHandling(&g.cfg.World)
	w, err := world.New(g.cfg.World, logger, tapper.NoopTapper())
	if err != nil {
		return nil, fmt.Errorf("failed to create world: %w", err)
	}
	g.world = w

	servicesToWaitFor := func() []services.Service {
		return []services.Service{}
	}

	return world.NewWorldService(g.world.Server, g.world.GetLoginClient(), g.world.GetFriendsClient(), servicesToWaitFor), nil
}

func (g *App) setupModuleManager(logger *slog.Logger) error {
	mm := modules.NewManager(logger)

	// Common is a module that exists only to map dependencies
	const Common = "common"

	mm.RegisterModule(Common, nil, modules.UserInvisibleModule)

	mm.RegisterModule(OnDemand, g.initOnDemand)
	mm.RegisterModule(Friends, g.initFriends)
	mm.RegisterModule(Login, g.initLogin)
	mm.RegisterModule(World, g.initWorld)
	mm.RegisterModule(Database, g.initDatabase, modules.UserInvisibleModule)
	mm.RegisterModule(Account, g.initAccount)
	mm.RegisterModule(Hiscore, g.initHiscore)

	mm.RegisterModule(SingleBinary, nil)

	deps := map[string][]string{
		Common: {},

		Database: {Common},
		OnDemand: {Common, World},
		Friends:  {Common, Database},
		Login:    {Common, Database},
		World:    {Common, Login, Friends},
		Account:  {Common, Database},
		Hiscore:  {Common, Database},

		SingleBinary: {OnDemand, Friends, Login, World, Account, Hiscore},
	}

	for mod, targets := range deps {
		if err := mm.AddDependency(mod, targets...); err != nil {
			return err
		}
	}

	g.ModuleManager = mm

	g.deps = deps

	return nil
}
