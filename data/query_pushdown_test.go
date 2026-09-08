package data

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	apiutil "github.com/sweetrpg/api-core.go/util"
	"github.com/sweetrpg/catalog-objects.go/vo"
	"github.com/sweetrpg/common.go/logging"
	dbconstants "github.com/sweetrpg/mongodb.go/constants"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
)

// QueryPushdownTestSuite covers stage 2 of catalog-list-handlers-query-pushdown: the multi-field
// `q` search on QueryVolumes, the shared name-`contains` filter on the four engine entities, and
// the search-field indexes that back them.
type QueryPushdownTestSuite struct {
	suite.Suite
}

func (suite *QueryPushdownTestSuite) SetupTest() {
	_ = os.Setenv(dbconstants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()
	ctx := suite.T().Context()
	for _, ensure := range []func(context.Context) error{
		EnsureVolumeVersioningIndexes, EnsurePublisherVersioningIndexes,
		EnsureStudioVersioningIndexes, EnsurePersonVersioningIndexes,
		EnsureLicenseVersioningIndexes,
	} {
		assert.NoError(suite.T(), ensure(ctx))
	}
}

func regexFilter(field, value string) []apiutil.Filter {
	op := "$regex"
	return []apiutil.Filter{{Field: field, Operation: &op, Value: []string{value}}}
}

func (suite *QueryPushdownTestSuite) indexExists(collection, name string) bool {
	cur, err := database.Db.Collection(collection).Indexes().List(suite.T().Context())
	assert.NoError(suite.T(), err)
	var specs []bson.M
	assert.NoError(suite.T(), cur.All(suite.T().Context(), &specs))
	for _, s := range specs {
		if s["name"] == name {
			return true
		}
	}
	return false
}

// 2.2: a `q` matching only a volume's description (not its title) is returned.
func (suite *QueryPushdownTestSuite) TestQueryVolumesMultiFieldSearchMatchesDescriptionOnly() {
	ctx := suite.T().Context()
	hit, err := AddVolume(ctx, &vo.VolumeVO{Title: "Wholly Ordinary Title", Description: "mentions qpzz2marker in the blurb"})
	assert.NoError(suite.T(), err)
	_, err = AddVolume(ctx, &vo.VolumeVO{Title: "qpzz2marker Not In Description", Description: "clean"})
	assert.NoError(suite.T(), err)

	results, err := QueryVolumes(ctx, apiutil.QueryParams{
		Limit:  200,
		Filter: []apiutil.Filter{{Field: "q", Value: []string{"qpzz2marker"}}},
	})
	assert.NoError(suite.T(), err)

	var ids []string
	for _, r := range results {
		ids = append(ids, r.ID)
	}
	assert.Contains(suite.T(), ids, *hit, "volume whose description contains the term must match")
	// The title-only match is also returned (q ORs across title too) - the point of 2.2 is that
	// a description-only hit is not missed, which the Contains above proves.
}

// 2.3: name-`contains` filter returns only matching records - one per engine entity type.
func (suite *QueryPushdownTestSuite) TestQueryPublishersNameContains() {
	ctx := suite.T().Context()
	hit, err := AddPublisher(ctx, &vo.PublisherVO{Name: "Qpzz3 Marker Press"})
	assert.NoError(suite.T(), err)
	_, err = AddPublisher(ctx, &vo.PublisherVO{Name: "Different House"})
	assert.NoError(suite.T(), err)

	results, err := QueryPublishers(ctx, apiutil.QueryParams{Limit: 200, Filter: regexFilter("name", "qpzz3")})
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), results, 1)
	if len(results) == 1 {
		assert.Equal(suite.T(), *hit, results[0].ID)
	}
}

func (suite *QueryPushdownTestSuite) TestQueryStudiosNameContains() {
	ctx := suite.T().Context()
	hit, err := AddStudio(ctx, &vo.StudioVO{Name: "Qpzz4 Marker Studio"})
	assert.NoError(suite.T(), err)
	_, err = AddStudio(ctx, &vo.StudioVO{Name: "Unrelated Studio"})
	assert.NoError(suite.T(), err)

	results, err := QueryStudios(ctx, apiutil.QueryParams{Limit: 200, Filter: regexFilter("name", "qpzz4")})
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), results, 1)
	if len(results) == 1 {
		assert.Equal(suite.T(), *hit, results[0].ID)
	}
}

func (suite *QueryPushdownTestSuite) TestQueryPersonsNameContains() {
	ctx := suite.T().Context()
	hit, err := AddPerson(ctx, &vo.PersonVO{Name: "Qpzz5 Marker Person"})
	assert.NoError(suite.T(), err)
	_, err = AddPerson(ctx, &vo.PersonVO{Name: "Someone Else"})
	assert.NoError(suite.T(), err)

	results, err := QueryPersons(ctx, apiutil.QueryParams{Limit: 200, Filter: regexFilter("name", "qpzz5")})
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), results, 1)
	if len(results) == 1 {
		assert.Equal(suite.T(), *hit, results[0].ID)
	}
}

func (suite *QueryPushdownTestSuite) TestQueryLicensesTitleContains() {
	ctx := suite.T().Context()
	hit, err := AddLicense(ctx, &vo.LicenseVO{Title: "Qpzz6 Marker License"})
	assert.NoError(suite.T(), err)
	_, err = AddLicense(ctx, &vo.LicenseVO{Title: "Ordinary License"})
	assert.NoError(suite.T(), err)

	results, err := QueryLicenses(ctx, apiutil.QueryParams{Limit: 200, Filter: regexFilter("title", "qpzz6")})
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), results, 1)
	if len(results) == 1 {
		assert.Equal(suite.T(), *hit, results[0].ID)
	}
}

// 2.4: each search field has a backing index.
func (suite *QueryPushdownTestSuite) TestSearchIndexesExist() {
	for _, field := range []string{"title_1", "description_1", "tags.value_1"} {
		assert.True(suite.T(), suite.indexExists(volumeVersionCollection, field), "volumes: %s", field)
	}
	assert.True(suite.T(), suite.indexExists(publisherVersionCollection, "name_1"))
	assert.True(suite.T(), suite.indexExists(studioVersionCollection, "name_1"))
	assert.True(suite.T(), suite.indexExists(personVersionCollection, "name_1"))
	assert.True(suite.T(), suite.indexExists(licenseVersionCollection, "title_1"))
}

func TestQueryPushdownTestSuite(t *testing.T) {
	suite.Run(t, new(QueryPushdownTestSuite))
}
