package data

import (
	"context"

	"github.com/sweetrpg/catalog-data.go/gamesystems"
	"github.com/sweetrpg/catalog-objects.go/vo"
	modelcorevo "github.com/sweetrpg/model-core.go/vo"
)

// GameSystemsClient resolves system references against gamesystems-api, the system of record
// for game systems (see platform's game-systems-catalog spec), instead of catalog-api storing
// its own copy. Set once at startup (see catalog-api's cmd/catalog-api/main.go); nil until then,
// in which case GetSystem/GetSystemStats degrade to "no data" rather than panicking - matching
// how resolveVolumeRelations already treats a missing/erroring relation as skip-and-log.
var GameSystemsClient *gamesystems.Client

// GetSystem resolves a game system by id against gamesystems-api's current (live) version.
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

// GetSystemStats returns the systems landing-page-summary card, computed by gamesystems-api.
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
