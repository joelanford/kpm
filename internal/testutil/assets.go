package testutil

import (
	"embed"
)

//go:embed testdata/*
var testdata embed.FS
