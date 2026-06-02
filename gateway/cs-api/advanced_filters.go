package csapi

import (
	"encoding/json"
	"net/url"
	"strings"
	"time"
)

type timeRange struct {
	start time.Time
	end   time.Time
}

func queryString(q url.Values, name string) string {
	return strings.TrimSpace(q.Get(name))
}

func queryHasAny(q url.Values, names ...string) bool {
	for _, name := range names {
		if queryString(q, name) != "" {
			return true
		}
	}
	return false
}

func resourceTimeIntersects(candidate, requested string) bool {
	candidateRange, ok := parseResourceTimeRange(candidate)
	if !ok {
		return false
	}
	requestedRange, ok := parseResourceTimeRange(requested)
	if !ok {
		return false
	}
	return !candidateRange.end.Before(requestedRange.start) && !requestedRange.end.Before(candidateRange.start)
}

func parseResourceTimeRange(value string) (timeRange, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return timeRange{}, false
	}
	if strings.Contains(value, "/") {
		parts := strings.Split(value, "/")
		if len(parts) != 2 {
			return timeRange{}, false
		}
		start, ok := parseResourceTimeEndpoint(parts[0], time.Time{})
		if !ok {
			return timeRange{}, false
		}
		end, ok := parseResourceTimeEndpoint(parts[1], time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC))
		if !ok {
			return timeRange{}, false
		}
		return timeRange{start: start, end: end}, true
	}
	instant, ok := parseResourceTimeEndpoint(value, time.Time{})
	if !ok {
		return timeRange{}, false
	}
	return timeRange{start: instant, end: instant}, true
}

func parseResourceTimeEndpoint(value string, openValue time.Time) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" || value == ".." {
		return openValue, true
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func propertyDefinitionsContain[T interface{ getDefinition() string }](properties []T, expected string) bool {
	if expected == "" {
		return true
	}
	for _, property := range properties {
		if property.getDefinition() == expected {
			return true
		}
	}
	return false
}

func jsonResourceString(resource json.RawMessage, name string) string {
	var obj map[string]any
	if err := json.Unmarshal(resource, &obj); err != nil {
		return ""
	}
	if value, ok := obj[name].(string); ok {
		return value
	}
	return ""
}
