// Package gamesystems is a minimal client for gamesystems-api's read endpoints, used by the
// data package to resolve a Volume's system references at read time instead of storing its own
// copy of system data. See platform's game-systems-catalog spec
// (openspec/changes/game-systems-service in sweetrpg/platform).
package gamesystems

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	modelcore "github.com/sweetrpg/model-core.go/vo"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Client calls gamesystems-api's read endpoints.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient builds a Client against gamesystems-api's base URL. An empty baseURL is accepted so
// the service can still start when GAMESYSTEMS_API_URL isn't configured; every call will then
// fail with a transport error, which callers already skip-and-log.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout:   5 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}
}

// NotFoundError means gamesystems-api has no game system at the given id.
type NotFoundError struct{ ID string }

func (e NotFoundError) Error() string { return fmt.Sprintf("gamesystems: %s not found", e.ID) }

// gameSystemResponse is gamesystems-api's flattened GET /systems(/:id) response shape (its
// models.GameSystemVersion, JSON-tagged) - only the fields this client needs are declared here.
type gameSystemResponse struct {
	RecordID string `json:"record_id"`
	Name     string `json:"name"`
	Edition  string `json:"edition"`
	Notes    string `json:"notes"`
	Tags     []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"tags"`
	SubmittedBy string    `json:"submitted_by"`
	SubmittedAt time.Time `json:"submitted_at"`
}

// System is the resolved shape a caller needs to fill in a volume's system reference.
type System struct {
	ID        string
	Name      string
	Edition   string
	Notes     string
	Tags      []modelcore.TagVO
	CreatedBy string
	CreatedAt time.Time
}

// Get resolves one game system by id against its current (live) version.
func (c *Client) Get(ctx context.Context, id string) (*System, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/systems/"+id, nil)
	if err != nil {
		return nil, fmt.Errorf("gamesystems: build get request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gamesystems: get request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, NotFoundError{ID: id}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gamesystems: unexpected status %d from get %s", resp.StatusCode, id)
	}

	var body gameSystemResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("gamesystems: decode get response: %w", err)
	}

	return &System{
		ID: body.RecordID, Name: body.Name, Edition: body.Edition, Notes: body.Notes,
		Tags: toTagVOs(body.Tags), CreatedBy: body.SubmittedBy, CreatedAt: body.SubmittedAt,
	}, nil
}

// Stats is the catalog-landing-page-summary card gamesystems-api backs: a live-record count
// plus the single most recently submitted live record.
type Stats struct {
	Count          int
	LastUpdated    *time.Time
	MostRecentID   string
	MostRecentName string
}

// GetStats computes Stats from GET /systems (every live game system's current version) - no
// dedicated stats endpoint on gamesystems-api yet, and system counts are small enough that
// computing this client-side is simpler than adding one. Revisit if that stops being true.
func (c *Client) GetStats(ctx context.Context) (*Stats, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/systems", nil)
	if err != nil {
		return nil, fmt.Errorf("gamesystems: build list request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gamesystems: list request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gamesystems: unexpected status %d from list", resp.StatusCode)
	}

	var all []gameSystemResponse
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		return nil, fmt.Errorf("gamesystems: decode list response: %w", err)
	}

	stats := &Stats{Count: len(all)}
	for _, gs := range all {
		if stats.LastUpdated == nil || gs.SubmittedAt.After(*stats.LastUpdated) {
			submittedAt := gs.SubmittedAt
			stats.LastUpdated = &submittedAt
			stats.MostRecentID = gs.RecordID
			stats.MostRecentName = gs.Name
		}
	}
	return stats, nil
}

func toTagVOs(tags []struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}) []modelcore.TagVO {
	out := make([]modelcore.TagVO, 0, len(tags))
	for _, t := range tags {
		out = append(out, modelcore.TagVO{Name: t.Name, Value: t.Value})
	}
	return out
}
