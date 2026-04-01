package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
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

	handler := telegram.NewHandler(bot, claudeClient, db, logger, cfg.adminTelegramID)

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
	telegramToken   string
	anthropicKey    string
	databaseURL     string
	adminTelegramID int64
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

	adminIDStr := os.Getenv("ADMIN_TELEGRAM_ID")
	if adminIDStr == "" {
		return nil, fmt.Errorf("ADMIN_TELEGRAM_ID is required")
	}
	adminID, err := strconv.ParseInt(adminIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("ADMIN_TELEGRAM_ID must be a valid integer: %w", err)
	}

	return &config{
		telegramToken:   token,
		anthropicKey:    apiKey,
		databaseURL:     databaseURL,
		adminTelegramID: adminID,
	}, nil
}
