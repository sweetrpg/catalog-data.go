package data

import (
	"context"
	"fmt"
	"time"

	"github.com/sweetrpg/catalog-objects.go/models"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// Soft deletion lives on a record's meta document (deleted_at), while list, count, and stats
// read the version collection. These helpers carry the deletion into the version-collection
// filter so skip/limit and CountDocuments run after it, in the database.

// metaRef decodes just the _id of a meta document.
type metaRef struct {
	ID string `bson:"_id"`
}

// excludeSoftDeleted adds a `record_id $nin <soft-deleted ids>` clause to a version-collection
// filter. A restored record has deleted_at null, so it is not in the list.
// ponytail: the list grows with the number of soft-deleted records of one type, which stays small
// in a curated catalog; switch to a $lookup pipeline if deletions ever run to the tens of thousands.
func excludeSoftDeleted(c context.Context, metaCollection string, filter bson.D) (bson.D, error) {
	deleted, err := database.Query[metaRef](metaCollection,
		bson.D{{Key: "deleted_at", Value: bson.D{{Key: "$ne", Value: nil}}}},
		nil, bson.D{{Key: "_id", Value: 1}}, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("list soft-deleted records in %s: %w", metaCollection, err)
	}
	if len(deleted) == 0 {
		return filter, nil
	}
	ids := make(bson.A, len(deleted))
	for i, d := range deleted {
		ids[i] = d.ID
	}
	return append(filter, bson.E{Key: "record_id", Value: bson.D{{Key: "$nin", Value: ids}}}), nil
}

// liveVersionFilter matches live versions of records that are not soft-deleted.
func liveVersionFilter(c context.Context, metaCollection string) (bson.D, error) {
	return excludeSoftDeleted(c, metaCollection, bson.D{{Key: "state", Value: string(models.VersionStateLive)}})
}

// newestRecordStats builds a TypeStats for one entity type: count of live, non-deleted records,
// plus the most recently created one (meta created_at) that has a live version. A newer record
// with no live version (pending review, rejected) is skipped. The join, sort and limit 1 run in
// Mongo. describe returns a live version's submitted-at time, record ID and display name.
func newestRecordStats[T any](c context.Context, metaCollection, versionCollection, typeName string,
	describe func(*T) (submittedAt time.Time, recordID, name string)) (*TypeStats, error) {

	filter, err := liveVersionFilter(c, metaCollection)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", typeName, err)
	}
	count, err := database.Db.Collection(versionCollection).CountDocuments(c, filter)
	if err != nil {
		return nil, fmt.Errorf("%s: count live versions: %w", typeName, err)
	}
	stats := &TypeStats{Count: int(count)}
	if stats.Count == 0 {
		return stats, nil
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: metaCollection}, {Key: "localField", Value: "record_id"},
			{Key: "foreignField", Value: "_id"}, {Key: "as", Value: "meta"},
		}}},
		{{Key: "$unwind", Value: "$meta"}},
		{{Key: "$sort", Value: bson.D{{Key: "meta.created_at", Value: -1}}}},
		{{Key: "$limit", Value: 1}},
		{{Key: "$project", Value: bson.D{{Key: "meta", Value: 0}}}},
	}
	cur, err := database.Db.Collection(versionCollection).Aggregate(c, pipeline)
	if err != nil {
		return nil, fmt.Errorf("%s: query newest live record: %w", typeName, err)
	}
	var newest []T
	if err := cur.All(c, &newest); err != nil {
		return nil, fmt.Errorf("%s: decode newest live record: %w", typeName, err)
	}
	if len(newest) == 0 {
		return stats, nil
	}
	submittedAt, recordID, name := describe(&newest[0])
	stats.LastUpdated = &submittedAt
	stats.MostRecentID = recordID
	stats.MostRecentName = name
	return stats, nil
}
