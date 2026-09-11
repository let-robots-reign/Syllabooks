package handlers

import (
	"encoding/json"
	"net/http"
)

type Book struct {
	ID    int    `json:"id"`
	Title string `json:"name"`
}

var books = []Book{
	{ID: 1, Title: "Learn Go"},
	{ID: 2, Title: "Build REST API"},
}

func BooksHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		json.NewEncoder(w).Encode(books)
	case http.MethodPost:
		var b Book
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		b.ID = len(books) + 1
		books = append(books, b)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(b)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
