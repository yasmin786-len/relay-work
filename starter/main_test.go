package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func resetStore() { store.mu.Lock(); store.events = make(map[string]Event); store.mu.Unlock() }

func post(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handlePostEvents(rec, req)
	return rec
}

func TestBatchPartialAcceptanceAndIdempotency(t *testing.T) {
	resetStore()
	body := `[{"event_id":"e1","campaign_id":"c1","contact_id":"u1","type":"opened","timestamp":"2026-08-10T09:00:00Z"},{"event_id":"e1","campaign_id":"c1","contact_id":"u1","type":"opened","timestamp":"2026-08-10T09:00:00Z"},{"event_id":"bad","campaign_id":"c1","contact_id":"u2","type":"bounced","timestamp":"2026-08-10T09:00:00Z"}]`
	rec := post(t, body)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["accepted"] != float64(1) || got["duplicates"] != float64(1) || got["rejected"] != float64(1) {
		t.Fatalf("unexpected response: %v", got)
	}
}

func TestStatsCountEventsAndUniqueContacts(t *testing.T) {
	resetStore()
	post(t, `[{"event_id":"1","campaign_id":"c","contact_id":"u","type":"opened","timestamp":"2026-08-10T09:00:00Z"},{"event_id":"2","campaign_id":"c","contact_id":"u","type":"opened","timestamp":"2026-08-10T09:01:00Z"},{"event_id":"3","campaign_id":"c","contact_id":"u","type":"delivered","timestamp":"2026-08-10T09:02:00Z"}]`)
	req := httptest.NewRequest("GET", "/campaigns/c/stats", nil)
	req.SetPathValue("campaignID", "c")
	rec := httptest.NewRecorder()
	handleGetStats(rec, req)
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["opened"] != float64(2) || got["unique_opens"] != float64(1) || got["delivered"] != float64(1) {
		t.Fatalf("unexpected stats: %v", got)
	}
}

func TestConcurrentDuplicateRequests(t *testing.T) {
	resetStore()
	body := `[{"event_id":"same","campaign_id":"c","contact_id":"u","type":"sent","timestamp":"2026-08-10T09:00:00Z"}]`
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("POST", "/events", bytes.NewBufferString(body))
			rec := httptest.NewRecorder()
			handlePostEvents(rec, req)
			if rec.Code != 200 {
				t.Errorf("status=%d", rec.Code)
			}
		}()
	}
	wg.Wait()
	store.mu.RLock()
	defer store.mu.RUnlock()
	if len(store.events) != 1 {
		t.Fatalf("expected one stored event, got %d", len(store.events))
	}
}
