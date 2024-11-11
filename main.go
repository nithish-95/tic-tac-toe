package main

import (
	"html/template"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func renderTemplate(w http.ResponseWriter, file string) {
	tmpl, err := template.ParseFiles(file)
	if err != nil {
		log.Printf("Error parsing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := tmpl.Execute(w, nil); err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func hello(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "./src/index.html")
}

func game(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "./src/game.html")
}

func main() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)

	// Routes
	r.Get("/", hello)
	r.Get("/game.html", game) // New route for game.html

	// Start the server
	log.Println("Starting server on :3000")
	if err := http.ListenAndServe(":3000", r); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
