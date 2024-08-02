package cli

import (
	"fmt"
	"os"
)

func handleError(errMsg string) {
	fmt.Println(errMsg)
	os.Exit(1)
}
