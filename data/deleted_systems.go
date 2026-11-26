package data

import (
	"context"
	"errors"
	"time"

	modelcore "github.com/sweetrpg/model-core.go/models"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const deletedSystemCollection = "deleted_systems"

// DeletedSystem is catalog's permanent tombstone for a game system deleted in game-systems-api
// (PADR-0047). While it exists, no new or newly current volume version may reference either of
// its identifiers. Game systems have no restore, so tombstones are never cleared.
type DeletedSystem struct {
	ID              string    `bson:"_id"`
	SystemID        string    `bson:"system_id"`
	SourceDeletedAt time.Time `bson:"source_deleted_at"`

	modelcore.Auditable `bson:",inline"`
}

// Refs returns the identifiers a volume may have stored for this system.
func (d DeletedSystem) Refs() []string {
	if d.SystemID == "" {
		return []string{d.ID}
	}
	return []string{d.ID, d.SystemID}
}

// EnsureDeletedSystemIndexes creates the tombstone lookup index and the volume index removal
// uses. Safe to call on every startup.
func EnsureDeletedSystemIndexes(c context.Context) error {
	if _, err := database.Db.Collection(deletedSystemCollection).Indexes().CreateOne(c, indexOn("system_id")); err != nil {
		return err
	}
	_, err := database.Db.Collection(volumeVersionCollection).Indexes().CreateOne(c, indexOn("system_ids"))
	return err
}

// RecordDeletedSystem stores the tombstone for system. Recording the same system again changes
// nothing. Call it before RemoveDeletedSystemReferences.
func RecordDeletedSystem(c context.Context, system DeletedSystem, actingUserID string) error {
	if system.ID == "" {
		return errors.New("deleted system: id is required")
	}
	modelcore.StampCreate(&system.Auditable, actingUserID, time.Now())
	_, err := database.Db.Collection(deletedSystemCollection).UpdateOne(c,
		bson.D{{Key: "_id", Value: system.ID}},
		bson.D{{Key: "$setOnInsert", Value: system}},
		options.Update().SetUpsert(true))
	return err
}

// ListDeletedSystems returns every tombstone, for reapplying cleanup during reconciliation.
func ListDeletedSystems(c context.Context) ([]DeletedSystem, error) {
	cur, err := database.Db.Collection(deletedSystemCollection).Find(c, bson.D{})
	if err != nil {
		return nil, err
	}
	systems := []DeletedSystem{}
	if err := cur.All(c, &systems); err != nil {
		return nil, err
	}
	return systems, nil
}

// deletedAmong returns the members of refs that name a tombstoned system.
func deletedAmong(c context.Context, refs []string) (map[string]bool, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	in := bson.D{{Key: "$in", Value: refs}}
	cur, err := database.Db.Collection(deletedSystemCollection).Find(c, bson.D{{Key: "$or", Value: bson.A{
		bson.D{{Key: "_id", Value: in}},
		bson.D{{Key: "system_id", Value: in}},
	}}})
	if err != nil {
		return nil, err
	}
	var found []DeletedSystem
	if err := cur.All(c, &found); err != nil {
		return nil, err
	}
	dead := make(map[string]bool, 2*len(found))
	for _, system := range found {
		for _, ref := range system.Refs() {
			dead[ref] = true
		}
	}
	return dead, nil
}
