package data

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	apiutil "github.com/sweetrpg/api-core.go/util"
	"github.com/sweetrpg/catalog-objects.go/models"
	"github.com/sweetrpg/catalog-objects.go/vo"
	"github.com/sweetrpg/common.go/logging"
	dbconstants "github.com/sweetrpg/mongodb.go/constants"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
)

// SoftDeleteCountsTestSuite covers catalog-entity-browse's list-total and page requirements and
// catalog-landing-summary's count / most-recent requirements for soft-deleted records: a
// soft-deleted record is excluded from query pages, from count totals, and from stats, and
// "most recent" means most recently created, not most recently edited.
type SoftDeleteCountsTestSuite struct {
	suite.Suite
}

func (suite *SoftDeleteCountsTestSuite) SetupTest() {
	_ = os.Setenv(dbconstants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()
	assert.NoError(suite.T(), EnsureStudioVersioningIndexes(suite.T().Context()))
	assert.NoError(suite.T(), EnsureVolumeVersioningIndexes(suite.T().Context()))
}

// pinClock hands out strictly increasing created_at values (1 ms apart, starting an hour ahead of
// the wall clock so they sort after records other suites left behind). Records added in the same
// millisecond share a created_at, so tests pin creation order instead of relying on insert timing.
var pinClock = time.Now().Add(time.Hour).Truncate(time.Millisecond)

func nextPinnedTime() time.Time {
	pinClock = pinClock.Add(time.Millisecond)
	return pinClock
}

func setCreatedAt(t *testing.T, metaCollection, id string, at time.Time) {
	_, err := database.Db.Collection(metaCollection).UpdateOne(t.Context(),
		bson.D{{Key: "_id", Value: id}}, bson.D{{Key: "$set", Value: bson.D{{Key: "created_at", Value: at}}}})
	assert.NoError(t, err)
}

func (suite *SoftDeleteCountsTestSuite) setCreatedAt(metaCollection, id string, at time.Time) {
	setCreatedAt(suite.T(), metaCollection, id, at)
}

func qParams(term string, start int64, limit int) apiutil.QueryParams {
	return apiutil.QueryParams{
		Start: start, Limit: limit,
		Filter: []apiutil.Filter{{Field: "q", Value: []string{term}}},
	}
}

func studioNames(rows []*vo.StudioVO) []string {
	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = r.Name
	}
	return names
}

// addStudios adds one studio per name and returns their IDs in the same order.
func (suite *SoftDeleteCountsTestSuite) addStudios(names ...string) []string {
	ids := make([]string, len(names))
	for i, n := range names {
		id, err := AddStudio(suite.T().Context(), &vo.StudioVO{Name: n})
		assert.NoError(suite.T(), err)
		ids[i] = *id
		suite.setCreatedAt(studioVersioning.metaCollection, ids[i], nextPinnedTime())
	}
	return ids
}

func (suite *SoftDeleteCountsTestSuite) TestStudioQueryAndCountExcludeSoftDeleted() {
	ctx := suite.T().Context()
	ids := suite.addStudios("sdc1 Alpha", "sdc1 Bravo", "sdc1 Charlie")
	assert.NoError(suite.T(), SoftDeleteStudio(ctx, ids[1], "tester"))

	rows, err := QueryStudios(ctx, qParams("sdc1", 0, 50))
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), []string{"sdc1 Alpha", "sdc1 Charlie"}, studioNames(rows))

	total, err := CountStudios(ctx, qParams("sdc1", 0, 0))
	assert.NoError(suite.T(), err)
	assert.EqualValues(suite.T(), 2, total)
}

// A page boundary: the soft-deleted record sorts into the first page, which must still come back
// full, with the remainder on page two - not a short first page.
func (suite *SoftDeleteCountsTestSuite) TestStudioPagesAreFullAroundSoftDeleted() {
	ctx := suite.T().Context()
	ids := suite.addStudios("sdc2 A", "sdc2 B", "sdc2 C", "sdc2 D", "sdc2 E")
	assert.NoError(suite.T(), SoftDeleteStudio(ctx, ids[0], "tester"))
	assert.NoError(suite.T(), SoftDeleteStudio(ctx, ids[2], "tester"))

	first, err := QueryStudios(ctx, qParams("sdc2", 0, 2))
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), []string{"sdc2 B", "sdc2 D"}, studioNames(first))

	second, err := QueryStudios(ctx, qParams("sdc2", 2, 2))
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), []string{"sdc2 E"}, studioNames(second))

	total, err := CountStudios(ctx, qParams("sdc2", 0, 0))
	assert.NoError(suite.T(), err)
	assert.EqualValues(suite.T(), 3, total)
}

func (suite *SoftDeleteCountsTestSuite) TestStudioRestoreReturnsToQueryCountAndStats() {
	ctx := suite.T().Context()
	ids := suite.addStudios("sdc3 Restored")
	assert.NoError(suite.T(), SoftDeleteStudio(ctx, ids[0], "tester"))
	assert.NoError(suite.T(), RestoreStudio(ctx, ids[0], "tester"))

	total, err := CountStudios(ctx, qParams("sdc3", 0, 0))
	assert.NoError(suite.T(), err)
	assert.EqualValues(suite.T(), 1, total)
	stats, err := GetStudioStats(ctx)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), ids[0], stats.MostRecentID)
}

func (suite *SoftDeleteCountsTestSuite) TestStudioStatsExcludeSoftDeleted() {
	ctx := suite.T().Context()
	before, err := GetStudioStats(ctx)
	assert.NoError(suite.T(), err)

	kept := suite.addStudios("sdc4 Kept")[0]
	gone := suite.addStudios("sdc4 Gone")[0] // newest
	assert.NoError(suite.T(), SoftDeleteStudio(ctx, gone, "tester"))

	after, err := GetStudioStats(ctx)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), before.Count+1, after.Count, "deleted studio is not counted")
	assert.Equal(suite.T(), kept, after.MostRecentID, "deleted studio is not the most recent")
	assert.Equal(suite.T(), "sdc4 Kept", after.MostRecentName)
}

// Editing an older record gives it a newer live version; "most recent" still means the record
// created last.
func (suite *SoftDeleteCountsTestSuite) TestStudioStatsMostRecentIsLastCreatedNotLastEdited() {
	ctx := suite.T().Context()
	ids := suite.addStudios("sdc5 Older", "sdc5 Newer")
	_, err := UpdateStudio(ctx, ids[0], &vo.StudioVO{Name: "sdc5 Older Edited"}, models.VersionStateLive)
	assert.NoError(suite.T(), err)

	stats, err := GetStudioStats(ctx)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), ids[1], stats.MostRecentID)
	assert.Equal(suite.T(), "sdc5 Newer", stats.MostRecentName)
	assert.NotNil(suite.T(), stats.LastUpdated)
}

// The newest record exists but its only version is pending review, so it is neither counted nor
// shown as "most recent".
func (suite *SoftDeleteCountsTestSuite) TestStudioStatsSkipNewestRecordWithoutLiveVersion() {
	ctx := suite.T().Context()
	before, err := GetStudioStats(ctx)
	assert.NoError(suite.T(), err)
	live := suite.addStudios("sdc8 Live")[0]
	pending := suite.addStudios("sdc8 Pending")[0] // newest
	assert.NoError(suite.T(), studioVersioning.setVersionState(ctx, pending, 1,
		bson.D{{Key: "state", Value: string(models.VersionStateSubmitted)}}))

	after, err := GetStudioStats(ctx)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), before.Count+1, after.Count)
	assert.Equal(suite.T(), live, after.MostRecentID)
	assert.Equal(suite.T(), "sdc8 Live", after.MostRecentName)
	assert.NotNil(suite.T(), after.LastUpdated)
}

func (suite *SoftDeleteCountsTestSuite) TestVolumeStatsSkipNewestRecordWithoutLiveVersion() {
	ctx := suite.T().Context()
	before, err := GetVolumeTypeStats(ctx)
	assert.NoError(suite.T(), err)
	ids := suite.addVolumes("sdc9 Live", "sdc9 Pending")
	_, err = database.Db.Collection(volumeVersionCollection).UpdateOne(ctx,
		bson.D{{Key: "record_id", Value: ids[1]}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "state", Value: string(models.VersionStateSubmitted)}}}})
	assert.NoError(suite.T(), err)

	after, err := GetVolumeTypeStats(ctx)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), before.Count+1, after.Count)
	assert.Equal(suite.T(), ids[0], after.MostRecentID)
	assert.Equal(suite.T(), "sdc9 Live", after.MostRecentName)
}

// Two records with the same created_at: the larger record ID (the later insert) is the most recent,
// every time.
func (suite *SoftDeleteCountsTestSuite) TestStatsMostRecentTieBreaksOnRecordID() {
	ctx := suite.T().Context()
	ids := suite.addStudios("sdctie First", "sdctie Second")
	tied := nextPinnedTime()
	for _, id := range ids {
		suite.setCreatedAt(studioVersioning.metaCollection, id, tied)
	}
	want := max(ids[0], ids[1])
	for range 5 {
		stats, err := GetStudioStats(ctx)
		assert.NoError(suite.T(), err)
		assert.Equal(suite.T(), want, stats.MostRecentID)
	}

	vids := suite.addVolumes("sdctie First", "sdctie Second")
	tied = nextPinnedTime()
	for _, id := range vids {
		suite.setCreatedAt(volumeMetaCollection, id, tied)
	}
	vstats, err := GetVolumeTypeStats(ctx)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), max(vids[0], vids[1]), vstats.MostRecentID)
}

func volumeTitles(rows []*vo.VolumeVO) []string {
	titles := make([]string, len(rows))
	for i, r := range rows {
		titles[i] = r.Title
	}
	return titles
}

func (suite *SoftDeleteCountsTestSuite) addVolumes(titles ...string) []string {
	ids := make([]string, len(titles))
	for i, t := range titles {
		id, err := AddVolume(suite.T().Context(), &vo.VolumeVO{Title: t})
		assert.NoError(suite.T(), err)
		ids[i] = *id
		suite.setCreatedAt(volumeMetaCollection, ids[i], nextPinnedTime())
	}
	return ids
}

func (suite *SoftDeleteCountsTestSuite) TestVolumePagesCountsAndStatsExcludeSoftDeleted() {
	ctx := suite.T().Context()
	before, err := GetVolumeTypeStats(ctx)
	assert.NoError(suite.T(), err)
	beforeCatalog, err := GetCatalogStats(ctx)
	assert.NoError(suite.T(), err)

	ids := suite.addVolumes("sdc6 A", "sdc6 B", "sdc6 C", "sdc6 D")
	assert.NoError(suite.T(), SoftDeleteVolume(ctx, ids[0], "tester"))
	assert.NoError(suite.T(), SoftDeleteVolume(ctx, ids[3], "tester")) // newest

	first, err := QueryVolumes(ctx, qParams("sdc6", 0, 2))
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), []string{"sdc6 B", "sdc6 C"}, volumeTitles(first))
	second, err := QueryVolumes(ctx, qParams("sdc6", 2, 2))
	assert.NoError(suite.T(), err)
	assert.Empty(suite.T(), second)

	total, err := CountVolumes(ctx, qParams("sdc6", 0, 0))
	assert.NoError(suite.T(), err)
	assert.EqualValues(suite.T(), 2, total)

	after, err := GetVolumeTypeStats(ctx)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), before.Count+2, after.Count)
	assert.Equal(suite.T(), ids[2], after.MostRecentID)
	afterCatalog, err := GetCatalogStats(ctx)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), beforeCatalog.VolumeCount+2, afterCatalog.VolumeCount)
}

func (suite *SoftDeleteCountsTestSuite) TestVolumeStatsMostRecentIsLastCreatedNotLastEdited() {
	ctx := suite.T().Context()
	ids := suite.addVolumes("sdc7 Older", "sdc7 Newer")
	_, err := UpdateVolume(ctx, ids[0], &vo.VolumeVO{Title: "sdc7 Older Edited"}, models.VersionStateLive)
	assert.NoError(suite.T(), err)

	stats, err := GetVolumeTypeStats(ctx)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), ids[1], stats.MostRecentID)
	assert.Equal(suite.T(), "sdc7 Newer", stats.MostRecentName)
}

func TestSoftDeleteCountsSuite(t *testing.T) {
	suite.Run(t, new(SoftDeleteCountsTestSuite))
}
