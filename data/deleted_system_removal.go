package data

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/sweetrpg/catalog-objects.go/models"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// replaceAttempts bounds retries when concurrent edits keep moving a volume's current version.
// A caller that gets the error redelivers or reconciles later.
const replaceAttempts = 5

// ChangedVolume is a volume that cleanup gave a new current version.
type ChangedVolume struct {
	RecordID string
	Title    string
	Version  int
}

// SystemRemoval reports what one cleanup run changed. Changed volumes need a change event and
// cache invalidation; PendingRecordIDs had only pending submissions edited.
type SystemRemoval struct {
	Changed          []ChangedVolume
	PendingRecordIDs []string
}

// RemoveDeletedSystemReferences removes system's references from every volume's current version
// (including soft-deleted volumes) and from pending submissions. Other history is left as it
// was. It is convergent: a run interrupted at any point is completed by the next, and a run with
// nothing left to do writes nothing. system must already be recorded with RecordDeletedSystem.
// On error the result holds what was changed before it.
func RemoveDeletedSystemReferences(c context.Context, system DeletedSystem, actingUserID string) (SystemRemoval, error) {
	var result SystemRemoval
	records, err := recordsToClean(c, system.Refs())
	if err != nil {
		return result, err
	}
	for _, recordID := range records {
		changed, err := cleanCurrentVersion(c, recordID, system.Refs(), actingUserID)
		if err != nil {
			return result, fmt.Errorf("volume %s: %w", recordID, err)
		}
		if changed != nil {
			result.Changed = append(result.Changed, *changed)
		}
	}
	result.PendingRecordIDs, err = removeFromPending(c, system.Refs())
	return result, err
}

// recordsToClean finds volumes where a version referencing refs is current or still marked
// live. The second case is what an interrupted run or edit leaves behind.
func recordsToClean(c context.Context, refs []string) ([]string, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{
			{Key: "system_ids", Value: bson.D{{Key: "$in", Value: refs}}},
			{Key: "state", Value: bson.D{{Key: "$ne", Value: string(models.VersionStateSubmitted)}}},
		}}},
		{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: volumeMetaCollection}, {Key: "localField", Value: "record_id"},
			{Key: "foreignField", Value: "_id"}, {Key: "as", Value: "meta"},
		}}},
		{{Key: "$unwind", Value: "$meta"}},
		{{Key: "$match", Value: bson.D{{Key: "$or", Value: bson.A{
			bson.D{{Key: "state", Value: string(models.VersionStateLive)}},
			bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$version", "$meta.current_version"}}}}},
		}}}}},
		{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$record_id"}}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	}
	cur, err := database.Db.Collection(volumeVersionCollection).Aggregate(c, pipeline)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID string `bson:"_id"`
	}
	if err := cur.All(c, &rows); err != nil {
		return nil, err
	}
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	return ids, nil
}

// cleanCurrentVersion replaces recordID's current version if it references a tombstoned system,
// then archives stale live versions. It returns nil when the current version was already clean.
func cleanCurrentVersion(c context.Context, recordID string, refs []string, actingUserID string) (*ChangedVolume, error) {
	for range replaceAttempts {
		meta, err := getVolumeMeta(c, recordID)
		if err != nil || meta == nil {
			return nil, err
		}
		current, err := getVolumeVersion(c, recordID, meta.CurrentVersion)
		if err != nil || current == nil {
			return nil, err
		}
		replacement := *current
		dirty, err := filterDeletedSystems(c, &replacement)
		if err != nil {
			return nil, err
		}
		if !dirty {
			return finishReplacement(c, current, refs)
		}
		done, err := replaceCurrentVersion(c, &replacement, meta.CurrentVersion, actingUserID)
		if err != nil {
			return nil, err
		}
		if done {
			changed := &ChangedVolume{RecordID: recordID, Title: replacement.Title, Version: replacement.Version}
			return changed, archiveStaleVersions(c, recordID, replacement.Version, refs)
		}
	}
	return nil, fmt.Errorf("current version kept changing after %d attempts", replaceAttempts)
}

// finishReplacement completes a replacement that stopped after moving the pointer. It returns
// the volume when its current version still had to be marked live, so it gets announced.
func finishReplacement(c context.Context, current *models.VolumeVersion, refs []string) (*ChangedVolume, error) {
	var repaired *ChangedVolume
	if current.State == models.VersionStateArchived {
		if err := markVolumeVersionLive(c, current.RecordID, current.Version); err != nil {
			return nil, err
		}
		repaired = &ChangedVolume{RecordID: current.RecordID, Title: current.Title, Version: current.Version}
	}
	return repaired, archiveStaleVersions(c, current.RecordID, current.Version, refs)
}

func markVolumeVersionLive(c context.Context, recordID string, version int) error {
	return setVolumeVersionState(c, recordID, version, bson.D{{Key: "state", Value: string(models.VersionStateLive)}})
}

// replaceCurrentVersion makes next (a filtered copy of version oldVersion) current, only if
// oldVersion still is. It returns false, leaving nothing behind, when a concurrent write got
// there first.
//
// next is inserted archived and marked live only once it is current, so a run that dies midway
// never leaves an extra live version, and concurrent runs never have to delete each other's
// copies. The old version stays live until last, which is how the next run finds the volume.
func replaceCurrentVersion(c context.Context, next *models.VolumeVersion, oldVersion int, actingUserID string) (bool, error) {
	versions := database.Db.Collection(volumeVersionCollection)
	number, err := nextVolumeVersionNumber(c, next.RecordID)
	if err != nil {
		return false, err
	}
	asDerivedVersion(next, number, oldVersion, actingUserID)
	if _, err := versions.InsertOne(c, next); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return false, nil
		}
		return false, err
	}
	moved, err := database.Db.Collection(volumeMetaCollection).UpdateOne(c,
		bson.D{{Key: "_id", Value: next.RecordID}, {Key: "current_version", Value: oldVersion}},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "current_version", Value: number},
			{Key: "updated_at", Value: time.Now()}, {Key: "updated_by", Value: actingUserID},
		}}})
	if err != nil {
		return false, err
	}
	if moved.MatchedCount == 0 {
		_, err := versions.DeleteOne(c, bson.D{{Key: "_id", Value: next.ID}})
		return false, err
	}
	if err := markVolumeVersionLive(c, next.RecordID, number); err != nil {
		return false, err
	}
	next.State = models.VersionStateLive
	return true, archiveVolumeVersion(c, next.RecordID, oldVersion)
}

// asDerivedVersion turns a copy of an existing version into a new, not yet current one.
func asDerivedVersion(v *models.VolumeVersion, number, baseVersion int, actingUserID string) {
	v.ID = primitive.NewObjectID().Hex()
	v.Version = number
	v.State = models.VersionStateArchived
	v.BaseVersion = &baseVersion
	v.SubmittedBy = actingUserID
	v.SubmittedAt = time.Now()
	v.ReviewedBy, v.ReviewedAt, v.ReviewNote, v.ResultingVersion = nil, nil, nil, nil
	v.StagedCoverAssetId, v.StagedSampleAssetIds = nil, nil
}

// archiveStaleVersions archives older live versions that still reference refs. Newer ones are
// left alone: a concurrent edit may be about to make one current.
func archiveStaleVersions(c context.Context, recordID string, current int, refs []string) error {
	_, err := database.Db.Collection(volumeVersionCollection).UpdateMany(c,
		bson.D{
			{Key: "record_id", Value: recordID},
			{Key: "state", Value: string(models.VersionStateLive)},
			{Key: "version", Value: bson.D{{Key: "$lt", Value: current}}},
			{Key: "system_ids", Value: bson.D{{Key: "$in", Value: refs}}},
		},
		bson.D{{Key: "$set", Value: bson.D{{Key: "state", Value: string(models.VersionStateArchived)}}}})
	return err
}

// removeFromPending edits pending submissions in place and returns the volumes it touched.
func removeFromPending(c context.Context, refs []string) ([]string, error) {
	versions := database.Db.Collection(volumeVersionCollection)
	in := bson.D{{Key: "$in", Value: refs}}
	filter := bson.D{
		{Key: "state", Value: string(models.VersionStateSubmitted)},
		{Key: "system_ids", Value: in},
	}
	found, err := versions.Distinct(c, "record_id", filter)
	if err != nil || len(found) == 0 {
		return nil, err
	}
	update := bson.D{{Key: "$pull", Value: bson.D{{Key: "system_ids", Value: in}}}}
	var titles bson.D
	for _, ref := range refs {
		if isFieldNameSafe(ref) {
			titles = append(titles, bson.E{Key: "system_titles." + ref, Value: ""})
		}
	}
	if len(titles) > 0 {
		update = append(update, bson.E{Key: "$unset", Value: titles})
	}
	if _, err := versions.UpdateMany(c, filter, update); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(found))
	for _, value := range found {
		if id, ok := value.(string); ok {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	logging.Logger.Info("removed deleted system from pending submissions", "refs", refs, "volumes", len(ids))
	return ids, nil
}

// isFieldNameSafe reports whether ref can be part of a MongoDB field path.
func isFieldNameSafe(ref string) bool {
	return ref != "" && !strings.Contains(ref, ".") && !strings.HasPrefix(ref, "$")
}
