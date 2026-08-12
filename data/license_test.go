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

const seedLicenseID = "seed-license"

type LicenseDataTestSuite struct {
	suite.Suite
}

func (suite *LicenseDataTestSuite) SetupTest() {
	_ = os.Setenv(constants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()

	_, err := database.Insert[models.License]("licenses", models.License{
		ID:     seedLicenseID,
		Title:  "Test License",
		Status: "draft",
	})
	assert.NoError(suite.T(), err)
}

func (suite *LicenseDataTestSuite) TearDownTest() {
	_, _ = database.Db.Collection("licenses").DeleteMany(
		suite.T().Context(), bson.D{{Key: "_id", Value: seedLicenseID}})
}

func (suite *LicenseDataTestSuite) TestUpdateLicense() {
	updated, err := UpdateLicense(suite.T().Context(), seedLicenseID, &vo.LicenseVO{
		Title:  "Updated License",
		Status: "active",
	})
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), updated)
	assert.Equal(suite.T(), "Updated License", updated.Title)
	assert.Equal(suite.T(), "active", updated.Status)

	fetched, err := GetLicense(suite.T().Context(), seedLicenseID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), fetched)
	assert.Equal(suite.T(), "Updated License", fetched.Title)
}

func (suite *LicenseDataTestSuite) TestUpdateLicenseNotFound() {
	updated, err := UpdateLicense(suite.T().Context(), "does-not-exist", &vo.LicenseVO{
		Title: "Doesn't Matter",
	})
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), updated)
}

func TestLicenseDbTestSuite(t *testing.T) {
	suite.Run(t, new(LicenseDataTestSuite))
}
