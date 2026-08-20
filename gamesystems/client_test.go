package gamesystems

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetStatsPicksMostRecent(t *testing.T) {
	older := time.Now().Add(-time.Hour)
	newer := time.Now()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]gameSystemResponse{
			{RecordID: "1", Name: "D&D", SubmittedAt: older},
			{RecordID: "2", Name: "Pathfinder", SubmittedAt: newer},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	stats, err := client.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}
	if stats.Count != 2 {
		t.Fatalf("Count = %d, want 2", stats.Count)
	}
	if stats.MostRecentID != "2" || stats.MostRecentName != "Pathfinder" {
		t.Fatalf("MostRecent = %s/%s, want 2/Pathfinder", stats.MostRecentID, stats.MostRecentName)
	}
}

func TestGetNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Get(context.Background(), "missing")
	if _, ok := err.(NotFoundError); !ok {
		t.Fatalf("Get() error = %v, want NotFoundError", err)
	}
}
