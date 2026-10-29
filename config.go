package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v2"
)

type Callback struct {
	Emoji   string `yaml:"emoji"`
	Message string `yaml:"message"`
}

// loadCallbacks reads and validates config.yaml.
// It fails instead of starting with an empty or partly parsed config,
// so a broken config does not look like a silently unresponsive bot.
func loadCallbacks(path string) ([]Callback, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	return parseCallbacks(bytes)
}

func parseCallbacks(bytes []byte) ([]Callback, error) {
	var callbacks []Callback
	if err := yaml.UnmarshalStrict(bytes, &callbacks); err != nil {
		return nil, fmt.Errorf("failed to parse callbacks: %w", err)
	}
	if len(callbacks) == 0 {
		return nil, errors.New("no callbacks are defined")
	}
	for i, c := range callbacks {
		if strings.TrimSpace(c.Emoji) == "" {
			return nil, fmt.Errorf("callback #%d: emoji is empty", i)
		}
		if strings.TrimSpace(c.Message) == "" {
			return nil, fmt.Errorf("callback #%d: message is empty", i)
		}
		callbacks[i].Emoji = strings.TrimSpace(c.Emoji)
	}
	return callbacks, nil
}

// parseIDList parses a comma separated list such as "B01,B02".
func parseIDList(s string) map[string]struct{} {
	ids := map[string]struct{}{}
	for _, id := range strings.Split(s, ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			ids[id] = struct{}{}
		}
	}
	return ids
}
