package config

import (
	"os"
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	content := `
server:
  port: 8080
  read_timeout: 15
  write_timeout: 15
mongodb:
  uri: "mongodb://localhost:27017"
  database: "test"
redis:
  addr: "localhost:6379"
  password: ""
  db: 0
jwt:
  secret: "test-secret"
  expiration: 3600
rate_limit:
  requests_per_minute: 60
  burst: 10
circuit_breaker:
  max_failures: 5
  timeout: 30
  half_open_max_requests: 3
cache:
  enabled: true
  ttl_seconds: 30
ip_filter:
  mode: "disabled"
  whitelist: []
  blacklist: []
retry:
  max_retries: 3
  initial_wait_ms: 100
  max_wait_ms: 2000
  multiplier: 2.0
routes:
  - path: "/api/users"
    target: "http://localhost:8081"
    methods: ["GET", "POST"]
    protected: false
`
	tmpFile, err := os.CreateTemp("", "gateway-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString(content)
	tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.MongoDB.Database != "test" {
		t.Errorf("expected database 'test', got %s", cfg.MongoDB.Database)
	}
	if len(cfg.Routes) != 1 {
		t.Errorf("expected 1 route, got %d", len(cfg.Routes))
	}
	if cfg.Cache.TTLSeconds != 30 {
		t.Errorf("expected cache TTL 30, got %d", cfg.Cache.TTLSeconds)
	}
	if cfg.Retry.MaxRetries != 3 {
		t.Errorf("expected max retries 3, got %d", cfg.Retry.MaxRetries)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/file.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "invalid-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString("invalid: yaml: content: [[[")
	tmpFile.Close()

	_, err = Load(tmpFile.Name())
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}
