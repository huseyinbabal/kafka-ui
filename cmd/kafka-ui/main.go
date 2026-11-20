// Command kafka-ui serves the Kafka UI web application.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/huseyinbabal/kafka-ui/internal/analytics"
	"github.com/huseyinbabal/kafka-ui/internal/api"
	"github.com/huseyinbabal/kafka-ui/internal/auth"
	"github.com/huseyinbabal/kafka-ui/internal/cluster"
	"github.com/huseyinbabal/kafka-ui/internal/config"
	"github.com/huseyinbabal/kafka-ui/internal/live"
	"github.com/huseyinbabal/kafka-ui/internal/storage"
	"github.com/huseyinbabal/kafka-ui/web"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "hash-password" {
		if err := hashPassword(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func hashPassword() error {
	fmt.Fprint(os.Stderr, "password: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return err
	}
	h, err := auth.HashPassword(strings.TrimRight(line, "\r\n"))
	if err != nil {
		return err
	}
	fmt.Println(h)
	return nil
}

func run() error {
	cfgPath := flag.String("config", envOr("KAFKAUI_CONFIG", "config.yaml"), "path to config file")
	logFormat := flag.String("log-format", envOr("KAFKAUI_LOG_FORMAT", "text"), "text or json")
	flag.Parse()

	var handler slog.Handler = slog.NewTextHandler(os.Stdout, nil)
	if *logFormat == "json" {
		handler = slog.NewJSONHandler(os.Stdout, nil)
	}
	log := slog.New(handler)
	slog.SetDefault(log)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := storage.Open(ctx, cfg.Storage.Path)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	authSvc := auth.NewService(db, cfg.Auth)
	reg, err := cluster.NewRegistry(cfg.Clusters, log)
	if err != nil {
		return err
	}
	defer reg.Close()

	go reg.RunHealthChecks(ctx, 15*time.Second)
	go every(ctx, time.Hour, func() {
		if err := authSvc.PurgeExpired(ctx); err != nil {
			log.Warn("purge sessions", "err", err)
		}
	})

	store := analytics.NewStore(db, cfg.Storage.Retention)
	collector := analytics.NewCollector(reg, store, cfg.Collector.Interval, log)
	go collector.Run(ctx)

	srv := &http.Server{
		Addr: cfg.Server.Addr,
		Handler: api.NewServer(cfg, authSvc, reg, web.Dist(), log).WithAnalytics(store, collector).
			WithLive(live.NewHub(ctx, cfg.Collector.LiveInterval, collector.PartitionRates, log)).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Server.Addr, "clusters", len(cfg.Clusters))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shCtx)
}

func every(ctx context.Context, d time.Duration, fn func()) {
	t := time.NewTicker(d)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn()
		}
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
