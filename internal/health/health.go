package health

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

type response struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// Handler returns an HTTP handler that serves /healthz and /readyz endpoints.
// upstreamURL is checked for readiness probes.
func Handler(upstreamURL string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, response{Status: "ok"})
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := checkUpstream(upstreamURL); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, response{
				Status: "not ready",
				Reason: "upstream unreachable",
			})
			return
		}
		writeJSON(w, http.StatusOK, response{Status: "ready"})
	})

	return mux
}

func checkUpstream(upstreamURL string) error {
	u, err := url.Parse(upstreamURL)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Head(u.String())
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
