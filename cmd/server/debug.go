// Copyright 2026 SuperSeriousLab
// Licensed under the Apache License, Version 2.0

package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const debugRingSize = 100

// PipelineLog is a single completed pipeline execution recorded in the debug ring.
type PipelineLog struct {
	Timestamp      time.Time `json:"ts"`
	InputLength    int       `json:"input_length"`
	DetectorCount  int       `json:"detector_count"`
	FindingsCount  int       `json:"findings_count"`
	LatencyMs      int64     `json:"latency_ms"`
	Mode           string    `json:"mode"`   // "deterministic" or "enriched"
	Status         string    `json:"status"` // "ok", "rejected", "error"
	Error          string    `json:"error,omitempty"`
}

// DebugRing is a thread-safe circular buffer of the last 100 pipeline executions.
type DebugRing struct {
	mu   sync.Mutex
	buf  [debugRingSize]PipelineLog
	head int // next write position (mod debugRingSize)
	size int // number of valid entries (capped at debugRingSize)
}

// NewDebugRing allocates a DebugRing ready for use.
func NewDebugRing() *DebugRing {
	return &DebugRing{}
}

// Record appends a PipelineLog entry to the ring, overwriting the oldest when full.
func (r *DebugRing) Record(entry PipelineLog) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf[r.head] = entry
	r.head = (r.head + 1) % debugRingSize
	if r.size < debugRingSize {
		r.size++
	}
}

// Since returns all entries whose Timestamp is within the given window ending
// at now. The returned slice is ordered oldest-first.
func (r *DebugRing) Since(window time.Duration) []PipelineLog {
	cutoff := time.Now().Add(-window)
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]PipelineLog, 0, r.size)
	start := (r.head - r.size + debugRingSize) % debugRingSize
	for i := range r.size {
		idx := (start + i) % debugRingSize
		if !r.buf[idx].Timestamp.Before(cutoff) {
			out = append(out, r.buf[idx])
		}
	}
	return out
}

// pipelineSummary holds aggregate stats for a slice of PipelineLog entries.
type pipelineSummary struct {
	Total         int   `json:"total"`
	OK            int   `json:"ok"`
	Rejected      int   `json:"rejected"`
	Errors        int   `json:"errors"`
	AvgLatencyMs  int64 `json:"avg_latency_ms"`
	AvgLatencyOkMs int64 `json:"avg_latency_ok_ms"`
}

func summarisePipeline(entries []PipelineLog) pipelineSummary {
	s := pipelineSummary{Total: len(entries)}
	var sumAll, sumOk int64
	var countOk int
	for _, e := range entries {
		sumAll += e.LatencyMs
		switch e.Status {
		case "ok":
			s.OK++
			sumOk += e.LatencyMs
			countOk++
		case "rejected":
			s.Rejected++
		default:
			s.Errors++
		}
	}
	if s.Total > 0 {
		s.AvgLatencyMs = sumAll / int64(s.Total)
	}
	if countOk > 0 {
		s.AvgLatencyOkMs = sumOk / int64(countOk)
	}
	return s
}

// handleDebugRecent serves GET /v1/debug/recent.
// Optional query parameter ?minutes=N (default 5) filters the window.
func handleDebugRecent(ring *DebugRing) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		minutes := 5
		if v := r.URL.Query().Get("minutes"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				minutes = n
			}
		}

		window := time.Duration(minutes) * time.Minute
		entries := ring.Since(window)

		resp := struct {
			WindowMinutes int             `json:"window_minutes"`
			Executions    []PipelineLog   `json:"executions"`
			Summary       pipelineSummary `json:"summary"`
		}{
			WindowMinutes: minutes,
			Executions:    entries,
			Summary:       summarisePipeline(entries),
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, `{"error":"encode failed"}`, http.StatusInternalServerError)
		}
	}
}
