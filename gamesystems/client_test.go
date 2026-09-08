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

// game-systems-api v0.7.0 changed GET /systems from a bare array to a
// { "systems": [...], "total": N, "page": p, "per_page": pp } envelope. The client must accept
// both so a catalog-api running against either game-systems-api version keeps working.
func TestListAcceptsEnvelopeAndBareArray(t *testing.T) {
	older := time.Now().Add(-time.Hour)
	newer := time.Now()
	rows := []gameSystemResponse{
		{RecordID: "1", Name: "D&D", SubmittedAt: older},
		{RecordID: "2", Name: "Pathfinder", SubmittedAt: newer},
	}

	cases := map[string]func(http.ResponseWriter){
		"bare array": func(w http.ResponseWriter) {
			_ = json.NewEncoder(w).Encode(rows)
		},
		"envelope": func(w http.ResponseWriter) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"systems": rows, "total": 2, "page": 1, "per_page": 24,
			})
		},
		"envelope with leading whitespace": func(w http.ResponseWriter) {
			body, _ := json.Marshal(map[string]any{"systems": rows, "total": 2})
			_, _ = w.Write(append([]byte("  \n"), body...))
		},
	}

	for name, write := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				write(w)
			}))
			defer srv.Close()

			systems, err := NewClient(srv.URL).List(context.Background())
			if err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if len(systems) != 2 || systems[0].ID != "1" || systems[1].Name != "Pathfinder" {
				t.Fatalf("List() = %+v, want the two seeded systems", systems)
			}
		})
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

func TestResponseAuditPrefersRealFieldsWithSubmittedFallback(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	submitted := time.Date(2025, 12, 12, 0, 0, 0, 0, time.UTC)

	// Post-adoption: real created_*/updated_* win.
	full := gameSystemResponse{
		CreatedAt: created, CreatedBy: "u1", UpdatedAt: updated, UpdatedBy: "u2",
		SubmittedAt: submitted, SubmittedBy: "s1",
	}
	cAt, uAt, cBy, uBy := full.audit()
	if !cAt.Equal(created) || cBy != "u1" || !uAt.Equal(updated) || uBy != "u2" {
		t.Errorf("full response: got (%v,%v,%q,%q), want real created/updated", cAt, uAt, cBy, uBy)
	}

	// Pre-adoption: no created_*/updated_*, fall back to submitted_* for both.
	legacy := gameSystemResponse{SubmittedAt: submitted, SubmittedBy: "s1"}
	cAt, uAt, cBy, uBy = legacy.audit()
	if !cAt.Equal(submitted) || cBy != "s1" || !uAt.Equal(submitted) || uBy != "s1" {
		t.Errorf("legacy response: got (%v,%v,%q,%q), want submitted_* fallback", cAt, uAt, cBy, uBy)
	}
}
