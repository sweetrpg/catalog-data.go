package data

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

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

// 3b.1: Count<Entity> equals the length of an unpaginated Query<Entity> for the same filter,
// both unfiltered and filtered.
func (suite *QueryPushdownTestSuite) TestCountMatchesUnpaginatedQueryLength() {
	ctx := suite.T().Context()
	const big = 100000

	assertCount := func(name string, count int64, err error, queryLen int) {
		assert.NoError(suite.T(), err, name)
		assert.EqualValues(suite.T(), queryLen, count, "%s: count must equal unpaginated Query length", name)
	}

	// Volumes: two carry the marker in the description, one does not.
	_, err := AddVolume(ctx, &vo.VolumeVO{Title: "V A", Description: "has cntmk1 marker"})
	assert.NoError(suite.T(), err)
	_, err = AddVolume(ctx, &vo.VolumeVO{Title: "cntmk1 in title too", Description: "x"})
	assert.NoError(suite.T(), err)
	_, err = AddVolume(ctx, &vo.VolumeVO{Title: "V C", Description: "unrelated"})
	assert.NoError(suite.T(), err)

	allVols, err := QueryVolumes(ctx, apiutil.QueryParams{Limit: big})
	assert.NoError(suite.T(), err)
	cVol, err := CountVolumes(ctx, apiutil.QueryParams{Limit: big})
	assertCount("volumes unfiltered", cVol, err, len(allVols))

	qVol := apiutil.QueryParams{Limit: big, Filter: []apiutil.Filter{{Field: "q", Value: []string{"cntmk1"}}}}
	fVols, err := QueryVolumes(ctx, qVol)
	assert.NoError(suite.T(), err)
	cVolF, err := CountVolumes(ctx, qVol)
	assertCount("volumes q=cntmk1", cVolF, err, len(fVols))

	// Engine entities.
	seedPub := func(n string) { _, e := AddPublisher(ctx, &vo.PublisherVO{Name: n}); assert.NoError(suite.T(), e) }
	seedPub("Cntmk2 One")
	seedPub("Cntmk2 Two")
	seedPub("Elsewhere")
	allPub, err := QueryPublishers(ctx, apiutil.QueryParams{Limit: big})
	assert.NoError(suite.T(), err)
	cPub, err := CountPublishers(ctx, apiutil.QueryParams{Limit: big})
	assertCount("publishers unfiltered", cPub, err, len(allPub))
	fPub, err := QueryPublishers(ctx, apiutil.QueryParams{Limit: big, Filter: regexFilter("name", "cntmk2")})
	assert.NoError(suite.T(), err)
	cPubF, err := CountPublishers(ctx, apiutil.QueryParams{Limit: big, Filter: regexFilter("name", "cntmk2")})
	assertCount("publishers name~cntmk2", cPubF, err, len(fPub))

	seedStu := func(n string) { _, e := AddStudio(ctx, &vo.StudioVO{Name: n}); assert.NoError(suite.T(), e) }
	seedStu("Cntmk3 One")
	seedStu("Other Studio")
	allStu, err := QueryStudios(ctx, apiutil.QueryParams{Limit: big})
	assert.NoError(suite.T(), err)
	cStu, err := CountStudios(ctx, apiutil.QueryParams{Limit: big})
	assertCount("studios unfiltered", cStu, err, len(allStu))
	fStu, err := QueryStudios(ctx, apiutil.QueryParams{Limit: big, Filter: regexFilter("name", "cntmk3")})
	assert.NoError(suite.T(), err)
	cStuF, err := CountStudios(ctx, apiutil.QueryParams{Limit: big, Filter: regexFilter("name", "cntmk3")})
	assertCount("studios name~cntmk3", cStuF, err, len(fStu))

	seedPer := func(n string) { _, e := AddPerson(ctx, &vo.PersonVO{Name: n}); assert.NoError(suite.T(), e) }
	seedPer("Cntmk4 One")
	seedPer("Nobody")
	allPer, err := QueryPersons(ctx, apiutil.QueryParams{Limit: big})
	assert.NoError(suite.T(), err)
	cPer, err := CountPersons(ctx, apiutil.QueryParams{Limit: big})
	assertCount("persons unfiltered", cPer, err, len(allPer))
	fPer, err := QueryPersons(ctx, apiutil.QueryParams{Limit: big, Filter: regexFilter("name", "cntmk4")})
	assert.NoError(suite.T(), err)
	cPerF, err := CountPersons(ctx, apiutil.QueryParams{Limit: big, Filter: regexFilter("name", "cntmk4")})
	assertCount("persons name~cntmk4", cPerF, err, len(fPer))

	seedLic := func(t string) { _, e := AddLicense(ctx, &vo.LicenseVO{Title: t}); assert.NoError(suite.T(), e) }
	seedLic("Cntmk5 One")
	seedLic("Plain License")
	allLic, err := QueryLicenses(ctx, apiutil.QueryParams{Limit: big})
	assert.NoError(suite.T(), err)
	cLic, err := CountLicenses(ctx, apiutil.QueryParams{Limit: big})
	assertCount("licenses unfiltered", cLic, err, len(allLic))
	fLic, err := QueryLicenses(ctx, apiutil.QueryParams{Limit: big, Filter: regexFilter("title", "cntmk5")})
	assert.NoError(suite.T(), err)
	cLicF, err := CountLicenses(ctx, apiutil.QueryParams{Limit: big, Filter: regexFilter("title", "cntmk5")})
	assertCount("licenses title~cntmk5", cLicF, err, len(fLic))
}

// 3b addendum: `sort=-field` sorts descending. go.jtlabs.io/query keeps the "-" and
// api-core.go hardcodes order 1, so normalizeSort has to turn {"-name": 1} into {"name": -1}.
func (suite *QueryPushdownTestSuite) TestDescendingSortByPrefixedField() {
	ctx := suite.T().Context()
	mk := fmt.Sprintf("srt%d", time.Now().UnixNano())

	for _, n := range []string{mk + " Charlie", mk + " Alpha", mk + " Bravo"} {
		_, err := AddPublisher(ctx, &vo.PublisherVO{Name: n})
		assert.NoError(suite.T(), err)
	}
	descParams := apiutil.QueryParams{
		Limit:  100,
		Sort:   []apiutil.Sort{{Field: "-name", Order: 1}}, // exactly what GetQueryParams yields for sort=-name
		Filter: regexFilter("name", mk),
	}
	rows, err := QueryPublishers(ctx, descParams)
	assert.NoError(suite.T(), err)
	got := make([]string, len(rows))
	for i, r := range rows {
		got[i] = r.Name
	}
	assert.Equal(suite.T(), []string{mk + " Charlie", mk + " Bravo", mk + " Alpha"}, got)

	// Ascending (plain sort=name) still works.
	ascParams := descParams
	ascParams.Sort = []apiutil.Sort{{Field: "name", Order: 1}}
	rows, err = QueryPublishers(ctx, ascParams)
	assert.NoError(suite.T(), err)
	for i, r := range rows {
		got[i] = r.Name
	}
	assert.Equal(suite.T(), []string{mk + " Alpha", mk + " Bravo", mk + " Charlie"}, got)
}

func (suite *QueryPushdownTestSuite) TestDescendingSortVolumesByPrefixedTitle() {
	ctx := suite.T().Context()
	mk := fmt.Sprintf("vsrt%d", time.Now().UnixNano())
	for _, t := range []string{mk + " C", mk + " A", mk + " B"} {
		_, err := AddVolume(ctx, &vo.VolumeVO{Title: t, Description: "x"})
		assert.NoError(suite.T(), err)
	}
	rows, err := QueryVolumes(ctx, apiutil.QueryParams{
		Limit:  100,
		Sort:   []apiutil.Sort{{Field: "-title", Order: 1}},
		Filter: []apiutil.Filter{{Field: "q", Value: []string{mk}}},
	})
	assert.NoError(suite.T(), err)
	got := make([]string, len(rows))
	for i, r := range rows {
		got[i] = r.Title
	}
	assert.Equal(suite.T(), []string{mk + " C", mk + " B", mk + " A"}, got)
}

func TestQueryPushdownTestSuite(t *testing.T) {
	suite.Run(t, new(QueryPushdownTestSuite))
}
