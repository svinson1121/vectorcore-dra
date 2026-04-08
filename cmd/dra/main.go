package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/svinson1121/vectorcore-dra/internal/api"
	"github.com/svinson1121/vectorcore-dra/internal/config"
	"github.com/svinson1121/vectorcore-dra/internal/logging"
	"github.com/svinson1121/vectorcore-dra/internal/metrics"
	"github.com/svinson1121/vectorcore-dra/internal/peermgr"
	"github.com/svinson1121/vectorcore-dra/internal/router"
	"github.com/svinson1121/vectorcore-dra/internal/transport"
)

const (
	appVersion = "0.2.0B"
	apiVersion = "0.2.0B"
)

func main() {
	debugMode := flag.Bool("d", false, "debug mode: log at DEBUG level to file+console, overrides config")
	cfgFlag   := flag.String("c", "", "path to config.yaml")
	showVer   := flag.Bool("v", false, "print version and exit")
	flag.Parse()

	if *showVer {
		fmt.Printf("VectorCore DRA  v%s (API v%s)\n", appVersion, apiVersion)
		os.Exit(0)
	}

	cfgPath := *cfgFlag
	if cfgPath == "" {
		cfgPath = os.Getenv("DRA_CONFIG")
	}
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}

	// Load config first so we can configure logging
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load config file %q (%v), using defaults\n", cfgPath, err)
		cfg = config.Default()
	}

	// Set up logger
	log, err := logging.New(logging.Config{
		Level: cfg.Logging.Level,
		File:  cfg.Logging.File,
	}, *debugMode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	log.Info("VectorCore DRA starting",
		zap.String("identity", cfg.DRA.Identity),
		zap.String("realm", cfg.DRA.Realm),
		zap.String("config_file", cfgPath),
		zap.Bool("debug_mode", *debugMode),
	)

	localIP := getLocalIP()
	log.Info("local IP determined", zap.String("local_ip", localIP.String()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Build router and peer manager
	r := router.New(cfg.DRA.Identity, log)
	mgr := peermgr.New(r, cfg.DRA.Identity, log)

	// Load initial route rules and IMSI routes
	r.UpdateRules(configRulesToRouterRules(cfg.RouteRules))
	r.UpdateIMSIRoutes(configIMSIToRouterIMSI(cfg.IMSIRoutes))

	// Initial peer sync
	mgr.Sync(ctx, cfg.Peers, cfg.DRA, cfg.Watchdog, cfg.Reconnect, localIP)

	// Start manager background goroutines (pending-entry sweep, etc.)
	mgr.Start(ctx)

	// Start HTTP API server
	apiSrv := api.New(ctx, cfg, cfgPath, mgr, r, log)
	mgr.SetRecorder(apiSrv.RecentMsgs)
	httpAddr := fmt.Sprintf("%s:%d", cfg.API.Address, cfg.API.Port)
	go func() {
		if err := apiSrv.Start(ctx, httpAddr); err != nil {
			log.Error("API server error", zap.Error(err))
		}
	}()

	// Config file watcher for hot reload
	watcher, watchErr := config.NewWatcher(cfgPath, func(newCfg *config.Config) {
		log.Info("applying hot-reloaded config")
		r.UpdateRules(configRulesToRouterRules(newCfg.RouteRules))
		r.UpdateIMSIRoutes(configIMSIToRouterIMSI(newCfg.IMSIRoutes))
		mgr.Sync(ctx, newCfg.Peers, newCfg.DRA, newCfg.Watchdog, newCfg.Reconnect, localIP)
	}, log)
	if watchErr != nil {
		log.Warn("config file watcher not started", zap.Error(watchErr))
	} else {
		watcher.Start(ctx)
		defer watcher.Stop()
	}

	// Start ALL configured listeners concurrently
	listeners := startListeners(ctx, cfg, mgr, log)
	if len(listeners) == 0 {
		// Fallback: default TCP listener
		t := transport.NewTCP()
		ln, err := t.Listen("0.0.0.0:3868")
		if err != nil {
			log.Fatal("failed to start default listener", zap.Error(err))
		}
		listeners = append(listeners, ln)
		log.Info("listening on default addr", zap.String("addr", "0.0.0.0:3868"), zap.String("transport", "tcp"))
		go acceptLoop(ctx, ln, "tcp", cfg, localIP, mgr, log)
	}

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	select {
	case sig := <-sigCh:
		log.Info("received shutdown signal", zap.String("signal", sig.String()))
	case <-ctx.Done():
	}

	log.Info("initiating graceful shutdown")
	cancel()

	// Give peers time to send DPR/DPA
	time.Sleep(2 * time.Second)

	for _, ln := range listeners {
		ln.Close()
	}

	log.Info("VectorCore DRA stopped")
}

// startListeners starts a listener goroutine for each entry in cfg.Listeners.
// All listeners start concurrently. Returns the list of active net.Listener handles.
func startListeners(ctx context.Context, cfg *config.Config, mgr *peermgr.Manager, log *zap.Logger) []net.Listener {
	var (
		mu        sync.Mutex
		listeners []net.Listener
	)
	var wg sync.WaitGroup

	for _, lcfg := range cfg.Listeners {
		lcfg := lcfg // capture
		wg.Add(1)
		go func() {
			defer wg.Done()
			addr := fmt.Sprintf("%s:%d", lcfg.Address, lcfg.Port)

			var t transport.Transport
			switch lcfg.Transport {
			case "tcp":
				t = transport.NewTCP()
			case "tcp+tls":
				t = transport.NewTCPTLS(transport.TLSConfig{
					CertFile: cfg.TLS.CertFile,
					KeyFile:  cfg.TLS.KeyFile,
					CAFile:   cfg.TLS.CAFile,
				})
			case "sctp":
				t = transport.NewSCTP()
			case "sctp+tls":
				// SCTP+TLS deferred - fall back to plain SCTP
				log.Warn("sctp+tls not yet supported, using plain sctp", zap.String("addr", addr))
				t = transport.NewSCTP()
			default:
				log.Error("unknown transport in listener config, skipping",
					zap.String("transport", lcfg.Transport),
					zap.String("addr", addr),
				)
				return
			}

			ln, err := t.Listen(addr)
			if err != nil {
				log.Fatal("failed to start listener",
					zap.String("addr", addr),
					zap.String("transport", lcfg.Transport),
					zap.Error(err),
				)
				return
			}

			log.Info("listening for Diameter connections",
				zap.String("addr", addr),
				zap.String("transport", lcfg.Transport),
			)

			mu.Lock()
			listeners = append(listeners, ln)
			mu.Unlock()

			go acceptLoop(ctx, ln, lcfg.Transport, cfg, getLocalIP(), mgr, log)
		}()
	}
	wg.Wait()
	return listeners
}

// acceptLoop accepts inbound Diameter connections on ln.
// transportName identifies the protocol of this listener (e.g. "tcp", "sctp").
// Unknown source IPs are dropped before any Diameter processing.
func acceptLoop(ctx context.Context, ln net.Listener, transportName string, cfg *config.Config, localIP net.IP, mgr *peermgr.Manager, log *zap.Logger) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				log.Warn("accept error", zap.Error(err))
				continue
			}
		}

		remoteAddr := conn.RemoteAddr().String()

		// Security: drop connections from unknown sources
		if !mgr.IsKnownAddress(conn.RemoteAddr()) {
			host, _, _ := net.SplitHostPort(remoteAddr)
			log.Warn("rejected inbound connection",
				zap.String("remote", remoteAddr),
				zap.String("reason", mgr.RejectReason(conn.RemoteAddr())),
			)
			metrics.RejectedConnectionsTotal.WithLabelValues(host).Inc()
			conn.Close()
			continue
		}

		log.Info("accepted inbound connection", zap.String("remote", remoteAddr))

		// All connections that passed IsKnownAddress belong to a configured peer.
		// Route them through AcceptInbound regardless of the peer's configured mode
		// (passive peers wait for inbound; active peers that dial us first are handled
		// via the RFC 6733 sec 5.6.4 fallback in AcceptInbound).
		if mgr.HasConfiguredPeer(conn.RemoteAddr()) {
			mgr.AcceptInbound(ctx, conn, transportName, cfg.DRA, cfg.Watchdog, localIP)
			continue
		}

		// Fallback: known IP but not in byAddr (race window after Sync). Drop.
		log.Warn("no configured peer for inbound connection, dropping",
			zap.String("remote", remoteAddr),
		)
		conn.Close()
	}
}

// configRulesToRouterRules converts config.RouteRule slice to router.Rule slice.
func configRulesToRouterRules(cfgRules []config.RouteRule) []router.Rule {
	rules := make([]router.Rule, 0, len(cfgRules))
	for _, r := range cfgRules {
		rules = append(rules, router.Rule{
			Priority:  r.Priority,
			DestHost:  r.DestHost,
			DestRealm: r.DestRealm,
			AppID:     r.AppID,
			PeerGroup: r.PeerGroup,
			Peer:      r.Peer,
			Action:    r.Action,
			Enabled:   r.Enabled,
		})
	}
	return rules
}

// configIMSIToRouterIMSI converts config.IMSIRoute slice to router.IMSIRoute slice.
func configIMSIToRouterIMSI(cfgRoutes []config.IMSIRoute) []router.IMSIRoute {
	routes := make([]router.IMSIRoute, 0, len(cfgRoutes))
	for _, r := range cfgRoutes {
		routes = append(routes, router.IMSIRoute{
			Prefix:    r.Prefix,
			DestRealm: r.DestRealm,
			PeerGroup: r.PeerGroup,
			Priority:  r.Priority,
			Enabled:   r.Enabled,
		})
	}
	return routes
}

// getLocalIP returns the primary non-loopback IPv4 address of this host.
func getLocalIP() net.IP {
	ifaces, err := net.Interfaces()
	if err != nil {
		return net.ParseIP("127.0.0.1")
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if ip4 := ip.To4(); ip4 != nil {
				return ip4
			}
		}
	}
	return net.ParseIP("127.0.0.1")
}
