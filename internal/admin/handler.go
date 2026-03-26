package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"api-gateway/internal/config"
	"api-gateway/internal/gateway"
	"api-gateway/internal/models"
	"api-gateway/internal/repository"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Handler struct {
	repo      *repository.MongoRepository
	gateway   *gateway.Gateway
	jwtSecret []byte
}

func NewHandler(repo *repository.MongoRepository, gw *gateway.Gateway, jwtSecret string) *Handler {
	return &Handler{
		repo:      repo,
		gateway:   gw,
		jwtSecret: []byte(jwtSecret),
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/admin/routes", h.handleRoutes)
	mux.HandleFunc("/admin/stats", h.handleStats)
	mux.HandleFunc("/admin/keys", h.handleKeys)
	mux.HandleFunc("/admin/token", h.handleGenerateToken)
}

func (h *Handler) handleRoutes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		routes, err := h.repo.GetRoutes(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to fetch routes")
			return
		}
		json.NewEncoder(w).Encode(routes)

	case http.MethodPost:
		var route models.Route
		if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if err := h.repo.InsertRoute(r.Context(), &route); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create route")
			return
		}

		// Update gateway routes
		h.refreshGatewayRoutes(r)

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(route)

	case http.MethodDelete:
		// Extract ID from query: /admin/routes?id=xxx
		id := r.URL.Query().Get("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "missing route id")
			return
		}
		if err := h.repo.DeleteRoute(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete route")
			return
		}

		h.refreshGatewayRoutes(r)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "route deleted"})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	stats, err := h.repo.GetStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch stats")
		return
	}
	json.NewEncoder(w).Encode(stats)
}

func (h *Handler) handleKeys(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var input struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	apiKey := &models.ApiKey{
		Key:  uuid.New().String(),
		Name: input.Name,
	}

	if err := h.repo.InsertApiKey(r.Context(), apiKey); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create API key")
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(apiKey)
}

func (h *Handler) handleGenerateToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var input struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": input.UserID,
		"role":    input.Role,
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	})

	tokenString, err := token.SignedString(h.jwtSecret)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"token": tokenString})
}

func (h *Handler) refreshGatewayRoutes(r *http.Request) {
	routes, err := h.repo.GetRoutes(r.Context())
	if err != nil {
		return
	}

	var routeConfigs []config.RouteConfig
	for _, route := range routes {
		routeConfigs = append(routeConfigs, config.RouteConfig{
			Path:      route.Path,
			Target:    route.Target,
			Methods:   route.Methods,
			Protected: route.Protected,
		})
	}
	h.gateway.UpdateRoutes(routeConfigs)
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// CORS middleware for admin endpoints
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/admin/") {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
