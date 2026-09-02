package data

import (
	"context"
	"strings"

	"github.com/sweetrpg/catalog-objects.go/models"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// UpdateVolumeSystemTitleBySystem sets the denormalized system title on every live volume
// version that references systemID, and returns the record IDs of the volumes it changed (for
// cache invalidation). It is a no-op for volumes whose stored title already equals title, so a
// redelivered system.updated event makes no further change. A systemID containing "." is
// skipped and logged - MongoDB forbids "." in a field name, and system_titles is keyed by ID.
func UpdateVolumeSystemTitleBySystem(c context.Context, systemID, title string) ([]string, error) {
	if strings.Contains(systemID, ".") {
		logging.Logger.Error("UpdateVolumeSystemTitleBySystem: system ID contains '.', skipping", "system_id", systemID)
		return nil, nil
	}

	coll := database.Db.Collection(volumeVersionCollection)
	field := "system_titles." + systemID

	// Match only the live versions that reference this system and don't already carry this
	// exact title - so an unchanged title (idempotent redelivery) touches nothing.
	filter := bson.D{
		{Key: "state", Value: string(models.VersionStateLive)},
		{Key: "system_ids", Value: systemID},
		{Key: field, Value: bson.D{{Key: "$ne", Value: title}}},
	}

	cur, err := coll.Find(c, filter, options.Find().SetProjection(bson.D{
		{Key: "_id", Value: 1}, {Key: "record_id", Value: 1},
	}))
	if err != nil {
		return nil, err
	}
	var matched []struct {
		ID       string `bson:"_id"`
		RecordID string `bson:"record_id"`
	}
	if err := cur.All(c, &matched); err != nil {
		return nil, err
	}
	if len(matched) == 0 {
		return nil, nil
	}

	versionIDs := make([]string, len(matched))
	recordIDs := make([]string, 0, len(matched))
	seen := make(map[string]struct{}, len(matched))
	for i, m := range matched {
		versionIDs[i] = m.ID
		if _, ok := seen[m.RecordID]; !ok {
			seen[m.RecordID] = struct{}{}
			recordIDs = append(recordIDs, m.RecordID)
		}
	}

	_, err = coll.UpdateMany(c,
		bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: versionIDs}}}},
		bson.D{{Key: "$set", Value: bson.D{{Key: field, Value: title}}}},
	)
	if err != nil {
		return nil, err
	}

	logging.Logger.Info("UpdateVolumeSystemTitleBySystem applied", "system_id", systemID, "volumes", len(recordIDs))
	return recordIDs, nil
}
