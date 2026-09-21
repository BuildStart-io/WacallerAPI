package config

import (
	"bufio"
	"flag"
	"os"
	"strconv"
	"strings"
)

// Config represents all server configuration parameters.
type Config struct {
	Addr               string
	DBPath             string
	MasterAPIKey       string
	GlobalWebhookURL   string
	WebhookSecret      string
	MaxCallsPerSession int
	Debug              bool
	StripeSecretKey    string
	StripePublishableKey string
	StripeWebhookSecret string
}

func loadDotEnv() {
	f, err := os.Open(".env")
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			v = strings.Trim(v, `"'`)
			if os.Getenv(k) == "" {
				_ = os.Setenv(k, v)
			}
		}
	}
}

// Load loads configuration from command line flags and environment variables.
func Load() *Config {
	loadDotEnv()

	cfg := &Config{
		Addr:                 ":8080",
		DBPath:               "wacaller.db",
		MasterAPIKey:         "",
		GlobalWebhookURL:     "",
		WebhookSecret:        "",
		MaxCallsPerSession:   8,
		Debug:                false,
		StripeSecretKey:      "",
		StripePublishableKey: "",
		StripeWebhookSecret:  "",
	}

	if env := os.Getenv("WACALLER_ADDR"); env != "" {
		cfg.Addr = env
	}
	if env := os.Getenv("WACALLER_DB"); env != "" {
		cfg.DBPath = env
	}
	if env := os.Getenv("WACALLER_API_KEY"); env != "" {
		cfg.MasterAPIKey = env
	}
	if env := os.Getenv("WACALLER_WEBHOOK_URL"); env != "" {
		cfg.GlobalWebhookURL = env
	}
	if env := os.Getenv("WACALLER_WEBHOOK_SECRET"); env != "" {
		cfg.WebhookSecret = env
	}
	if env := os.Getenv("STRIPE_SECRET_KEY"); env != "" {
		cfg.StripeSecretKey = env
	}
	if env := os.Getenv("STRIPE_PUBLISHABLE_KEY"); env != "" {
		cfg.StripePublishableKey = env
	}
	if env := os.Getenv("STRIPE_WEBHOOK_SECRET"); env != "" {
		cfg.StripeWebhookSecret = env
	}
	if env := os.Getenv("WACALLER_MAX_CALLS"); env != "" {
		if val, err := strconv.Atoi(env); err == nil {
			cfg.MaxCallsPerSession = val
		}
	}
	if env := os.Getenv("WACALLER_DEBUG"); env == "true" || env == "1" {
		cfg.Debug = true
	}

	flag.StringVar(&cfg.Addr, "addr", cfg.Addr, "HTTP/WebSocket listen address")
	flag.StringVar(&cfg.DBPath, "db", cfg.DBPath, "SQLite database path")
	flag.StringVar(&cfg.MasterAPIKey, "api-key", cfg.MasterAPIKey, "Master API Key (optional, enables API protection if set)")
	flag.StringVar(&cfg.GlobalWebhookURL, "webhook-url", cfg.GlobalWebhookURL, "Global fallback webhook URL")
	flag.StringVar(&cfg.WebhookSecret, "webhook-secret", cfg.WebhookSecret, "Secret for signing webhook HMAC payloads")
	flag.StringVar(&cfg.StripeSecretKey, "stripe-secret", cfg.StripeSecretKey, "Stripe Secret API Key (sk_live_... or sk_test_...)")
	flag.StringVar(&cfg.StripePublishableKey, "stripe-key", cfg.StripePublishableKey, "Stripe Publishable Key (pk_live_... or pk_test_...)")
	flag.StringVar(&cfg.StripeWebhookSecret, "stripe-webhook-secret", cfg.StripeWebhookSecret, "Stripe Webhook Signing Secret (whsec_...)")
	flag.IntVar(&cfg.MaxCallsPerSession, "max-calls-per-session", cfg.MaxCallsPerSession, "Max concurrent calls per session (0 = unlimited)")
	flag.BoolVar(&cfg.Debug, "debug", cfg.Debug, "Enable debug logs")

	if !flag.Parsed() {
		flag.Parse()
	}

	return cfg
}
