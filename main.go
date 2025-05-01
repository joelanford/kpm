package main

import (
	"log"

	"github.com/joelanford/kpm/internal/cli"
)

func main() {
	if err := cli.Root("kpm").Execute(); err != nil {
		log.Fatal(err)
	}
}
