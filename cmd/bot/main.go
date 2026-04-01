package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	"github.com/xenbyte/Asterisk/internal/admin"
	"github.com/xenbyte/Asterisk/internal/claude"
	"github.com/xenbyte/Asterisk/internal/storage"
	"github.com/xenbyte/Asterisk/internal/telegram"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	_ = godotenv.Load()

	cfg, err := loadConfig()
	if err != nil {
		logger.Error("configuration error", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	db, err := storage.New(ctx, cfg.databaseURL)
	if err != nil {
		logger.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	logger.Info("database connected")

	bot, err := tgbotapi.NewBotAPI(cfg.telegramToken)
	if err != nil {
		logger.Error("failed to create telegram bot", "error", err)
		os.Exit(1)
	}
	logger.Info("authorized on telegram", "username", bot.Self.UserName)

	claudeClient := claude.NewClient(cfg.anthropicKey)

	handler := telegram.NewHandler(bot, claudeClient, db, logger)

	adminSrv := admin.New(db, cfg.adminToken, cfg.adminPort)
	go func() {
		logger.Info("starting admin API", "port", cfg.adminPort)
		if err := adminSrv.Start(); err != nil {
			logger.Error("admin API error", "error", err)
		}
	}()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	logger.Info("bot started, listening for updates")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sig
		logger.Info("shutting down gracefully")
		bot.StopReceivingUpdates()
	}()

	for update := range updates {
		handler.HandleUpdate(update)
	}

	logger.Info("bot stopped")
}

type config struct {
	telegramToken string
	anthropicKey  string
	databaseURL   string
	adminToken    string
	adminPort     string
}

func loadConfig() (*config, error) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is required")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		return nil, fmt.Errorf("ADMIN_TOKEN is required")
	}

	adminPort := os.Getenv("ADMIN_PORT")
	if adminPort == "" {
		adminPort = "8080"
	}

	return &config{
		telegramToken: token,
		anthropicKey:  apiKey,
		databaseURL:   databaseURL,
		adminToken:    adminToken,
		adminPort:     adminPort,
	}, nil
}
