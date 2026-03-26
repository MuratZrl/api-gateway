package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
)

type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

var (
	users = map[string]User{
		"1": {ID: "1", Name: "Ali Yılmaz", Email: "ali@example.com"},
		"2": {ID: "2", Name: "Ayşe Demir", Email: "ayse@example.com"},
		"3": {ID: "3", Name: "Mehmet Kaya", Email: "mehmet@example.com"},
	}
	mu sync.RWMutex
	nextID = 4
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodGet:
			mu.RLock()
			userList := make([]User, 0, len(users))
			for _, u := range users {
				userList = append(userList, u)
			}
			mu.RUnlock()
			json.NewEncoder(w).Encode(userList)

		case http.MethodPost:
			var user User
			if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid body"})
				return
			}
			mu.Lock()
			user.ID = fmt.Sprintf("%d", nextID)
			nextID++
			users[user.ID] = user
			mu.Unlock()

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(user)

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"service": "user-service", "status": "ok"}`))
	})

	log.Println("User Service starting on :8081")
	log.Fatal(http.ListenAndServe(":8081", mux))
}
