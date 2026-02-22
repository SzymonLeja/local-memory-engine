package config

import (
	"log"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	ListenAddr       string
	PostgresDSN      string
	QdrantURL        string
	QdrantCollection string
	OllamaURL        string
	EmbeddingModel   string
	AllowedOrigins   []string
	ApiKey           string
	VaultRoot        string
	WatchPath        string
}

func Load() *Config {
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")
	if err := viper.ReadInConfig(); err != nil {
		log.Println(".env is missing, switching to automatic environment variables loading")
	}

	viper.AutomaticEnv()

	viper.SetDefault("LISTEN_ADDR", ":8080")
	viper.SetDefault("EMBEDDING_MODEL", "nomic-embed-text")
	viper.SetDefault("ALLOWED_ORIGINS", "http://localhost")
	viper.SetDefault("VAULT_ROOT", "./vault")
	viper.SetDefault("QDRANT_COLLECTION", "lme")
	viper.SetDefault("WATCH_PATH", ".")

	cfg := &Config{
		ListenAddr:       viper.GetString("LISTEN_ADDR"),
		PostgresDSN:      viper.GetString("POSTGRES_DSN"),
		QdrantURL:        viper.GetString("QDRANT_URL"),
		QdrantCollection: viper.GetString("QDRANT_COLLECTION"),
		OllamaURL:        viper.GetString("OLLAMA_URL"),
		EmbeddingModel:   viper.GetString("EMBEDDING_MODEL"),
		AllowedOrigins:   strings.Split(viper.GetString("ALLOWED_ORIGINS"), ","),
		ApiKey:           viper.GetString("API_KEY"),
		VaultRoot:        viper.GetString("VAULT_ROOT"),
		WatchPath:        viper.GetString("WATCH_PATH"),
	}

	if cfg.PostgresDSN == "" {
		log.Fatal("POSTGRES_DSN is required")
	}
	if cfg.QdrantURL == "" {
		log.Fatal("QDRANT_URL is required")
	}
	if cfg.OllamaURL == "" {
		log.Fatal("OLLAMA_URL is required")
	}

	return cfg
}
