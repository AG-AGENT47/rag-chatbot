package guardrails

import (
	"errors"
	"strings"
)

// ErrTooLong is returned when the input exceeds the maximum length.
var ErrTooLong = errors.New("message too long")

// ErrInjection is returned when the input contains a known injection pattern.
var ErrInjection = errors.New("invalid input")

var injectionPatterns = []string{
	"ignore previous",
	"system prompt",
	"act as",
	"pretend you are",
	"you are now",
	"disregard",
	"jailbreak",
}

// Validate checks the input for guardrail violations.
// Returns ErrTooLong or ErrInjection if a violation is detected.
func Validate(msg string) error {
	if len(msg) > 1000 {
		return ErrTooLong
	}
	lower := strings.ToLower(msg)
	for _, pattern := range injectionPatterns {
		if strings.Contains(lower, pattern) {
			return ErrInjection
		}
	}
	return nil
}
