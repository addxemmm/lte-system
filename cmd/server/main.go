package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	ltesystem "github.com/addxemmm/lte-system"
	"github.com/addxemmm/lte-system/internal/api"
	"github.com/addxemmm/lte-system/internal/config"
	"github.com/addxemmm/lte-system/internal/lte"
	"github.com/addxemmm/lte-system/internal/webui"
)

// loadServerConfig requires an actual configuration file for anonymous mode,
// so absence of all discovered files does not silently enable anonymous mode.
func loadServerConfig(path string, candidates []string) (config.Config, error) {
	if path != "" {
		return config.Load(path)
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return config.Load(candidate)
		} else if !os.IsNotExist(err) {
			return config.Config{}, fmt.Errorf("inspect config %s: %w", candidate, err)
		}
	}
	cfg, err := config.Load("")
	if err == nil && cfg.APIToken == "" {
		return cfg, fmt.Errorf("no configuration file found: create a config with api_token: \"\" to explicitly allow anonymous access")
	}
	return cfg, err
}

func main() {
	cfg, err := loadServerConfig(os.Getenv("LTE_CONFIG"), []string{"/app/configs/app.yaml", "configs/app.yaml", "/data/app.yaml"})
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := cfg.EnsureDirs(); err != nil {
		log.Fatalf("ensure dirs: %v", err)
	}
	mgr := lte.New(cfg)
	if cfg.APIToken == "" {
		log.Printf("WARNING: API token is empty; anonymous API access enabled (trusted LAN only)")
	} else {
		log.Printf("API bearer authentication enabled")
	}
	apiHandler := api.New(cfg, mgr).Handler()
	uiPort, _ := config.ListenPort(cfg.UIListenAddr)
	apiPort, _ := config.ListenPort(cfg.ListenAddr)
	ui := webui.Handler(apiHandler, webui.PublicConfig{Version: ltesystem.Version(), APIExposed: cfg.ExposeAPI, UIPort: uiPort, APIPort: apiPort, AuthRequired: cfg.APIToken != ""}, cfg.UIAllowedHosts...)
	servers, err := bindServers(cfg, ui, apiHandler)
	if err != nil {
		log.Fatalf("bind listeners: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	serveErr := serveUntilStopped(ctx, servers)
	mgr.Stop()
	if serveErr != nil {
		log.Fatalf("HTTP server failed after cleanup: %v", serveErr)
	}
}

type boundServer struct {
	name     string
	server   *http.Server
	listener net.Listener
}

// Bind every requested socket before serving any request, so a conflicting test
// port cannot leave a half-started console. ExposeAPI=false never opens an API socket.
func bindServers(cfg config.Config, ui, apiHandler http.Handler) ([]boundServer, error) {
	definitions := []struct {
		name, address string
		handler       http.Handler
	}{{"console", cfg.UIListenAddr, ui}}
	if cfg.ExposeAPI {
		definitions = append(definitions, struct {
			name, address string
			handler       http.Handler
		}{"api", cfg.ListenAddr, apiHandler})
	}
	var result []boundServer
	for _, d := range definitions {
		listener, err := net.Listen("tcp", d.address)
		if err != nil {
			for _, prior := range result {
				_ = prior.listener.Close()
			}
			return nil, fmt.Errorf("%s listener: %w", d.name, err)
		}
		result = append(result, boundServer{name: d.name, listener: listener, server: &http.Server{
			Addr: d.address, Handler: d.handler,
			ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
			WriteTimeout: 220 * time.Second, IdleTimeout: 120 * time.Second,
		}})
	}
	return result, nil
}

func serveUntilStopped(ctx context.Context, servers []boundServer) error {
	failures := make(chan error, len(servers))
	for _, entry := range servers {
		entry := entry
		go func() {
			log.Printf("lte-system %s listening on %s", entry.name, entry.listener.Addr())
			err := entry.server.Serve(entry.listener)
			if err != nil && err != http.ErrServerClosed {
				failures <- fmt.Errorf("%s: %w", entry.name, err)
			}
		}()
	}
	var result error
	select {
	case <-ctx.Done():
	case result = <-failures:
	}
	drain, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for _, entry := range servers {
		wg.Add(1)
		go func(entry boundServer) {
			defer wg.Done()
			if err := entry.server.Shutdown(drain); err != nil {
				_ = entry.server.Close()
			}
			_ = entry.listener.Close()
		}(entry)
	}
	wg.Wait()
	return result
}
