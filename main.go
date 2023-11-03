package main

import (
	"compress/gzip"
	"context"
	"io"
	"io/fs"
	"log"
	"os"

	"github.com/spf13/cobra"
	"oras.land/oras-go/v2/content/oci"

	buildv1 "github.com/joelanford/kpm/build/v1"
	"github.com/joelanford/kpm/internal/tar"
	kpmoci "github.com/joelanford/kpm/oci"
)

func main() {
	cmd := cobra.Command{
		Use:  "kpm <spec-file> <output-file>",
		Args: cobra.ExactArgs(2),
	}
	var (
		workingDirectory string
	)
	cmd.Flags().StringVarP(&workingDirectory, "working-directory", "C", "", "working directory used to resolve relative paths for bundle content")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		ctx := cmd.Context()
		specFile, err := os.Open(args[0])
		if err != nil {
			return err
		}
		bundle, err := buildv1.Bundle(specFile, os.DirFS(workingDirectory))
		if err != nil {
			return err
		}

		tmpDir, err := os.MkdirTemp("", "kpm-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)
		tmpStore, err := oci.NewWithContext(ctx, tmpDir)
		if err != nil {
			return err
		}
		desc, err := kpmoci.Push(ctx, bundle, tmpStore, kpmoci.PushOptions{})
		if err != nil {
			return err
		}
		tag := bundle.String()
		if err := tmpStore.Tag(ctx, desc, tag); err != nil {
			return err
		}
		log.Printf("pushed %s and tagged as %q", desc.Digest, tag)

		outputFile, err := os.Create(args[1])
		if err != nil {
			return err
		}
		defer outputFile.Close()
		if err := writeKpmArchive(os.DirFS(tmpDir), outputFile); err != nil {
			defer os.Remove(args[1])
			return err
		}
		log.Printf("wrote %s", args[1])
		return nil
	}

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func writeKpmArchive(src fs.FS, w io.Writer) error {
	gzw := gzip.NewWriter(w)
	defer gzw.Close()
	return tar.Directory(gzw, src)
}
