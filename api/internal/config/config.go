// Package config reads settings from environment variables.
package config

import (
	"os"
	"strings"
)

type Config struct {
	DatabaseURL    string   // DATABASE_URL
	HTTPAddr       string   // HTTP_ADDR, default ":8080"
	CookieSecure   bool     // COOKIE_SECURE, default true; set "false" for local http
	AllowedOrigins []string // ALLOWED_ORIGINS, comma separated, e.g. "https://pos.example.vn,http://localhost:3000"
	PublicBaseURL  string   // PUBLIC_BASE_URL, used to build invoice lookup links, e.g. "https://pos.example.vn"
}

func Load() Config {
	c := Config{
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		HTTPAddr:      envOr("HTTP_ADDR", ":8080"),
		CookieSecure:  os.Getenv("COOKIE_SECURE") != "false",
		PublicBaseURL: strings.TrimRight(envOr("PUBLIC_BASE_URL", "http://localhost:3000"), "/"),
	}
	for _, o := range strings.Split(envOr("ALLOWED_ORIGINS", "http://localhost:3000"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			c.AllowedOrigins = append(c.AllowedOrigins, o)
		}
	}
	return c
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
