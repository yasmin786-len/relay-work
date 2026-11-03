package main

import "encoding/json"

// Event is one provider webhook event. Timestamp is retained as the original
// RFC3339 string so the provider's event time is not confused with receive time.
type Event struct {
	EventID    string          `json:"event_id"`
	CampaignID string          `json:"campaign_id"`
	ContactID  string          `json:"contact_id"`
	Type       string          `json:"type"`
	Timestamp  string          `json:"timestamp"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
}
