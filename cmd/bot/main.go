package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
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

	db, err := storage.Open(cfg.dbPath)
	if err != nil {
		logger.Error("failed to open database", "error", err, "path", cfg.dbPath)
		os.Exit(1)
	}
	defer db.Close()
	logger.Info("database opened", "path", cfg.dbPath)

	bot, err := tgbotapi.NewBotAPI(cfg.telegramToken)
	if err != nil {
		logger.Error("failed to create telegram bot", "error", err)
		os.Exit(1)
	}
	logger.Info("authorized on telegram", "username", bot.Self.UserName)

	claudeClient := claude.NewClient(cfg.anthropicKey)

	handler := telegram.NewHandler(bot, claudeClient, db, cfg.allowedUserID, logger)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	logger.Info("bot started, listening for updates", "allowed_user_id", cfg.allowedUserID)

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
	allowedUserID int64
	dbPath        string
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

	userIDStr := os.Getenv("TELEGRAM_ALLOWED_USER_ID")
	if userIDStr == "" {
		return nil, fmt.Errorf("TELEGRAM_ALLOWED_USER_ID is required")
	}
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("TELEGRAM_ALLOWED_USER_ID must be an integer: %w", err)
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("creating data directory: %w", err)
	}

	return &config{
		telegramToken: token,
		anthropicKey:  apiKey,
		allowedUserID: userID,
		dbPath:        filepath.Join(dataDir, "asterisk.db"),
	}, nil
}
