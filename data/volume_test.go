package data

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	apiutil "github.com/sweetrpg/api-core.go/util"
	"github.com/sweetrpg/catalog-objects.go/vo"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/mongodb.go/constants"
	"github.com/sweetrpg/mongodb.go/database"
)

type DataTestSuite struct {
	suite.Suite
}

func (suite *DataTestSuite) SetupTest() {
	os.Setenv(constants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()
}

func (suite *DataTestSuite) TestAddVolume() {
	id, err := AddVolume(suite.T().Context(), &vo.VolumeVO{
		Title:       "Test Volume",
		Description: "This is a test volume.",
	})
	assert.Nil(suite.T(), err)
	assert.NotEmpty(suite.T(), id)
}

func (suite *DataTestSuite) TestGetVolume() {
	volume, err := GetVolume(suite.T().Context(), "000000000000000000000000")
	assert.Nil(suite.T(), err)
	assert.NotEmpty(suite.T(), volume)
}

func (suite *DataTestSuite) TestQueryVolumes() {
	params := apiutil.QueryParams{
		Start: 0,
		Limit: 10,
	}
	volumes, err := QueryVolumes(suite.T().Context(), params)
	assert.Nil(suite.T(), err)
	assert.NotEmpty(suite.T(), volumes)
}

func (suite *DataTestSuite) TestQueryVolumesSorted() {
	params := apiutil.QueryParams{
		Start: 0,
		Limit: 10,
		Sort:  make([]apiutil.Sort, 0),
	}
	volumes, err := QueryVolumes(suite.T().Context(), params)
	assert.Nil(suite.T(), err)
	assert.NotEmpty(suite.T(), volumes)
}

func (suite *DataTestSuite) TestQueryVolumesFiltered() {
	params := apiutil.QueryParams{
		Start:  0,
		Limit:  10,
		Filter: make([]apiutil.Filter, 0),
	}
	volumes, err := QueryVolumes(suite.T().Context(), params)
	assert.Nil(suite.T(), err)
	assert.NotEmpty(suite.T(), volumes)
}

func (suite *DataTestSuite) TestQueryVolumesProjected() {
	params := apiutil.QueryParams{
		Start:      0,
		Limit:      10,
		Projection: make([]apiutil.Projection, 0),
	}
	volumes, err := QueryVolumes(suite.T().Context(), params)
	assert.Nil(suite.T(), err)
	assert.NotEmpty(suite.T(), volumes)
}

func (suite *DataTestSuite) TestQueryVolumesPaged() {
	params := apiutil.QueryParams{
		Limit: 10,
		Start: 0,
	}
	volumes, err := QueryVolumes(suite.T().Context(), params)
	assert.Nil(suite.T(), err)
	assert.NotEmpty(suite.T(), volumes)
}

func TestDbTestSuite(t *testing.T) {
	suite.Run(t, new(DataTestSuite))
}
