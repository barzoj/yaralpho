package app

// Version is the application version identifier. It is intended to be set at
// build time via -ldflags "-X github.com/barzoj/yaralpho/internal/app.Version=$(git rev-parse HEAD)".
// When unset, it defaults to "dev".
var Version = "dev"
