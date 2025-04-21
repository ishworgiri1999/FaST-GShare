package main_test

import (
	"context"
	"log"

	"entgo.io/ent/dialect"
	"github.com/KontonGu/FaST-GShare/internal/db/ent"
	_ "github.com/mattn/go-sqlite3"
)

func Example_test() {
	// Create an ent.Client with in-memory SQLite database.
	client, err := ent.Open(dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1")
	if err != nil {
		log.Fatalf("failed opening connection to sqlite: %v", err)
	}

	defer client.Close()
	ctx := context.Background()
	// Run the automatic migration tool to create all schema resources.
	if err := client.Schema.Create(ctx); err != nil {
		log.Fatalf("failed creating schema resources: %v", err)
	}
	// Output:
}
