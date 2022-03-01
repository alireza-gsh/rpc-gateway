package main

import (
	"net/http"
)

const (
	Reroutes int = iota
	Retries
	TargetName
)

// GetReroutesFromContext returns the attempts for request
func GetReroutesFromContext(r *http.Request) uint {
	if reroutes, ok := r.Context().Value(Reroutes).(uint); ok {
		return reroutes
	}
	return 0
}

// GetRetryFromContext returns the attempts for request
func GetRetryFromContext(r *http.Request) uint {
	if retries, ok := r.Context().Value(Retries).(uint); ok {
		return retries
	}
	return 0
}

// GetTargetNameFromContext returns the attempts for request
func GetTargetNameFromContext(r *http.Request) string {
	if targetName, ok := r.Context().Value(TargetName).(string); ok {
		return targetName
	}
	return ""
}

func ReadUserIP(r *http.Request) string {
	IPAddress := r.Header.Get("X-Real-Ip")
	if IPAddress == "" {
		IPAddress = r.Header.Get("X-Forwarded-For")
	}
	if IPAddress == "" {
		IPAddress = r.RemoteAddr
	}
	return IPAddress
}
