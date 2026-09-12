package main

import "embed"

// dist is the built frontend, compiled into the binary so that a deploy is
// one file (PRD §12). `make build` fills the directory from frontend/dist. In
// development it holds only .gitkeep, which is why the pattern needs all:,
// and Caddy sends pages to Vite instead.
//
//go:embed all:dist
var dist embed.FS
