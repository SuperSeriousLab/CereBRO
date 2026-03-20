// cerebro-server exposes the CereBRO cognitive pipeline as an HTTP service.
//
// Endpoints:
//
//	POST /analyze                   — Run the adaptive pipeline on a ConversationSnapshot
//	GET  /health                    — Health check
//	GET  /info                      — Pipeline variant and configuration info
//	POST /findings/{id}/outcome     — Validate a finding (TP or FP)
//	GET  /findings?detector=X&limit=20 — List recent findings with outcome status
//	GET  /metrics/detectors         — Per-detector TP/FP precision metrics
//	GET  /v1/debug/recent           — Last 100 pipeline executions (ring buffer, ?minutes=N)
//
// Usage:
//
//	cerebro-server --addr :8070 [--sophrim http://192.168.14.65:8090] [--outcomes /var/lib/cerebro/outcomes.ndjson]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	sophrimTimeout         = 200 * time.Millisecond
	defaultSophrimEndpoint = "http://192.168.14.65:8090"
	readTimeout            = 10 * time.Second
	writeTimeout           = 30 * time.Second
	idleTimeout            = 60 * time.Second
	maxBodyBytes           = 4 * 1024 * 1024 // 4 MB
)

var (
	version   = "dev"
	startTime time.Time
)

func main() {
	addr := flag.String("addr", ":8070", "listen address")
	sophrimURL := flag.String("sophrim", sophrimEndpointDefault(), "Sophrim endpoint for domain context")
	ptsURL := flag.String("pts", ptsEndpointDefault(), "PTS endpoint for anomaly signals")
	outcomesPath := flag.String("outcomes", pipeline.DefaultOutcomesPath, "Path to NDJSON outcomes file")
	flag.Parse()

	startTime = time.Now()

	var sophrimClient *pipeline.SophrimClient
	if *sophrimURL != "" {
		sophrimClient = pipeline.NewSophrimClient(*sophrimURL, sophrimTimeout)
	}

	store := pipeline.NewOutcomeStore(*outcomesPath)
	log.Printf("cerebro-server: outcome store at %s", *outcomesPath)

	ring := NewDebugRing()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/info", infoHandler)
	mux.HandleFunc("/analyze", analyzeHandler(sophrimClient, *ptsURL, store, ring))
	mux.HandleFunc("/findings", findingsHandler(store))
	mux.HandleFunc("/findings/", findingOutcomeHandler(store))
	mux.HandleFunc("/metrics/detectors", detectorMetricsHandler(store))
	mux.HandleFunc("/v1/debug/recent", handleDebugRecent(ring))

	srv := &http.Server{
		Addr:         *addr,
		Handler:      mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("cerebro-server: listening on %s (sophrim=%s, pts=%s)", *addr, *sophrimURL, *ptsURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("cerebro-server: listen error: %v", err)
		}
	}()

	<-done
	log.Println("cerebro-server: shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("cerebro-server: shutdown error: %v", err)
	}
	log.Println("cerebro-server: stopped")
}

// healthHandler returns a simple status JSON.
func healthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"uptime": time.Since(startTime).String(),
	})
}

// infoHandler returns pipeline variant information.
func infoHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service":          "cerebro",
		"version":          version,
		"default_variant":  pipeline.AdaptiveVariantName(nil),
		"classical_variant": pipeline.AdaptiveVariantName(&pipeline.DomainContext{TextEra: "classical", Confidence: 0.9}),
	})
}

// analyzeRequest is the JSON envelope accepted by POST /analyze.
type analyzeRequest struct {
	// Snapshot is the conversation to analyze. Accepts the protobuf JSON
	// encoding of cog.reasoning.v1.ConversationSnapshot.
	Snapshot json.RawMessage `json:"snapshot"`

	// DomainContext is optional upstream domain context. When omitted and
	// Sophrim is configured, the server fetches it automatically.
	DomainContext *domainContextRequest `json:"domain_context,omitempty"`
}

type domainContextRequest struct {
	PrimaryDomain string  `json:"primary_domain"`
	TextEra       string  `json:"text_era"`
	Confidence    float64 `json:"confidence"`
}

// analyzeResponse is the JSON response from POST /analyze.
type analyzeResponse struct {
	Variant        string  `json:"variant"`
	IntegrityScore float64 `json:"integrity_score"`
	FindingCount   int     `json:"finding_count"`
	CriticalCount  uint32  `json:"critical_count"`
	WarningCount   uint32  `json:"warning_count"`
	CautionCount   uint32  `json:"caution_count"`
	Rejected       bool    `json:"rejected"`
	DurationMS     int64   `json:"duration_ms"`

	// Findings contains the full CognitiveAssessment findings in protobuf JSON.
	Findings json.RawMessage `json:"findings,omitempty"`

	// Report contains the full CerebroReport in protobuf JSON.
	Report json.RawMessage `json:"report,omitempty"`
}

func analyzeHandler(sophrimClient *pipeline.SophrimClient, ptsEndpoint string, store *pipeline.OutcomeStore, ring *DebugRing) http.HandlerFunc {
	pjUnmarshaler := protojson.UnmarshalOptions{DiscardUnknown: true}
	pjMarshaler := protojson.MarshalOptions{EmitUnpopulated: false}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		// Limit body size.
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

		var req analyzeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
			return
		}

		if len(req.Snapshot) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "snapshot is required"})
			return
		}

		// Unmarshal the protobuf snapshot from JSON.
		var snap reasoningv1.ConversationSnapshot
		if err := pjUnmarshaler.Unmarshal(req.Snapshot, &snap); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid snapshot: " + err.Error()})
			return
		}

		// Compute input length (total chars across all turns) for the debug ring.
		inputLen := 0
		for _, t := range snap.GetTurns() {
			inputLen += len(t.GetRawText())
		}

		// Resolve domain context: explicit > Sophrim fetch > nil.
		var domain *pipeline.DomainContext
		if req.DomainContext != nil {
			domain = &pipeline.DomainContext{
				PrimaryDomain: req.DomainContext.PrimaryDomain,
				TextEra:       req.DomainContext.TextEra,
				Confidence:    req.DomainContext.Confidence,
			}
		} else if sophrimClient != nil {
			summary := snapshotSummary(&snap)
			if summary != "" {
				ctx, cancel := context.WithTimeout(r.Context(), sophrimTimeout)
				defer cancel()
				domain = sophrimClient.FetchDomainContext(ctx, summary)
			}
		}

		start := time.Now()
		result, err := pipeline.RunAdaptive(&snap, domain, ptsEndpoint, store)
		elapsed := time.Since(start)

		if err != nil {
			ring.Record(PipelineLog{
				Timestamp:   time.Now(),
				InputLength: inputLen,
				LatencyMs:   elapsed.Milliseconds(),
				Status:      "error",
				Error:       err.Error(),
			})
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "pipeline error: " + err.Error()})
			return
		}

		// Determine mode: "enriched" when ML enrichments are present, else "deterministic".
		mode := "deterministic"
		if len(result.MLEnrichments) > 0 {
			mode = "enriched"
		}
		debugStatus := "ok"
		if result.Rejected {
			debugStatus = "rejected"
		}
		ring.Record(PipelineLog{
			Timestamp:     time.Now(),
			InputLength:   inputLen,
			DetectorCount: len(result.Findings),
			FindingsCount: len(result.Findings),
			LatencyMs:     elapsed.Milliseconds(),
			Mode:          mode,
			Status:        debugStatus,
		})

		resp := analyzeResponse{
			Variant:    pipeline.AdaptiveVariantName(domain),
			Rejected:   result.Rejected,
			DurationMS: elapsed.Milliseconds(),
		}

		if result.Report != nil {
			resp.IntegrityScore = result.Report.GetOverallIntegrityScore()
			resp.CriticalCount = result.Report.GetCriticalCount()
			resp.WarningCount = result.Report.GetWarningCount()
			resp.CautionCount = result.Report.GetCautionCount()
		}

		resp.FindingCount = len(result.Findings)

		// Marshal findings to protobuf JSON.
		if len(result.Findings) > 0 {
			findingsJSON := make([]json.RawMessage, len(result.Findings))
			for i, f := range result.Findings {
				b, err := pjMarshaler.Marshal(f)
				if err == nil {
					findingsJSON[i] = b
				}
			}
			if b, err := json.Marshal(findingsJSON); err == nil {
				resp.Findings = b
			}
		}

		// Marshal the CerebroReport.
		cerebroReport := result.ToCerebroReport()
		if cerebroReport != nil {
			if b, err := pjMarshaler.Marshal(cerebroReport); err == nil {
				resp.Report = b
			}
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// snapshotSummary extracts a text summary for Sophrim queries.
func snapshotSummary(snap *reasoningv1.ConversationSnapshot) string {
	if snap == nil {
		return ""
	}
	if obj := strings.TrimSpace(snap.GetObjective()); obj != "" {
		return obj
	}
	var sb strings.Builder
	for i, turn := range snap.GetTurns() {
		if i >= 3 {
			break
		}
		if i > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(strings.TrimSpace(turn.GetRawText()))
	}
	summary := sb.String()
	if len(summary) > 500 {
		summary = summary[:500]
	}
	return summary
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func sophrimEndpointDefault() string {
	if ep := os.Getenv("SOPHRIM_ENDPOINT"); ep != "" {
		return ep
	}
	return defaultSophrimEndpoint
}

func ptsEndpointDefault() string {
	if ep := os.Getenv("PTS_ENDPOINT"); ep != "" {
		return ep
	}
	return "http://192.168.14.68:9746"
}

func init() {
	if v := os.Getenv("CEREBRO_VERSION"); v != "" {
		version = v
	}
}

// ── Outcome / metrics endpoints ──────────────────────────────────────────────

// outcomeRequest is the payload for POST /findings/{id}/outcome.
type outcomeRequest struct {
	Correct bool   `json:"correct"` // true = TP, false = FP
	Notes   string `json:"notes"`   // optional human annotation
}

// findingsHandler handles GET /findings?detector=X&limit=N.
func findingsHandler(store *pipeline.OutcomeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		detector := r.URL.Query().Get("detector")
		limit := 20
		if lStr := r.URL.Query().Get("limit"); lStr != "" {
			if n := parseInt(lStr); n > 0 {
				limit = n
			}
		}
		findings, err := store.List(detector, limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if findings == nil {
			findings = []pipeline.FindingOutcome{}
		}
		writeJSON(w, http.StatusOK, findings)
	}
}

// findingOutcomeHandler handles POST /findings/{id}/outcome.
// URL pattern: /findings/<id>/outcome
func findingOutcomeHandler(store *pipeline.OutcomeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Expect path: /findings/{id}/outcome
		path := strings.TrimPrefix(r.URL.Path, "/findings/")
		path = strings.TrimSuffix(path, "/outcome")
		id := strings.TrimSpace(path)

		if id == "" || !strings.HasSuffix(r.URL.Path, "/outcome") {
			// Not an outcome path — treat as list endpoint redirect.
			findingsHandler(store)(w, r)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		var req outcomeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
			return
		}

		if err := store.Validate(id, req.Correct, req.Notes); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			} else {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			}
			return
		}

		finding, err := store.GetByID(id)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]string{"status": "validated"})
			return
		}
		writeJSON(w, http.StatusOK, finding)
	}
}

// detectorMetricsHandler handles GET /metrics/detectors.
func detectorMetricsHandler(store *pipeline.OutcomeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		metrics, err := store.Metrics()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if metrics == nil {
			metrics = []pipeline.DetectorMetrics{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"detectors": metrics,
		})
	}
}

// parseInt parses a decimal string. Returns 0 on failure.
func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
