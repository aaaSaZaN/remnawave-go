package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	AppPort         string
	MetricsPort     string
	AppSecret       string
	DBDriver        string
	DatabaseURL     string
	FrontEndDomain  string
	PanelDomain     string
	SubPublicDomain string
	JWTLifetime     int
}

func Load() *Config {
	_ = godotenv.Load(".env", "../.env")

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "3000"
	}

	metricsPort := os.Getenv("METRICS_PORT")
	if metricsPort == "" {
		metricsPort = "3001"
	}

	secret := os.Getenv("APP_SECRET")
	if secret == "" || secret == "change_me" {
		secret = "remnawave-default-secret-key-32chars-minimum!!"
	}

	dbDriver := strings.ToLower(os.Getenv("DB_DRIVER"))
	dbURL := os.Getenv("DATABASE_URL")

	if dbDriver == "" {
		if strings.HasPrefix(dbURL, "postgres://") || strings.HasPrefix(dbURL, "postgresql://") {
			dbDriver = "postgres"
		} else {
			dbDriver = "sqlite"
		}
	}

	if dbURL == "" {
		if dbDriver == "postgres" {
			dbURL = "postgresql://postgres:postgres@localhost:5432/remnawave"
		} else {
			dbURL = "remnawave.db"
		}
	}

	feDomain := os.Getenv("FRONT_END_DOMAIN")
	if feDomain == "" {
		feDomain = "*"
	}

	panelDomain := os.Getenv("PANEL_DOMAIN")
	if panelDomain == "" {
		panelDomain = "localhost:3000"
	}

	subDomain := os.Getenv("SUB_PUBLIC_DOMAIN")
	if subDomain == "" {
		subDomain = "localhost:3000/api/sub"
	}

	jwtLifetime := 12
	if lifetimeStr := os.Getenv("JWT_AUTH_LIFETIME"); lifetimeStr != "" {
		if val, err := strconv.Atoi(lifetimeStr); err == nil && val > 0 {
			jwtLifetime = val
		}
	}

	return &Config{
		AppPort:         port,
		MetricsPort:     metricsPort,
		AppSecret:       secret,
		DBDriver:        dbDriver,
		DatabaseURL:     dbURL,
		FrontEndDomain:  feDomain,
		PanelDomain:     panelDomain,
		SubPublicDomain: subDomain,
		JWTLifetime:     jwtLifetime,
	}
}
