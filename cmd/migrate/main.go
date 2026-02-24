package main

import (
	"context"
	"haruki-cloud/database/sekai"
	"log"

	_ "github.com/lib/pq"
)

func main() {
	client, err := sekai.Open("postgres", "host=localhost port=5432 user=postgres dbname=haruki_sekai password=postgres sslmode=disable")
	if err != nil {
		log.Fatalf("failed opening connection to postgres: %v", err)
	}
	defer client.Close()
	// Run the auto migration tool.
	if err := client.Schema.Create(context.Background()); err != nil {
		log.Fatalf("failed creating schema resources: %v", err)
	}
	log.Println("migration complete")
}
