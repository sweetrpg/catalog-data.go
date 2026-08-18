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

type PersonDataTestSuite struct {
	suite.Suite
	seedPersonID string
}

func (suite *PersonDataTestSuite) SetupTest() {
	_ = os.Setenv(constants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()
	assert.NoError(suite.T(), EnsurePersonVersioningIndexes(suite.T().Context()))

	id, err := AddPerson(suite.T().Context(), &vo.PersonVO{Name: "Test Person"})
	assert.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), id)
	suite.seedPersonID = *id
}

func (suite *PersonDataTestSuite) TestUpdatePersonLive() {
	updated, err := UpdatePerson(suite.T().Context(), suite.seedPersonID, &vo.PersonVO{
		Name: "Updated Person",
	}, models.VersionStateLive)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), updated)
	assert.Equal(suite.T(), "Updated Person", updated.Name)

	fetched, err := GetPerson(suite.T().Context(), suite.seedPersonID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), fetched)
	assert.Equal(suite.T(), "Updated Person", fetched.Name)
}

func (suite *PersonDataTestSuite) TestUpdatePersonNotFound() {
	updated, err := UpdatePerson(suite.T().Context(), "does-not-exist", &vo.PersonVO{
		Name: "Doesn't Matter",
	}, models.VersionStateLive)
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), updated)
}

func (suite *PersonDataTestSuite) TestAcceptPersonVersionFullAccept() {
	submitted, err := UpdatePerson(suite.T().Context(), suite.seedPersonID, &vo.PersonVO{
		Name: "Proposed Person",
	}, models.VersionStateSubmitted)
	assert.NoError(suite.T(), err)

	accepted, conflicts, err := AcceptPersonVersion(
		suite.T().Context(), suite.seedPersonID, submitted.Version, nil, "editor-1", nil)
	assert.NoError(suite.T(), err)
	assert.Empty(suite.T(), conflicts)
	assert.Equal(suite.T(), string(models.VersionStateLive), string(accepted.State))

	fetched, err := GetPerson(suite.T().Context(), suite.seedPersonID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Proposed Person", fetched.Name)
}

func TestPersonDbTestSuite(t *testing.T) {
	suite.Run(t, new(PersonDataTestSuite))
}
