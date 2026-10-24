package utils

import (
	"runtime/debug"

	"github.com/rs/zerolog/log"
)

// Recover keeps a panic in a single connection/packet from taking the whole server down.
// Usage: defer utils.Recover("context")
func Recover(context string) {
	if r := recover(); r != nil {
		log.Error().Msgf("Recovered from panic in %s: %v\n%s", context, r, debug.Stack())
	}
}
