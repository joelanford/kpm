package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"

	convertv1 "github.com/joelanford/kpm/internal/experimental/pkg/convert/v1"
)

func main() {
	fbcDir := os.Args[1]
	outDir := os.Args[2]

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	if pprofFilePath := os.Getenv("PPROF"); pprofFilePath != "" {
		pprofFile, err := os.Create(pprofFilePath)
		if err != nil {
			log.Fatal(err)
		}
		defer pprofFile.Close()
		if err := pprof.StartCPUProfile(pprofFile); err != nil {
			log.Fatal(err)
		}
		defer pprof.StopCPUProfile()
	}

	desc, err := convertv1.FBCToOCI(ctx, os.DirFS(fbcDir), outDir)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(desc)
}
