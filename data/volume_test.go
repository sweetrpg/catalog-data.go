package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/sweetrpg/catalog-objects.go/vo"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/db.go/database"
)

type DataTestSuite struct {
	suite.Suite
}

func (suite *DataTestSuite) SetupTest() {
	logging.Init()
	database.SetupDatabase()
}

func (suite *DataTestSuite) TestAddVolume() {
	id, err := AddVolume(suite.T().Context(), &vo.VolumeVO{
		Title:       "Test Volume",
		Description: "This is a test volume.",
	})
	assert.NotNil(suite.T(), err)
	assert.NotEmpty(suite.T(), id)
}

func (suite *DataTestSuite) TestGetVolume() {
	// volume, err := GetVolume(suite.T().Context(), "60c72b2f9b1d4c001c8e4a5f")
}

func TestDbTestSuite(t *testing.T) {
	suite.Run(t, new(DataTestSuite))
}
