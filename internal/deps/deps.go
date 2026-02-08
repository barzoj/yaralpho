// Package deps exists solely to pin core third-party modules during early scaffolding.
// It will be removed once production code references these dependencies directly.
package deps

import (
	_ "github.com/github/copilot-sdk/go"
	_ "github.com/gorilla/mux"
	_ "github.com/slack-go/slack"
	_ "github.com/stretchr/testify/assert"
	_ "go.mongodb.org/mongo-driver/mongo"
	_ "go.uber.org/zap"
)
