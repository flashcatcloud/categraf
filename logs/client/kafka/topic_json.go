//go:build !no_logs

package kafka

import (
	_ "github.com/mailru/easyjson/gen"
)

// easyjson:json
type (
	Data struct {
		Topic string `json:"topic"`
	}
)
