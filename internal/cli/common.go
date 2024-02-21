package cli

import (
	"github.com/joelanford/kpm/internal/console"
)

func handleError(err error) {
	if err != nil {
		console.Fatalf(1, "💥 %v", err)
	}
}
