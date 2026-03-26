package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"api-gateway/internal/admin"
	"api-gateway/internal/config"
	"api-gateway/internal/gateway"
	"api-gateway/internal/middleware"
	"api-gateway/internal/repository"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

func main() {
	// Load config
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "configs/gateway.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize OpenTelemetry tracing
	if cfg.Tracing.Enabled {
		tp, err := middleware.InitTracer(context.Background(), "api-gateway", cfg.Tracing.Endpoint)
		if err != nil {
			log.Printf("Warning: Failed to initialize tracing: %v", err)
		} else {
			defer tp.Shutdown(context.Background())
			middleware.LogInfo("Tracing initialized", map[string]interface{}{"endpoint": cfg.Tracing.Endpoint})
		}
	}

	// Connect to MongoDB
	repo, err := repository.NewMongoRepository(cfg.MongoDB.URI, cfg.MongoDB.Database)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	middleware.LogInfo("Connected to MongoDB", map[string]interface{}{"uri": cfg.MongoDB.URI})

	// Connect to Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	middleware.LogInfo("Connected to Redis", map[string]interface{}{"addr": cfg.Redis.Addr})

	// Create gateway
	gw := gateway.New(cfg.Routes)
	middleware.LogInfo("Routes loaded", map[string]interface{}{"count": len(cfg.Routes)})

	// Create middleware
	rateLimiter := middleware.NewRateLimiter(redisClient, cfg.RateLimit.RequestsPerMinute)
	auth := middleware.NewAuth(cfg.JWT.Secret, gw, repo)
	cbManager := middleware.NewCircuitBreakerManager(
		cfg.CircuitBreaker.MaxFailures,
		cfg.CircuitBreaker.Timeout,
		cfg.CircuitBreaker.HalfOpenMaxRequests,
	)

	// Cache middleware
	cache := middleware.NewCache(redisClient, cfg.Cache.TTLSeconds)

	// IP Filter middleware
	ipFilter := middleware.NewIPFilter(
		middleware.IPFilterMode(cfg.IPFilter.Mode),
		cfg.IPFilter.Whitelist,
		cfg.IPFilter.Blacklist,
	)

	// Retry middleware
	retryCfg := &middleware.RetryConfig{
		MaxRetries:  cfg.Retry.MaxRetries,
		InitialWait: time.Duration(cfg.Retry.InitialWaitMs) * time.Millisecond,
		MaxWait:     time.Duration(cfg.Retry.MaxWaitMs) * time.Millisecond,
		Multiplier:  cfg.Retry.Multiplier,
	}

	// Structured logger
	structLogger := middleware.NewStructuredLogger()

	// Request Validation schemas
	minPrice := 0.0
	maxPrice := 1000000.0
	validator := middleware.NewRequestValidator(map[string]*middleware.ValidationSchema{
		"POST /api/users": {
			Fields: map[string]middleware.FieldRule{
				"name":  {Type: "string", Required: true, MinLen: 2, MaxLen: 100},
				"email": {Type: "string", Required: true, MinLen: 5, MaxLen: 255},
			},
		},
		"POST /api/products": {
			Fields: map[string]middleware.FieldRule{
				"name":  {Type: "string", Required: true, MinLen: 2, MaxLen: 200},
				"price": {Type: "number", Required: true, Min: &minPrice, Max: &maxPrice},
				"stock": {Type: "number", Required: false},
			},
		},
	})

	// Request/Response Transform rules
	transformRules := map[string]*middleware.TransformRule{
		"/api/": {
			AddRequestHeaders: map[string]string{
				"X-Gateway":    "api-gateway",
				"X-Request-ID": "auto",
			},
			AddResponseHeaders: map[string]string{
				"X-Powered-By": "API Gateway",
			},
			RemoveResponseHeaders: []string{"Server"},
		},
	}

	// Setup routes
	mux := http.NewServeMux()

	// Admin API
	adminHandler := admin.NewHandler(repo, gw, cfg.JWT.Secret)
	adminHandler.RegisterRoutes(mux)

	// Prometheus metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status": "ok", "version": "1.0.0"}`))
	})

	// Gateway handler for all other routes
	mux.Handle("/api/", gw)

	// Chain middleware (outermost runs first):
	// CORS -> Metrics -> Tracing -> IPFilter -> StructuredLog -> Logging -> RateLimit -> Auth -> Retry -> CircuitBreaker -> Cache -> Validation -> Transform -> Handler
	var handler http.Handler = mux
	handler = middleware.Transform(transformRules)(handler)
	handler = validator.Middleware(handler)
	if cfg.Cache.Enabled {
		handler = cache.Middleware(handler)
	}
	handler = cbManager.Middleware(handler)
	handler = middleware.Retry(retryCfg)(handler)
	handler = auth.Middleware(handler)
	handler = rateLimiter.Middleware(handler)
	handler = middleware.Logging(repo)(handler)
	handler = structLogger.Middleware(handler)
	handler = ipFilter.Middleware(handler)
	handler = middleware.Tracing(handler)
	handler = middleware.Metrics(handler)
	handler = admin.CORS(handler)

	// Start server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
	}

	middleware.LogInfo("API Gateway starting", map[string]interface{}{
		"addr":            addr,
		"rate_limit":      cfg.RateLimit.RequestsPerMinute,
		"cache_enabled":   cfg.Cache.Enabled,
		"cache_ttl":       cfg.Cache.TTLSeconds,
		"ip_filter":       cfg.IPFilter.Mode,
		"retry":           cfg.Retry.MaxRetries,
		"circuit_breaker": cfg.CircuitBreaker.MaxFailures,
		"tracing":         cfg.Tracing.Enabled,
	})

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
