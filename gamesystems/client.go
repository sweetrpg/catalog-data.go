// Package game-systems is a minimal client for game-systems-api's read endpoints, used by the
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

// Client calls game-systems-api's read endpoints.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient builds a Client against game-systems-api's base URL. An empty baseURL is accepted so
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

// NotFoundError means game-systems-api has no game system at the given id.
type NotFoundError struct{ ID string }

func (e NotFoundError) Error() string { return fmt.Sprintf("gamesystems: %s not found", e.ID) }

// gameSystemResponse is game-systems-api's flattened GET /systems(/:id) response shape (its
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
	// Platform audit block from game-systems-api's stable record (EntityMeta), added when that
	// service adopted the audit-fields convention. Absent on a pre-adoption response, in which
	// case these decode to zero and the caller falls back to submitted_* below.
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedBy string    `json:"updated_by"`
	UpdatedAt time.Time `json:"updated_at"`
}

// audit resolves the response's audit fields, preferring the real created_*/updated_* and
// falling back to the version's submitted_* for a pre-adoption game-systems-api.
func (r gameSystemResponse) audit() (createdAt, updatedAt time.Time, createdBy, updatedBy string) {
	createdAt, createdBy = r.CreatedAt, r.CreatedBy
	if createdAt.IsZero() {
		createdAt, createdBy = r.SubmittedAt, r.SubmittedBy
	}
	updatedAt, updatedBy = r.UpdatedAt, r.UpdatedBy
	if updatedAt.IsZero() {
		updatedAt, updatedBy = createdAt, createdBy
	}
	return
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
	UpdatedBy string
	UpdatedAt time.Time
}

// Get resolves one game system by id against its current (live) version.
func (c *Client) Get(ctx context.Context, id string) (*System, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/systems/"+id, nil)
	if err != nil {
		return nil, fmt.Errorf("gamesystems: build get request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("game-systems: get request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, NotFoundError{ID: id}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("game-systems: unexpected status %d from get %s", resp.StatusCode, id)
	}

	var body gameSystemResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("game-systems: decode get response: %w", err)
	}

	createdAt, updatedAt, createdBy, updatedBy := body.audit()
	return &System{
		ID: body.RecordID, Name: body.Name, Edition: body.Edition, Notes: body.Notes,
		Tags:      toTagVOs(body.Tags),
		CreatedBy: createdBy, CreatedAt: createdAt, UpdatedBy: updatedBy, UpdatedAt: updatedAt,
	}, nil
}

// List resolves every live game system against game-systems-api's current versions - backs
// catalog-api's /systems list route (the autocomplete/picker source for a volume's system
// associations) as well as GetStats below.
func (c *Client) List(ctx context.Context) ([]*System, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/systems", nil)
	if err != nil {
		return nil, fmt.Errorf("game-systems: build list request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("game-systems: list request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("game-systems: unexpected status %d from list", resp.StatusCode)
	}

	var all []gameSystemResponse
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		return nil, fmt.Errorf("game-systems: decode list response: %w", err)
	}

	systems := make([]*System, len(all))
	for i, gs := range all {
		createdAt, updatedAt, createdBy, updatedBy := gs.audit()
		systems[i] = &System{
			ID: gs.RecordID, Name: gs.Name, Edition: gs.Edition, Notes: gs.Notes,
			Tags:      toTagVOs(gs.Tags),
			CreatedBy: createdBy, CreatedAt: createdAt, UpdatedBy: updatedBy, UpdatedAt: updatedAt,
		}
	}
	return systems, nil
}

// Stats is the catalog-landing-page-summary card game-systems-api backs: a live-record count
// plus the single most recently submitted live record.
type Stats struct {
	Count          int
	LastUpdated    *time.Time
	MostRecentID   string
	MostRecentName string
}

// GetStats computes Stats from List - no dedicated stats endpoint on game-systems-api yet, and
// system counts are small enough that computing this client-side is simpler than adding one.
// Revisit if that stops being true.
func (c *Client) GetStats(ctx context.Context) (*Stats, error) {
	systems, err := c.List(ctx)
	if err != nil {
		return nil, err
	}

	stats := &Stats{Count: len(systems)}
	for _, s := range systems {
		if stats.LastUpdated == nil || s.UpdatedAt.After(*stats.LastUpdated) {
			updatedAt := s.UpdatedAt
			stats.LastUpdated = &updatedAt
			stats.MostRecentID = s.ID
			stats.MostRecentName = s.Name
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
