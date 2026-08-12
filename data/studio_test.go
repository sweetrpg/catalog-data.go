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
	"go.mongodb.org/mongo-driver/bson"
)

const seedStudioID = "seed-studio"

type StudioDataTestSuite struct {
	suite.Suite
}

func (suite *StudioDataTestSuite) SetupTest() {
	_ = os.Setenv(constants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()

	_, err := database.Insert[models.Studio]("studios", models.Studio{
		ID:   seedStudioID,
		Name: "Test Studio",
	})
	assert.NoError(suite.T(), err)
}

func (suite *StudioDataTestSuite) TearDownTest() {
	_, _ = database.Db.Collection("studios").DeleteMany(
		suite.T().Context(), bson.D{{Key: "_id", Value: seedStudioID}})
}

func (suite *StudioDataTestSuite) TestUpdateStudio() {
	updated, err := UpdateStudio(suite.T().Context(), seedStudioID, &vo.StudioVO{
		Name: "Updated Studio",
	})
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), updated)
	assert.Equal(suite.T(), "Updated Studio", updated.Name)

	fetched, err := GetStudio(suite.T().Context(), seedStudioID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), fetched)
	assert.Equal(suite.T(), "Updated Studio", fetched.Name)
}

func (suite *StudioDataTestSuite) TestUpdateStudioNotFound() {
	updated, err := UpdateStudio(suite.T().Context(), "does-not-exist", &vo.StudioVO{
		Name: "Doesn't Matter",
	})
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), updated)
}

func TestStudioDbTestSuite(t *testing.T) {
	suite.Run(t, new(StudioDataTestSuite))
}
