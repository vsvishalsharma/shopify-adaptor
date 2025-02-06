package main

import (
	"log"
	"net/http"
)

func main() {
	// Initialize configuration
	initConfig()

	// Set up routes
	http.HandleFunc("/search", searchHandler)
	
	// Log server startup
	log.Println("Server started on :9090")
	
	// Start HTTP server
	log.Fatal(http.ListenAndServe(":9090", nil))
}