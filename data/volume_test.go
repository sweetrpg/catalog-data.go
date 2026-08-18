package data

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	apiutil "github.com/sweetrpg/api-core.go/util"
	"github.com/sweetrpg/catalog-objects.go/models"
	"github.com/sweetrpg/catalog-objects.go/vo"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/mongodb.go/constants"
	"github.com/sweetrpg/mongodb.go/database"
)

type VolumeDataTestSuite struct {
	suite.Suite
	seedVolumeID string
}

func (suite *VolumeDataTestSuite) SetupTest() {
	_ = os.Setenv(constants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()
	assert.NoError(suite.T(), EnsureVolumeVersioningIndexes(suite.T().Context()))

	id, err := AddVolume(suite.T().Context(), &vo.VolumeVO{
		Title:       "Test Volume",
		Description: "This is a test volume.",
	})
	assert.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), id)
	suite.seedVolumeID = *id
}

func (suite *VolumeDataTestSuite) TestAddVolume() {
	id, err := AddVolume(suite.T().Context(), &vo.VolumeVO{
		Title:       "Another Test Volume",
		Description: "This is another test volume.",
	})
	assert.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), id)
	assert.NotEqual(suite.T(), suite.seedVolumeID, *id)
}

func (suite *VolumeDataTestSuite) TestGetVolume() {
	volume, err := GetVolume(suite.T().Context(), suite.seedVolumeID)
	assert.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), volume)
	assert.Equal(suite.T(), "Test Volume", volume.Title)
}

func (suite *VolumeDataTestSuite) TestGetVolumeNotFound() {
	volume, err := GetVolume(suite.T().Context(), "000000000000000000000000")
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), volume)
}

func (suite *VolumeDataTestSuite) TestQueryVolumes() {
	params := apiutil.QueryParams{
		Start: 0,
		Limit: 10,
	}
	volumes, err := QueryVolumes(suite.T().Context(), params)
	assert.Nil(suite.T(), err)
	assert.NotEmpty(suite.T(), volumes)
}

func (suite *VolumeDataTestSuite) TestQueryVolumesSorted() {
	params := apiutil.QueryParams{
		Start: 0,
		Limit: 10,
		Sort: []apiutil.Sort{
			{Field: "title", Order: 1},
		},
	}
	volumes, err := QueryVolumes(suite.T().Context(), params)
	assert.Nil(suite.T(), err)
	assert.NotEmpty(suite.T(), volumes)
}

func (suite *VolumeDataTestSuite) TestQueryVolumesFiltered() {
	params := apiutil.QueryParams{
		Start:  0,
		Limit:  10,
		Filter: make([]apiutil.Filter, 0),
	}
	volumes, err := QueryVolumes(suite.T().Context(), params)
	assert.Nil(suite.T(), err)
	assert.NotEmpty(suite.T(), volumes)
}

func (suite *VolumeDataTestSuite) TestQueryVolumesProjected() {
	params := apiutil.QueryParams{
		Start:      0,
		Limit:      10,
		Projection: make([]apiutil.Projection, 0),
	}
	volumes, err := QueryVolumes(suite.T().Context(), params)
	assert.Nil(suite.T(), err)
	assert.NotEmpty(suite.T(), volumes)
}

// TestQueryVolumesDefaultsToTitleSortSoEditingDoesNotEvictARecord reproduces a live-observed
// bug: with no explicit sort, QueryVolumes used Mongo's natural (insertion) order, so editing a
// volume - which inserts a new live version document rather than mutating one in place - moved
// that volume to the back of natural order and could push it past a caller's page limit
// entirely. A browse list must not reorder itself just because one record was edited.
func (suite *VolumeDataTestSuite) TestQueryVolumesDefaultsToTitleSortSoEditingDoesNotEvictARecord() {
	// "AAA Volume" sorts first alphabetically but is added and edited last, so it's the most
	// recently inserted live version - the position natural order would put at the back.
	for i := 0; i < 5; i++ {
		_, err := AddVolume(suite.T().Context(), &vo.VolumeVO{Title: "Filler Volume"})
		assert.NoError(suite.T(), err)
	}
	firstID, err := AddVolume(suite.T().Context(), &vo.VolumeVO{Title: "AAA Volume"})
	assert.NoError(suite.T(), err)
	_, err = UpdateVolume(suite.T().Context(), *firstID, &vo.VolumeVO{Title: "AAA Volume", Description: "edited"}, models.VersionStateLive)
	assert.NoError(suite.T(), err)

	volumes, err := QueryVolumes(suite.T().Context(), apiutil.QueryParams{Start: 0, Limit: 3})
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), volumes, 3)
	assert.Equal(suite.T(), "AAA Volume", volumes[0].Title)
}

func (suite *VolumeDataTestSuite) TestQueryVolumesPaged() {
	params := apiutil.QueryParams{
		Limit: 10,
		Start: 0,
	}
	volumes, err := QueryVolumes(suite.T().Context(), params)
	assert.Nil(suite.T(), err)
	assert.NotEmpty(suite.T(), volumes)
}

func (suite *VolumeDataTestSuite) TestUpdateVolumeLiveMovesCurrentVersion() {
	version, err := UpdateVolume(suite.T().Context(), suite.seedVolumeID, &vo.VolumeVO{
		Title:       "Updated Test Volume",
		Description: "This volume was updated.",
	}, models.VersionStateLive)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), version)
	assert.Equal(suite.T(), models.VersionStateLive, models.VersionState(version.State))
	assert.Equal(suite.T(), 2, version.Version)

	fetched, err := GetVolume(suite.T().Context(), suite.seedVolumeID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), fetched)
	assert.Equal(suite.T(), "Updated Test Volume", fetched.Title)

	versions, err := ListVolumeVersions(suite.T().Context(), suite.seedVolumeID)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), versions, 2)
	assert.Equal(suite.T(), models.VersionStateArchived, models.VersionState(versions[1].State))
}

func (suite *VolumeDataTestSuite) TestUpdateVolumeSubmittedLeavesCurrentVersionUnchanged() {
	version, err := UpdateVolume(suite.T().Context(), suite.seedVolumeID, &vo.VolumeVO{
		Title: "Proposed Title",
	}, models.VersionStateSubmitted)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), version)
	assert.Equal(suite.T(), models.VersionStateSubmitted, models.VersionState(version.State))

	fetched, err := GetVolume(suite.T().Context(), suite.seedVolumeID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Test Volume", fetched.Title)
}

func (suite *VolumeDataTestSuite) TestUpdateVolumeSetsCoverAndSampleAssetIds() {
	version, err := UpdateVolume(suite.T().Context(), suite.seedVolumeID, &vo.VolumeVO{
		Title:          "Volume With Assets",
		CoverAssetId:   "cover-abc",
		SampleAssetIds: []string{"sample-1", "sample-2"},
	}, models.VersionStateLive)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), version)
	assert.Equal(suite.T(), "cover-abc", version.CoverAssetId)
	assert.Equal(suite.T(), []string{"sample-1", "sample-2"}, version.SampleAssetIds)

	fetched, err := GetVolume(suite.T().Context(), suite.seedVolumeID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "cover-abc", fetched.CoverAssetId)
	assert.Equal(suite.T(), []string{"sample-1", "sample-2"}, fetched.SampleAssetIds)
}

func (suite *VolumeDataTestSuite) TestUpdateVolumePreservesCreatedAudit() {
	before, err := GetVolume(suite.T().Context(), suite.seedVolumeID)
	assert.NoError(suite.T(), err)

	_, err = UpdateVolume(suite.T().Context(), suite.seedVolumeID, &vo.VolumeVO{
		Title: "Another Update",
	}, models.VersionStateLive)
	assert.NoError(suite.T(), err)

	after, err := GetVolume(suite.T().Context(), suite.seedVolumeID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), before.CreatedAt, after.CreatedAt)
	assert.True(suite.T(), after.UpdatedAt.After(before.UpdatedAt) || after.UpdatedAt.Equal(before.UpdatedAt))
}

func (suite *VolumeDataTestSuite) TestUpdateVolumeSetsFormat() {
	version, err := UpdateVolume(suite.T().Context(), suite.seedVolumeID, &vo.VolumeVO{
		Title:  "Volume With Format",
		Format: "hardcover",
	}, models.VersionStateLive)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "hardcover", version.Format)

	fetched, err := GetVolume(suite.T().Context(), suite.seedVolumeID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "hardcover", fetched.Format)
}

func (suite *VolumeDataTestSuite) TestUpdateVolumeNotFound() {
	version, err := UpdateVolume(suite.T().Context(), "000000000000000000000000", &vo.VolumeVO{
		Title: "Does Not Exist",
	}, models.VersionStateLive)
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), version)
}

// TestUpdateVolumeWithPublisherRelationship guards against a regression: VolumeVO.Publishers
// used to be a value slice ([]PublisherVO), which panicked when a caller further up the stack
// (catalog-api) marshaled it via the jsonapi library - a nil-check on a non-pointer struct.
// UpdateVolume/GetVolume don't marshal jsonapi themselves, but this confirms the pointer-slice
// round-trips correctly at this layer, which is the part that regressed.
func (suite *VolumeDataTestSuite) TestUpdateVolumeWithPublisherRelationship() {
	publisherID, err := AddPublisher(suite.T().Context(), &vo.PublisherVO{Name: "Test Publisher"})
	assert.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), publisherID)

	_, err = UpdateVolume(suite.T().Context(), suite.seedVolumeID, &vo.VolumeVO{
		Title:      "Volume With Publisher",
		Publishers: []*vo.PublisherVO{{ID: *publisherID}},
	}, models.VersionStateLive)
	assert.NoError(suite.T(), err)

	fetched, err := GetVolume(suite.T().Context(), suite.seedVolumeID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), fetched)
	assert.Len(suite.T(), fetched.Publishers, 1)
	assert.Equal(suite.T(), *publisherID, fetched.Publishers[0].ID)
	assert.Equal(suite.T(), "Test Publisher", fetched.Publishers[0].Name)
}

func (suite *VolumeDataTestSuite) TestListVolumeVersions() {
	_, err := UpdateVolume(suite.T().Context(), suite.seedVolumeID, &vo.VolumeVO{Title: "V2"}, models.VersionStateLive)
	assert.NoError(suite.T(), err)

	versions, err := ListVolumeVersions(suite.T().Context(), suite.seedVolumeID)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), versions, 2)
	assert.Equal(suite.T(), 2, versions[0].Version)
	assert.Equal(suite.T(), 1, versions[1].Version)
}

func (suite *VolumeDataTestSuite) TestGetVolumeVersion() {
	version, err := GetVolumeVersion(suite.T().Context(), suite.seedVolumeID, 1)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), version)
	assert.Equal(suite.T(), "Test Volume", version.Title)
}

func (suite *VolumeDataTestSuite) TestGetVolumeVersionNotFound() {
	version, err := GetVolumeVersion(suite.T().Context(), suite.seedVolumeID, 99)
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), version)
}

func (suite *VolumeDataTestSuite) TestAcceptVolumeVersionFullCleanPromotesSubmission() {
	submitted, err := UpdateVolume(suite.T().Context(), suite.seedVolumeID, &vo.VolumeVO{
		Title:       "Submitted Title",
		Description: "This is a test volume.",
	}, models.VersionStateSubmitted)
	assert.NoError(suite.T(), err)

	accepted, conflicts, err := AcceptVolumeVersion(suite.T().Context(), suite.seedVolumeID, submitted.Version, nil, "editor-1", nil, nil, nil)
	assert.NoError(suite.T(), err)
	assert.Empty(suite.T(), conflicts)
	assert.Equal(suite.T(), submitted.Version, accepted.Version)
	assert.Equal(suite.T(), models.VersionStateLive, models.VersionState(accepted.State))

	fetched, err := GetVolume(suite.T().Context(), suite.seedVolumeID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Submitted Title", fetched.Title)
}

func (suite *VolumeDataTestSuite) TestAcceptVolumeVersionSelectedFieldsDerivesNewVersion() {
	submitted, err := UpdateVolume(suite.T().Context(), suite.seedVolumeID, &vo.VolumeVO{
		Title:       "Submitted Title",
		Description: "Submitted description.",
	}, models.VersionStateSubmitted)
	assert.NoError(suite.T(), err)

	accepted, conflicts, err := AcceptVolumeVersion(suite.T().Context(), suite.seedVolumeID, submitted.Version, []string{"title"}, "editor-1", nil, nil, nil)
	assert.NoError(suite.T(), err)
	assert.Empty(suite.T(), conflicts)
	assert.NotEqual(suite.T(), submitted.Version, accepted.Version)
	assert.Equal(suite.T(), "Submitted Title", accepted.Title)
	assert.Equal(suite.T(), "This is a test volume.", accepted.Description)

	original, err := GetVolumeVersion(suite.T().Context(), suite.seedVolumeID, submitted.Version)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), models.VersionStatePartiallyAccepted, models.VersionState(original.State))
	assert.NotNil(suite.T(), original.ResultingVersion)
	assert.Equal(suite.T(), accepted.Version, *original.ResultingVersion)
}

func (suite *VolumeDataTestSuite) TestAcceptVolumeVersionConflictExcludesDriftedField() {
	submitted, err := UpdateVolume(suite.T().Context(), suite.seedVolumeID, &vo.VolumeVO{
		Title:       "Submitted Title",
		Description: "Submitted description.",
	}, models.VersionStateSubmitted)
	assert.NoError(suite.T(), err)

	// A different edit lands on the live record after submission, drifting Description.
	_, err = UpdateVolume(suite.T().Context(), suite.seedVolumeID, &vo.VolumeVO{
		Title:       "Test Volume",
		Description: "Someone else changed this first.",
	}, models.VersionStateLive)
	assert.NoError(suite.T(), err)

	accepted, conflicts, err := AcceptVolumeVersion(suite.T().Context(), suite.seedVolumeID, submitted.Version, nil, "editor-1", nil, nil, nil)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), []string{"description"}, conflicts)
	assert.Equal(suite.T(), "Submitted Title", accepted.Title)
	assert.Equal(suite.T(), "Someone else changed this first.", accepted.Description)
}

func (suite *VolumeDataTestSuite) TestRejectVolumeVersion() {
	submitted, err := UpdateVolume(suite.T().Context(), suite.seedVolumeID, &vo.VolumeVO{
		Title: "Rejected Title",
	}, models.VersionStateSubmitted)
	assert.NoError(suite.T(), err)

	note := "not a good fit"
	err = RejectVolumeVersion(suite.T().Context(), suite.seedVolumeID, submitted.Version, "editor-1", &note)
	assert.NoError(suite.T(), err)

	rejected, err := GetVolumeVersion(suite.T().Context(), suite.seedVolumeID, submitted.Version)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), models.VersionStateRejected, models.VersionState(rejected.State))
	assert.Equal(suite.T(), &note, rejected.ReviewNote)

	fetched, err := GetVolume(suite.T().Context(), suite.seedVolumeID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Test Volume", fetched.Title)
}

func (suite *VolumeDataTestSuite) TestSetCurrentVolumeVersionRollback() {
	_, err := UpdateVolume(suite.T().Context(), suite.seedVolumeID, &vo.VolumeVO{
		Title: "V2 Title",
	}, models.VersionStateLive)
	assert.NoError(suite.T(), err)

	restored, err := SetCurrentVolumeVersion(suite.T().Context(), suite.seedVolumeID, 1)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), models.VersionStateLive, models.VersionState(restored.State))

	fetched, err := GetVolume(suite.T().Context(), suite.seedVolumeID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Test Volume", fetched.Title)

	v2, err := GetVolumeVersion(suite.T().Context(), suite.seedVolumeID, 2)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), models.VersionStateArchived, models.VersionState(v2.State))
}

func TestDbTestSuite(t *testing.T) {
	suite.Run(t, new(VolumeDataTestSuite))
}
