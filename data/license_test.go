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

type LicenseDataTestSuite struct {
	suite.Suite
	seedLicenseID string
}

func (suite *LicenseDataTestSuite) SetupTest() {
	_ = os.Setenv(constants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()
	assert.NoError(suite.T(), EnsureLicenseVersioningIndexes(suite.T().Context()))

	id, err := AddLicense(suite.T().Context(), &vo.LicenseVO{Title: "Test License", Status: "draft"})
	assert.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), id)
	suite.seedLicenseID = *id
}

func (suite *LicenseDataTestSuite) TestUpdateLicenseLive() {
	updated, err := UpdateLicense(suite.T().Context(), suite.seedLicenseID, &vo.LicenseVO{
		Title: "Updated License", Status: "active",
	}, models.VersionStateLive)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), updated)
	assert.Equal(suite.T(), "Updated License", updated.Title)
	assert.Equal(suite.T(), "active", updated.Status)

	fetched, err := GetLicense(suite.T().Context(), suite.seedLicenseID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), fetched)
	assert.Equal(suite.T(), "Updated License", fetched.Title)
}

func (suite *LicenseDataTestSuite) TestUpdateLicenseNotFound() {
	updated, err := UpdateLicense(suite.T().Context(), "does-not-exist", &vo.LicenseVO{
		Title: "Doesn't Matter",
	}, models.VersionStateLive)
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), updated)
}

func (suite *LicenseDataTestSuite) TestAcceptLicenseVersionFullAccept() {
	submitted, err := UpdateLicense(suite.T().Context(), suite.seedLicenseID, &vo.LicenseVO{
		Title: "Proposed License",
	}, models.VersionStateSubmitted)
	assert.NoError(suite.T(), err)

	accepted, conflicts, err := AcceptLicenseVersion(
		suite.T().Context(), suite.seedLicenseID, submitted.Version, nil, "editor-1", nil)
	assert.NoError(suite.T(), err)
	assert.Empty(suite.T(), conflicts)
	assert.Equal(suite.T(), string(models.VersionStateLive), string(accepted.State))

	fetched, err := GetLicense(suite.T().Context(), suite.seedLicenseID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Proposed License", fetched.Title)
}

func TestLicenseDbTestSuite(t *testing.T) {
	suite.Run(t, new(LicenseDataTestSuite))
}
