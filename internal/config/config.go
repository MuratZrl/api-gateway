package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server         ServerConfig         `yaml:"server"`
	MongoDB        MongoDBConfig        `yaml:"mongodb"`
	Redis          RedisConfig          `yaml:"redis"`
	JWT            JWTConfig            `yaml:"jwt"`
	RateLimit      RateLimitConfig      `yaml:"rate_limit"`
	CircuitBreaker CircuitBreakerConfig `yaml:"circuit_breaker"`
	Cache          CacheConfig          `yaml:"cache"`
	IPFilter       IPFilterConfig       `yaml:"ip_filter"`
	Retry          RetryConfig          `yaml:"retry"`
	Routes         []RouteConfig        `yaml:"routes"`
}

type ServerConfig struct {
	Port         int `yaml:"port"`
	ReadTimeout  int `yaml:"read_timeout"`
	WriteTimeout int `yaml:"write_timeout"`
}

type MongoDBConfig struct {
	URI      string `yaml:"uri"`
	Database string `yaml:"database"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type JWTConfig struct {
	Secret     string `yaml:"secret"`
	Expiration int    `yaml:"expiration"`
}

type RateLimitConfig struct {
	RequestsPerMinute int `yaml:"requests_per_minute"`
	Burst             int `yaml:"burst"`
}

type CircuitBreakerConfig struct {
	MaxFailures        int `yaml:"max_failures"`
	Timeout            int `yaml:"timeout"`
	HalfOpenMaxRequests int `yaml:"half_open_max_requests"`
}

type CacheConfig struct {
	Enabled    bool `yaml:"enabled"`
	TTLSeconds int  `yaml:"ttl_seconds"`
}

type IPFilterConfig struct {
	Mode      string   `yaml:"mode"` // "whitelist", "blacklist", "disabled"
	Whitelist []string `yaml:"whitelist"`
	Blacklist []string `yaml:"blacklist"`
}

type RetryConfig struct {
	MaxRetries      int `yaml:"max_retries"`
	InitialWaitMs   int `yaml:"initial_wait_ms"`
	MaxWaitMs       int `yaml:"max_wait_ms"`
	Multiplier      float64 `yaml:"multiplier"`
}

type RouteConfig struct {
	Path      string   `yaml:"path"`
	Target    string   `yaml:"target"`
	Targets   []string `yaml:"targets"` // Multiple targets for load balancing
	Methods   []string `yaml:"methods"`
	Protected bool     `yaml:"protected"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
