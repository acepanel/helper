package embed

import "embed"

//go:embed all:locales/*
var LocalesFS embed.FS
