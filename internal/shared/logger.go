package shared

import (
	"encoding/json"
	"fmt"
	"time"
)

// LogFields is an alias for a map of structured log fields.
type LogFields map[string]any

// LogEvent writes a structured JSON log line to stdout.
func LogEvent(event string, fields LogFields) {
	payload := map[string]any{
		"ts":      time.Now().UTC().Format(time.RFC3339),
		"service": "loyalty-core-points",
		"event":   event,
	}

	for key, value := range fields {
		if value != nil {
			payload[key] = value
		}
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("{\"ts\":\"%s\",\"service\":\"loyalty-core-points\",\"event\":\"logger.error\",\"message\":\"json_marshal_failed\"}\n",
			time.Now().UTC().Format(time.RFC3339))
		return
	}

	fmt.Println(string(bytes))
}
