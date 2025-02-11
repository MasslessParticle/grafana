package server

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	_ "github.com/grafana/grafana/pkg/extensions"
	"github.com/grafana/grafana/pkg/internal/api"
	"github.com/grafana/grafana/pkg/internal/api/routing"
	"github.com/grafana/grafana/pkg/internal/bus"
	"github.com/grafana/grafana/pkg/internal/infra/localcache"
	"github.com/grafana/grafana/pkg/internal/infra/log"
	"github.com/grafana/grafana/pkg/internal/infra/metrics"
	_ "github.com/grafana/grafana/pkg/internal/infra/remotecache"
	_ "github.com/grafana/grafana/pkg/internal/infra/serverlock"
	_ "github.com/grafana/grafana/pkg/internal/infra/tracing"
	_ "github.com/grafana/grafana/pkg/internal/infra/usagestats"
	"github.com/grafana/grafana/pkg/internal/login"
	"github.com/grafana/grafana/pkg/internal/login/social"
	"github.com/grafana/grafana/pkg/internal/middleware"
	_ "github.com/grafana/grafana/pkg/internal/plugins/manager"
	"github.com/grafana/grafana/pkg/internal/registry"
	_ "github.com/grafana/grafana/pkg/internal/services/alerting"
	_ "github.com/grafana/grafana/pkg/internal/services/auth"
	_ "github.com/grafana/grafana/pkg/internal/services/auth/jwt"
	_ "github.com/grafana/grafana/pkg/internal/services/cleanup"
	_ "github.com/grafana/grafana/pkg/internal/services/librarypanels"
	_ "github.com/grafana/grafana/pkg/internal/services/login/loginservice"
	_ "github.com/grafana/grafana/pkg/internal/services/ngalert"
	_ "github.com/grafana/grafana/pkg/internal/services/notifications"
	_ "github.com/grafana/grafana/pkg/internal/services/provisioning"
	_ "github.com/grafana/grafana/pkg/internal/services/rendering"
	_ "github.com/grafana/grafana/pkg/internal/services/search"
	_ "github.com/grafana/grafana/pkg/internal/services/sqlstore"
	"github.com/grafana/grafana/pkg/internal/setting"
)

// Config contains parameters for the New function.
type Config struct {
	ConfigFile  string
	HomePath    string
	PidFile     string
	Version     string
	Commit      string
	BuildBranch string
	Listener    net.Listener
}

type serviceRegistry interface {
	IsDisabled(srv registry.Service) bool
	GetServices() []*registry.Descriptor
}

type globalServiceRegistry struct{}

func (r *globalServiceRegistry) IsDisabled(srv registry.Service) bool {
	return registry.IsDisabled(srv)
}

func (r *globalServiceRegistry) GetServices() []*registry.Descriptor {
	return registry.GetServices()
}

// New returns a new instance of Server.
func New(cfg Config) (*Server, error) {
	s := newServer(cfg)
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

func newServer(cfg Config) *Server {
	rootCtx, shutdownFn := context.WithCancel(context.Background())
	childRoutines, childCtx := errgroup.WithContext(rootCtx)

	return &Server{
		context:          childCtx,
		shutdownFn:       shutdownFn,
		shutdownFinished: make(chan struct{}),
		childRoutines:    childRoutines,
		log:              log.New("server"),
		// Need to use the singleton setting.Cfg instance, to make sure we use the same as is injected in the DI
		// graph
		cfg: setting.GetCfg(),

		configFile:  cfg.ConfigFile,
		homePath:    cfg.HomePath,
		pidFile:     cfg.PidFile,
		version:     cfg.Version,
		commit:      cfg.Commit,
		buildBranch: cfg.BuildBranch,

		serviceRegistry: &globalServiceRegistry{},
		listener:        cfg.Listener,
	}
}

// Server is responsible for managing the lifecycle of services.
type Server struct {
	context          context.Context
	shutdownFn       context.CancelFunc
	childRoutines    *errgroup.Group
	log              log.Logger
	cfg              *setting.Cfg
	shutdownOnce     sync.Once
	shutdownFinished chan struct{}
	isInitialized    bool
	mtx              sync.Mutex
	listener         net.Listener

	configFile  string
	homePath    string
	pidFile     string
	version     string
	commit      string
	buildBranch string

	serviceRegistry serviceRegistry

	HTTPServer *api.HTTPServer `inject:""`
}

// init initializes the server and its services.
func (s *Server) init() error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.isInitialized {
		return nil
	}
	s.isInitialized = true

	s.loadConfiguration()
	s.writePIDFile()
	if err := metrics.SetEnvironmentInformation(s.cfg.MetricsGrafanaEnvironmentInfo); err != nil {
		return err
	}

	login.Init()
	social.NewOAuthService()

	services := s.serviceRegistry.GetServices()
	if err := s.buildServiceGraph(services); err != nil {
		return err
	}

	if s.listener != nil {
		for _, service := range services {
			if httpS, ok := service.Instance.(*api.HTTPServer); ok {
				// Configure the api.HTTPServer if necessary
				// Hopefully we can find a better solution, maybe with a more advanced DI framework, f.ex. Dig?
				s.log.Debug("Using provided listener for HTTP server")
				httpS.Listener = s.listener
			}
		}
	}

	return nil
}

// Run initializes and starts services. This will block until all services have
// exited. To initiate shutdown, call the Shutdown method in another goroutine.
func (s *Server) Run() error {
	defer close(s.shutdownFinished)

	if err := s.init(); err != nil {
		return err
	}

	services := s.serviceRegistry.GetServices()

	// Start background services.
	for _, svc := range services {
		service, ok := svc.Instance.(registry.BackgroundService)
		if !ok {
			continue
		}

		if s.serviceRegistry.IsDisabled(svc.Instance) {
			continue
		}

		// Variable is needed for accessing loop variable in callback
		descriptor := svc
		s.childRoutines.Go(func() error {
			select {
			case <-s.context.Done():
				return s.context.Err()
			default:
			}
			err := service.Run(s.context)
			// Do not return context.Canceled error since errgroup.Group only
			// returns the first error to the caller - thus we can miss a more
			// interesting error.
			if err != nil && !errors.Is(err, context.Canceled) {
				s.log.Error("Stopped "+descriptor.Name, "reason", err)
				return fmt.Errorf("%s run error: %w", descriptor.Name, err)
			}
			s.log.Debug("Stopped "+descriptor.Name, "reason", err)
			return nil
		})
	}

	s.notifySystemd("READY=1")

	s.log.Debug("Waiting on services...")
	return s.childRoutines.Wait()
}

// Shutdown initiates Grafana graceful shutdown. This shuts down all
// running background services. Since Run blocks Shutdown supposed to
// be run from a separate goroutine.
func (s *Server) Shutdown(ctx context.Context, reason string) error {
	var err error
	s.shutdownOnce.Do(func() {
		s.log.Info("Shutdown started", "reason", reason)
		// Call cancel func to stop services.
		s.shutdownFn()
		// Wait for server to shut down
		select {
		case <-s.shutdownFinished:
			s.log.Debug("Finished waiting for server to shut down")
		case <-ctx.Done():
			s.log.Warn("Timed out while waiting for server to shut down")
			err = fmt.Errorf("timeout waiting for shutdown")
		}
	})

	return err
}

// ExitCode returns an exit code for a given error.
func (s *Server) ExitCode(runError error) int {
	if runError != nil {
		s.log.Error("Server shutdown", "error", runError)
		return 1
	}
	return 0
}

// writePIDFile retrieves the current process ID and writes it to file.
func (s *Server) writePIDFile() {
	if s.pidFile == "" {
		return
	}

	// Ensure the required directory structure exists.
	err := os.MkdirAll(filepath.Dir(s.pidFile), 0700)
	if err != nil {
		s.log.Error("Failed to verify pid directory", "error", err)
		os.Exit(1)
	}

	// Retrieve the PID and write it to file.
	pid := strconv.Itoa(os.Getpid())
	if err := ioutil.WriteFile(s.pidFile, []byte(pid), 0644); err != nil {
		s.log.Error("Failed to write pidfile", "error", err)
		os.Exit(1)
	}

	s.log.Info("Writing PID file", "path", s.pidFile, "pid", pid)
}

// buildServiceGraph builds a graph of services and their dependencies.
func (s *Server) buildServiceGraph(services []*registry.Descriptor) error {
	// Specify service dependencies.
	objs := []interface{}{
		bus.GetBus(),
		s.cfg,
		routing.NewRouteRegister(middleware.ProvideRouteOperationName, middleware.RequestMetrics(s.cfg)),
		localcache.New(5*time.Minute, 10*time.Minute),
		s,
	}
	return registry.BuildServiceGraph(objs, services)
}

// loadConfiguration loads settings and configuration from config files.
func (s *Server) loadConfiguration() {
	args := &setting.CommandLineArgs{
		Config:   s.configFile,
		HomePath: s.homePath,
		Args:     flag.Args(),
	}

	if err := s.cfg.Load(args); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to start grafana. error: %s\n", err.Error())
		os.Exit(1)
	}

	s.log.Info("Starting "+setting.ApplicationName,
		"version", s.version,
		"commit", s.commit,
		"branch", s.buildBranch,
		"compiled", time.Unix(setting.BuildStamp, 0),
	)

	s.cfg.LogConfigSources()
}

// notifySystemd sends state notifications to systemd.
func (s *Server) notifySystemd(state string) {
	notifySocket := os.Getenv("NOTIFY_SOCKET")
	if notifySocket == "" {
		s.log.Debug(
			"NOTIFY_SOCKET environment variable empty or unset, can't send systemd notification")
		return
	}

	socketAddr := &net.UnixAddr{
		Name: notifySocket,
		Net:  "unixgram",
	}
	conn, err := net.DialUnix(socketAddr.Net, nil, socketAddr)
	if err != nil {
		s.log.Warn("Failed to connect to systemd", "err", err, "socket", notifySocket)
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			s.log.Warn("Failed to close connection", "err", err)
		}
	}()

	_, err = conn.Write([]byte(state))
	if err != nil {
		s.log.Warn("Failed to write notification to systemd", "err", err)
	}
}
