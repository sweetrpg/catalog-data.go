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

// seedPublisherID is a fixed, non-ObjectID string on purpose - Publisher's "_id" is a plain
// string field (not a primitive.ObjectID), so database.Insert's ObjectID-typed return value
// doesn't apply here; the seed document's own ID is used directly instead.
const seedPublisherID = "seed-publisher"

type PublisherDataTestSuite struct {
	suite.Suite
}

func (suite *PublisherDataTestSuite) SetupTest() {
	_ = os.Setenv(constants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()

	_, err := database.Insert[models.Publisher]("publishers", models.Publisher{
		ID:      seedPublisherID,
		Name:    "Test Publisher",
		Address: "123 Test St",
	})
	assert.NoError(suite.T(), err)
}

func (suite *PublisherDataTestSuite) TearDownTest() {
	_, _ = database.Db.Collection("publishers").DeleteMany(
		suite.T().Context(), bson.D{{Key: "_id", Value: seedPublisherID}})
}

func (suite *PublisherDataTestSuite) TestUpdatePublisher() {
	updated, err := UpdatePublisher(suite.T().Context(), seedPublisherID, &vo.PublisherVO{
		Name:    "Updated Publisher",
		Address: "456 Updated Ave",
	})
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), updated)
	assert.Equal(suite.T(), "Updated Publisher", updated.Name)
	assert.Equal(suite.T(), "456 Updated Ave", updated.Address)

	fetched, err := GetPublisher(suite.T().Context(), seedPublisherID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), fetched)
	assert.Equal(suite.T(), "Updated Publisher", fetched.Name)
}

func (suite *PublisherDataTestSuite) TestUpdatePublisherNotFound() {
	updated, err := UpdatePublisher(suite.T().Context(), "does-not-exist", &vo.PublisherVO{
		Name: "Doesn't Matter",
	})
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), updated)
}

func TestPublisherDbTestSuite(t *testing.T) {
	suite.Run(t, new(PublisherDataTestSuite))
}
