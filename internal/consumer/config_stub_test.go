package consumer

import (
	"fmt"
	"strings"
)

type stubConfig map[string]string

func (s stubConfig) Get(key string) (string, error) {
	if val, ok := s[key]; ok && strings.TrimSpace(val) != "" {
		return val, nil
	}
	return "", fmt.Errorf("config key %s not found", key)
}
