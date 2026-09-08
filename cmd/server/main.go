package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/addxemmm/lte-system/internal/api"
	"github.com/addxemmm/lte-system/internal/config"
	"github.com/addxemmm/lte-system/internal/lte"
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
	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.New(cfg, mgr).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// Must exceed the longest handler context (/writesim 180s + margin).
		WriteTimeout: 220 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	go func() {
		log.Printf("lte-system listening on %s (data=%s)", cfg.ListenAddr, cfg.DataDir)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	// A SIM operation may take 180s; allow its database update to finish.
	// Compose stop_grace_period must exceed this drain deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	mgr.Stop()
}
