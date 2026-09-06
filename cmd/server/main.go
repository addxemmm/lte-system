package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/addxemmm/lte-system/internal/api"
	"github.com/addxemmm/lte-system/internal/config"
	"github.com/addxemmm/lte-system/internal/lte"
)

func main() {
	cfgPath := os.Getenv("LTE_CONFIG")
	if cfgPath == "" {
		for _, cand := range []string{"/app/configs/app.yaml", "configs/app.yaml", "/data/app.yaml"} {
			if _, err := os.Stat(cand); err == nil {
				cfgPath = cand
				break
			}
		}
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := cfg.EnsureDirs(); err != nil {
		log.Fatalf("ensure dirs: %v", err)
	}
	mgr := lte.New(cfg)
	if strings.TrimSpace(os.Getenv("LTE_API_TOKEN")) == "" {
		log.Printf("WARNING: LTE_API_TOKEN unset, API is open (LAN-only deployment required)")
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
