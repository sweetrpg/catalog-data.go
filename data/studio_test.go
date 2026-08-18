package data

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/sweetrpg/catalog-objects.go/models"
	"github.com/sweetrpg/catalog-objects.go/vo"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/mongodb.go/constants"
	"github.com/sweetrpg/mongodb.go/database"
)

type StudioDataTestSuite struct {
	suite.Suite
	seedStudioID string
}

func (suite *StudioDataTestSuite) SetupTest() {
	_ = os.Setenv(constants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()
	assert.NoError(suite.T(), EnsureStudioVersioningIndexes(suite.T().Context()))

	id, err := AddStudio(suite.T().Context(), &vo.StudioVO{Name: "Test Studio"})
	assert.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), id)
	suite.seedStudioID = *id
}

func (suite *StudioDataTestSuite) TestUpdateStudioLive() {
	updated, err := UpdateStudio(suite.T().Context(), suite.seedStudioID, &vo.StudioVO{
		Name: "Updated Studio",
	}, models.VersionStateLive)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), updated)
	assert.Equal(suite.T(), "Updated Studio", updated.Name)

	fetched, err := GetStudio(suite.T().Context(), suite.seedStudioID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), fetched)
	assert.Equal(suite.T(), "Updated Studio", fetched.Name)
}

func (suite *StudioDataTestSuite) TestUpdateStudioNotFound() {
	updated, err := UpdateStudio(suite.T().Context(), "does-not-exist", &vo.StudioVO{
		Name: "Doesn't Matter",
	}, models.VersionStateLive)
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), updated)
}

func (suite *StudioDataTestSuite) TestAcceptStudioVersionFullAccept() {
	submitted, err := UpdateStudio(suite.T().Context(), suite.seedStudioID, &vo.StudioVO{
		Name: "Proposed Studio",
	}, models.VersionStateSubmitted)
	assert.NoError(suite.T(), err)

	accepted, conflicts, err := AcceptStudioVersion(
		suite.T().Context(), suite.seedStudioID, submitted.Version, nil, "editor-1", nil)
	assert.NoError(suite.T(), err)
	assert.Empty(suite.T(), conflicts)
	assert.Equal(suite.T(), string(models.VersionStateLive), string(accepted.State))

	fetched, err := GetStudio(suite.T().Context(), suite.seedStudioID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Proposed Studio", fetched.Name)
}

func TestStudioDbTestSuite(t *testing.T) {
	suite.Run(t, new(StudioDataTestSuite))
}
