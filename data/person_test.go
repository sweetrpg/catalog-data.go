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

const seedPersonID = "seed-person"

type PersonDataTestSuite struct {
	suite.Suite
}

func (suite *PersonDataTestSuite) SetupTest() {
	_ = os.Setenv(constants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()

	_, err := database.Insert[models.Person]("persons", models.Person{
		ID:   seedPersonID,
		Name: "Test Person",
	})
	assert.NoError(suite.T(), err)
}

func (suite *PersonDataTestSuite) TearDownTest() {
	_, _ = database.Db.Collection("persons").DeleteMany(
		suite.T().Context(), bson.D{{Key: "_id", Value: seedPersonID}})
}

func (suite *PersonDataTestSuite) TestUpdatePerson() {
	updated, err := UpdatePerson(suite.T().Context(), seedPersonID, &vo.PersonVO{
		Name: "Updated Person",
	})
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), updated)
	assert.Equal(suite.T(), "Updated Person", updated.Name)

	fetched, err := GetPerson(suite.T().Context(), seedPersonID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), fetched)
	assert.Equal(suite.T(), "Updated Person", fetched.Name)
}

func (suite *PersonDataTestSuite) TestUpdatePersonNotFound() {
	updated, err := UpdatePerson(suite.T().Context(), "does-not-exist", &vo.PersonVO{
		Name: "Doesn't Matter",
	})
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), updated)
}

func TestPersonDbTestSuite(t *testing.T) {
	suite.Run(t, new(PersonDataTestSuite))
}
