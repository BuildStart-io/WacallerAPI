package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"wacallerapi/internal/api"
	"wacallerapi/internal/config"
	"wacallerapi/internal/session"
	"wacallerapi/internal/store"
	"wacallerapi/internal/web"
	"wacallerapi/internal/webhook"

	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	waStore "go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

func init() {
	waStore.DeviceProps.PlatformType = waCompanionReg.DeviceProps_CHROME.Enum()
	waStore.DeviceProps.Os = proto.String("Mac OS")
	waStore.DeviceProps.RequireFullSync = proto.Bool(false)
	if waStore.DeviceProps.HistorySyncConfig != nil {
		waStore.DeviceProps.HistorySyncConfig.SupportCallLogHistory = proto.Bool(true)
	}
	waStore.SetOSInfo("Mac OS", [3]uint32{14, 5, 0})
}

func main() {
	cfg := config.Load()

	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("initializing WacallerAPI storage", "db", cfg.DBPath)
	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		log.Error("failed to open database", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	waLogger := waLog.Noop
	if cfg.Debug {
		waLogger = waLog.Stdout("WA", "INFO", true)
	}

	container := sqlstore.NewWithDB(st.DB(), "sqlite3", waLogger)
	if err := container.Upgrade(ctx); err != nil {
		log.Error("failed to upgrade whatsmeow database", "err", err)
		os.Exit(1)
	}

	dispatcher := webhook.NewDispatcher(st, cfg.GlobalWebhookURL, cfg.WebhookSecret, log)
	sessionMgr := session.NewSessionManager(ctx, container, st, dispatcher, log, cfg.MaxCallsPerSession)

	log.Info("restoring existing sessions...")
	if err := sessionMgr.Restore(ctx); err != nil {
		log.Warn("warning: error while restoring sessions", "err", err)
	}

	apiServer := api.New(cfg, st, sessionMgr, dispatcher, log)

	mainMux := http.NewServeMux()
	mainMux.Handle("/api/", apiServer.Routes())
	mainMux.Handle("/", web.Handler())

	server := &http.Server{
		Addr:         cfg.Addr,
		Handler:      mainMux,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		fmt.Printf("\n🚀 \x1b[1;32mWacallerAPI Server running at http://localhost%s\x1b[0m\n", cfg.Addr)
		fmt.Printf("   ├─ Developer Portal & Dashboard : \x1b[36mhttp://localhost%s\x1b[0m\n", cfg.Addr)
		fmt.Printf("   ├─ OpenAPI 3.0 Specification    : \x1b[36mhttp://localhost%s/api/openapi.json\x1b[0m\n", cfg.Addr)
		fmt.Printf("   ├─ REST API Gateway             : \x1b[36mhttp://localhost%s/api/v1/\x1b[0m\n", cfg.Addr)
		fmt.Printf("   └─ WebSocket Audio Stream       : \x1b[36mws://localhost%s/api/v1/sessions/{sid}/calls/{id}/stream\x1b[0m\n\n", cfg.Addr)

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down WacallerAPI...")
	sessionMgr.DisconnectAll()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	log.Info("WacallerAPI stopped cleanly")
}
