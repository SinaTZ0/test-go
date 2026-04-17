package main

import (
	"context"
	"log"
	"net/http"
	"os"
)

const defaultDatabaseURL = "postgres://user:password@localhost:5432/go-todo"

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = defaultDatabaseURL
	}

	app, err := NewApp(context.Background(), databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if closeErr := app.Close(); closeErr != nil {
			log.Printf("close app: %v", closeErr)
		}
	}()

	log.Println("server starting at :8899")
	log.Fatal(http.ListenAndServe(":8899", app))
}
