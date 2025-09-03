package main

import (
	"log"

	"github.com/operator-framework/kpm/internal/cli"
)

func main() {
	if err := cli.Root("kpm").Execute(); err != nil {
		log.Fatal(err)
	}
}
