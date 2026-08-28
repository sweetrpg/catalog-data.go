package data

import (
	"context"
	"strings"

	"github.com/sweetrpg/catalog-data.go/gamesystems"
	"github.com/sweetrpg/catalog-objects.go/vo"
	modelcorevo "github.com/sweetrpg/model-core.go/vo"
)

// GameSystemsClient resolves system references against game-systems-api, the system of record
// for game systems (see platform's game-systems-catalog spec), instead of catalog-api storing
// its own copy. Set once at startup (see catalog-api's cmd/catalog-api/main.go); nil until then,
// in which case GetSystem/GetSystemStats degrade to "no data" rather than panicking - matching
// how resolveVolumeRelations already treats a missing/erroring relation as skip-and-log.
var GameSystemsClient *gamesystems.Client

// GetSystem resolves a game system by id against game-systems-api's current (live) version.
func GetSystem(c context.Context, id string) (*vo.SystemVO, error) {
	if GameSystemsClient == nil {
		return nil, nil
	}
	system, err := GameSystemsClient.Get(c, id)
	if err != nil {
		if _, ok := err.(gamesystems.NotFoundError); ok {
			return nil, nil
		}
		return nil, err
	}
	return &vo.SystemVO{
		ID: system.ID, GameSystem: system.Name, Edition: system.Edition, Notes: system.Notes,
		Tags: system.Tags,
		AuditableVO: modelcorevo.AuditableVO{
			CreatedAt: system.CreatedAt, CreatedBy: system.CreatedBy,
			UpdatedAt: system.CreatedAt, UpdatedBy: system.CreatedBy,
		},
	}, nil
}

// QuerySystems lists every live game system against game-systems-api's current versions. Backs
// catalog-api's /systems list route - a nil GameSystemsClient (unconfigured GAMESYSTEMS_API_URL)
// degrades to an empty list rather than an error, matching GetSystem's fail-open convention.
func QuerySystems(c context.Context) ([]*vo.SystemVO, error) {
	if GameSystemsClient == nil {
		return []*vo.SystemVO{}, nil
	}
	systems, err := GameSystemsClient.List(c)
	if err != nil {
		return nil, err
	}
	vos := make([]*vo.SystemVO, len(systems))
	for i, system := range systems {
		vos[i] = &vo.SystemVO{
			ID: system.ID, GameSystem: system.Name, Edition: system.Edition, Notes: system.Notes,
			Tags: system.Tags,
			AuditableVO: modelcorevo.AuditableVO{
				CreatedAt: system.CreatedAt, CreatedBy: system.CreatedBy,
				UpdatedAt: system.CreatedAt, UpdatedBy: system.CreatedBy,
			},
		}
	}
	return vos, nil
}

// SearchSystems finds live game systems whose name contains query (case-insensitive) - same
// scan-in-memory approach as data.SearchPersons, over game-systems-api's full live list rather
// than a Mongo collection (there is no local collection for Systems to query).
func SearchSystems(c context.Context, query string) ([]*vo.SystemVO, error) {
	all, err := QuerySystems(c)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(query)
	matches := make([]*vo.SystemVO, 0, len(all))
	for _, s := range all {
		if strings.Contains(strings.ToLower(s.GameSystem), needle) {
			matches = append(matches, s)
		}
	}
	return matches, nil
}

// GetSystemsMap resolves every live game system in one call, keyed by ID - lets a caller
// resolving many volumes' system references (e.g. QueryVolumes) do it with a single
// game-systems-api round trip instead of one per volume. Returns an empty (non-nil) map, not an
// error, when the client isn't configured, matching GetSystem's fail-open behavior.
func GetSystemsMap(c context.Context) (map[string]*vo.SystemVO, error) {
	if GameSystemsClient == nil {
		return map[string]*vo.SystemVO{}, nil
	}
	systems, err := GameSystemsClient.List(c)
	if err != nil {
		return nil, err
	}
	m := make(map[string]*vo.SystemVO, len(systems))
	for _, system := range systems {
		m[system.ID] = &vo.SystemVO{
			ID: system.ID, GameSystem: system.Name, Edition: system.Edition, Notes: system.Notes,
			Tags: system.Tags,
			AuditableVO: modelcorevo.AuditableVO{
				CreatedAt: system.CreatedAt, CreatedBy: system.CreatedBy,
				UpdatedAt: system.CreatedAt, UpdatedBy: system.CreatedBy,
			},
		}
	}
	return m, nil
}

// GetSystemStats returns the systems landing-page-summary card, computed by game-systems-api.
func GetSystemStats(c context.Context) (*TypeStats, error) {
	if GameSystemsClient == nil {
		return &TypeStats{}, nil
	}
	stats, err := GameSystemsClient.GetStats(c)
	if err != nil {
		return nil, err
	}
	return &TypeStats{
		Count: stats.Count, LastUpdated: stats.LastUpdated,
		MostRecentID: stats.MostRecentID, MostRecentName: stats.MostRecentName,
	}, nil
}
