package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxBodyBytes = 5 << 20

type Store struct {
	mu     sync.RWMutex
	events map[string]Event
}

var store = &Store{events: make(map[string]Event)}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /events", handlePostEvents)
	mux.HandleFunc("GET /campaigns/{campaignID}/stats", handleGetStats)
	mux.HandleFunc("GET /campaigns/{campaignID}/events", handleGetEvents)
	addr := os.Getenv("PORT")
	if addr == "" {
		addr = "8080"
	}
	if !strings.HasPrefix(addr, ":") {
		addr = ":" + addr
	}
	log.Printf("Relay Campaign Events API listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, loggingMiddleware(mux)))
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

type itemResult struct {
	Index  int    `json:"index"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

func handlePostEvents(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not read request body", "detail": err.Error()})
		return
	}
	var rawEvents []json.RawMessage
	if err := json.Unmarshal(body, &rawEvents); err != nil || rawEvents == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be a JSON array of events"})
		return
	}
	if len(rawEvents) > 1000 {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "batch cannot exceed 1000 events"})
		return
	}

	results := make([]itemResult, 0, len(rawEvents))
	accepted, duplicates, rejected := 0, 0, 0
	store.mu.Lock()
	defer store.mu.Unlock()
	for i, raw := range rawEvents {
		var ev Event
		if err := json.Unmarshal(raw, &ev); err != nil {
			rejected++
			results = append(results, itemResult{i, "rejected", "invalid JSON event: " + err.Error()})
			continue
		}
		if err := validateEvent(ev); err != nil {
			rejected++
			results = append(results, itemResult{i, "rejected", err.Error()})
			continue
		}
		if existing, ok := store.events[ev.EventID]; ok {
			if sameEvent(existing, ev) {
				duplicates++
				results = append(results, itemResult{i, "duplicate", "event_id was already accepted"})
			} else {
				rejected++
				results = append(results, itemResult{i, "rejected", "event_id conflicts with a previously accepted event"})
			}
			continue
		}
		store.events[ev.EventID] = ev
		accepted++
		results = append(results, itemResult{i, "accepted", ""})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"received": len(rawEvents), "accepted": accepted, "duplicates": duplicates,
		"rejected": rejected, "results": results,
	})
}

func validateEvent(ev Event) error {
	if strings.TrimSpace(ev.EventID) == "" {
		return errors.New("event_id is required")
	}
	if strings.TrimSpace(ev.CampaignID) == "" {
		return errors.New("campaign_id is required")
	}
	if strings.TrimSpace(ev.ContactID) == "" {
		return errors.New("contact_id is required")
	}
	switch ev.Type {
	case "sent", "delivered", "opened", "clicked":
	default:
		return errors.New("type must be sent, delivered, opened, or clicked")
	}
	if _, err := time.Parse(time.RFC3339Nano, ev.Timestamp); err != nil {
		return errors.New("timestamp must be a valid RFC3339 timestamp")
	}
	return nil
}

func sameEvent(a, b Event) bool {
	if a.EventID != b.EventID || a.CampaignID != b.CampaignID || a.ContactID != b.ContactID || a.Type != b.Type {
		return false
	}
	ta, errA := time.Parse(time.RFC3339Nano, a.Timestamp)
	tb, errB := time.Parse(time.RFC3339Nano, b.Timestamp)
	if errA != nil || errB != nil || !ta.Equal(tb) {
		return false
	}
	return canonicalJSON(a.Metadata) == canonicalJSON(b.Metadata)
}

// canonicalJSON makes semantically identical metadata compare equally even
// when object key order or insignificant whitespace differs.
func canonicalJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(bytes.TrimSpace(raw))
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return string(bytes.TrimSpace(raw))
	}
	return string(canonical)
}

func handleGetStats(w http.ResponseWriter, r *http.Request) {
	campaignID := r.PathValue("campaignID")
	if strings.TrimSpace(campaignID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "campaign_id is required"})
		return
	}
	stats := map[string]int{"sent": 0, "delivered": 0, "opened": 0, "clicked": 0}
	uniqueContacts := make(map[string]struct{})
	store.mu.RLock()
	for _, ev := range store.events {
		if ev.CampaignID != campaignID {
			continue
		}
		stats[ev.Type]++
		if ev.Type == "opened" {
			uniqueContacts[ev.ContactID] = struct{}{}
		}
	}
	store.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"campaign_id": campaignID, "sent": stats["sent"], "delivered": stats["delivered"],
		"opened": stats["opened"], "clicked": stats["clicked"], "unique_opens": len(uniqueContacts),
	})
}

func handleGetEvents(w http.ResponseWriter, r *http.Request) {
	campaignID := r.PathValue("campaignID")
	limit, offset := 50, 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := parsePositiveInt(v); err == nil {
			limit = n
		} else {
			writeJSON(w, 400, map[string]string{"error": "limit must be a positive integer"})
			return
		}
	}
	if limit > 200 {
		limit = 200
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := parseNonNegativeInt(v); err == nil {
			offset = n
		} else {
			writeJSON(w, 400, map[string]string{"error": "offset must be a non-negative integer"})
			return
		}
	}
	items := make([]Event, 0)
	store.mu.RLock()
	for _, ev := range store.events {
		if ev.CampaignID == campaignID {
			items = append(items, ev)
		}
	}
	store.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool {
		if items[i].Timestamp == items[j].Timestamp {
			return items[i].EventID < items[j].EventID
		}
		return items[i].Timestamp > items[j].Timestamp
	})
	total := len(items)
	if offset >= total {
		items = []Event{}
	} else {
		end := offset + limit
		if end > total {
			end = total
		}
		items = items[offset:end]
	}
	writeJSON(w, 200, map[string]any{"campaign_id": campaignID, "total": total, "limit": limit, "offset": offset, "events": items})
}

func parsePositiveInt(s string) (int, error) {
	n, err := parseNonNegativeInt(s)
	if err == nil && n == 0 {
		return 0, errors.New("must be positive")
	}
	return n, err
}
func parseNonNegativeInt(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errors.New("invalid integer")
		}
		n = n*10 + int(c-'0')
		if n > 100000000 {
			return 0, errors.New("too large")
		}
	}
	return n, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: %v", err)
	}
}
