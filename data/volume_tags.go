package data

import (
	"context"

	"github.com/sweetrpg/catalog-objects.go/models"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// GetVolumeTags returns the distinct tag names attached to live volumes, ranked by how many
// volumes carry each tag (most-used first). It powers the catalog-web landing-page tag cloud,
// which drops the low-frequency tail - so limit caps the result, and an empty catalog returns an
// empty slice. A limit of 0 or negative means "use the default cloud size".
func GetVolumeTags(c context.Context, limit int) ([]string, error) {
	logging.Logger.Debug("GetVolumeTags", "c", c, "limit", limit)
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	coll := database.Db.Collection(volumeVersionCollection)
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{{Key: "state", Value: string(models.VersionStateLive)}}}},
		bson.D{{Key: "$unwind", Value: "$tags"}},
		bson.D{{Key: "$match", Value: bson.D{{Key: "tags.name", Value: bson.D{{Key: "$ne", Value: ""}}}}}},
		bson.D{{Key: "$sortByCount", Value: "$tags.name"}},
		bson.D{{Key: "$limit", Value: limit}},
	}

	cur, err := coll.Aggregate(c, pipeline)
	if err != nil {
		return nil, err
	}
	var ranked []struct {
		Name string `bson:"_id"`
	}
	if err := cur.All(c, &ranked); err != nil {
		return nil, err
	}

	tags := make([]string, 0, len(ranked))
	for _, r := range ranked {
		tags = append(tags, r.Name)
	}
	logging.Logger.Debug("GetVolumeTags returned", "tags", tags)
	return tags, nil
}
