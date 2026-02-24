//go:build ignore

package main

import (
	"log"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

func main() {
	if err := entc.Generate("./schema", &gen.Config{
		Package: "haruki-cloud/database/bot",
		Target:  "../../database/bot",
	}); err != nil {
		log.Fatal("running ent codegen:", err)
	}
}
