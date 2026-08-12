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
			{Field: "Title", Order: 1},
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

func (suite *VolumeDataTestSuite) TestUpdateVolume() {
	updated, err := UpdateVolume(suite.T().Context(), suite.seedVolumeID, &vo.VolumeVO{
		Title:       "Updated Test Volume",
		Description: "This volume was updated.",
	})
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), updated)
	assert.Equal(suite.T(), "Updated Test Volume", updated.Title)
	assert.Equal(suite.T(), "This volume was updated.", updated.Description)

	fetched, err := GetVolume(suite.T().Context(), suite.seedVolumeID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), fetched)
	assert.Equal(suite.T(), "Updated Test Volume", fetched.Title)
}

func (suite *VolumeDataTestSuite) TestUpdateVolumePreservesCreatedAudit() {
	before, err := GetVolume(suite.T().Context(), suite.seedVolumeID)
	assert.NoError(suite.T(), err)

	updated, err := UpdateVolume(suite.T().Context(), suite.seedVolumeID, &vo.VolumeVO{
		Title: "Another Update",
	})
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), updated)
	assert.Equal(suite.T(), before.CreatedAt, updated.CreatedAt)
	assert.True(suite.T(), updated.UpdatedAt.After(before.UpdatedAt) || updated.UpdatedAt.Equal(before.UpdatedAt))
}

func (suite *VolumeDataTestSuite) TestUpdateVolumeSetsFormat() {
	updated, err := UpdateVolume(suite.T().Context(), suite.seedVolumeID, &vo.VolumeVO{
		Title:  "Volume With Format",
		Format: "hardcover",
	})
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), updated)
	assert.Equal(suite.T(), "hardcover", updated.Format)

	fetched, err := GetVolume(suite.T().Context(), suite.seedVolumeID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "hardcover", fetched.Format)
}

func (suite *VolumeDataTestSuite) TestUpdateVolumeNotFound() {
	updated, err := UpdateVolume(suite.T().Context(), "000000000000000000000000", &vo.VolumeVO{
		Title: "Does Not Exist",
	})
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), updated)
}

// TestUpdateVolumeWithPublisherRelationship guards against a regression: VolumeVO.Publishers
// used to be a value slice ([]PublisherVO), which panicked when a caller further up the stack
// (catalog-api) marshaled it via the jsonapi library - a nil-check on a non-pointer struct.
// UpdateVolume/GetVolume don't marshal jsonapi themselves, but this confirms the pointer-slice
// round-trips correctly at this layer, which is the part that regressed.
func (suite *VolumeDataTestSuite) TestUpdateVolumeWithPublisherRelationship() {
	_, err := database.Insert("publishers", models.Publisher{ID: "pub-test-1", Name: "Test Publisher"})
	assert.NoError(suite.T(), err)

	updated, err := UpdateVolume(suite.T().Context(), suite.seedVolumeID, &vo.VolumeVO{
		Title:      "Volume With Publisher",
		Publishers: []*vo.PublisherVO{{ID: "pub-test-1"}},
	})
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), updated)
	assert.Len(suite.T(), updated.Publishers, 1)
	assert.Equal(suite.T(), "pub-test-1", updated.Publishers[0].ID)

	fetched, err := GetVolume(suite.T().Context(), suite.seedVolumeID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), fetched)
	assert.Len(suite.T(), fetched.Publishers, 1)
	assert.Equal(suite.T(), "pub-test-1", fetched.Publishers[0].ID)
	assert.Equal(suite.T(), "Test Publisher", fetched.Publishers[0].Name)
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

func TestDbTestSuite(t *testing.T) {
	suite.Run(t, new(VolumeDataTestSuite))
}
