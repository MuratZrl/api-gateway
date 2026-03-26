package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
)

type Product struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
	Stock int     `json:"stock"`
}

var (
	products = map[string]Product{
		"1": {ID: "1", Name: "Laptop", Price: 25999.99, Stock: 50},
		"2": {ID: "2", Name: "Klavye", Price: 899.99, Stock: 200},
		"3": {ID: "3", Name: "Monitor", Price: 8499.99, Stock: 30},
	}
	mu     sync.RWMutex
	nextID = 4
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/products", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodGet:
			mu.RLock()
			productList := make([]Product, 0, len(products))
			for _, p := range products {
				productList = append(productList, p)
			}
			mu.RUnlock()
			json.NewEncoder(w).Encode(productList)

		case http.MethodPost:
			var product Product
			if err := json.NewDecoder(r.Body).Decode(&product); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid body"})
				return
			}
			mu.Lock()
			product.ID = fmt.Sprintf("%d", nextID)
			nextID++
			products[product.ID] = product
			mu.Unlock()

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(product)

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"service": "product-service", "status": "ok"}`))
	})

	log.Println("Product Service starting on :8082")
	log.Fatal(http.ListenAndServe(":8082", mux))
}
