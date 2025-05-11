package data

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	apiutil "github.com/sweetrpg/api-core.go/util"
	"github.com/sweetrpg/catalog-objects.go/vo"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/db.go/constants"
	"github.com/sweetrpg/db.go/database"
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
	assert.NotNil(suite.T(), err)
	assert.NotEmpty(suite.T(), id)
}

func (suite *DataTestSuite) TestGetVolume() {
	volume, err := GetVolume(suite.T().Context(), "64c7cf2fa3fc8ee7407f9b2e")
	assert.Nil(suite.T(), err)
	assert.NotEmpty(suite.T(), volume)
}

func (suite *DataTestSuite) TestQueryVolumes() {
	params := apiutil.QueryParams{}
	volumes, err := QueryVolumes(suite.T().Context(), params)
	assert.Nil(suite.T(), err)
	assert.NotEmpty(suite.T(), volumes)
}

func (suite *DataTestSuite) TestQueryVolumesSorted() {
	params := apiutil.QueryParams{
		Sort: make([]apiutil.Sort, 0),
	}
	volumes, err := QueryVolumes(suite.T().Context(), params)
	assert.Nil(suite.T(), err)
	assert.NotEmpty(suite.T(), volumes)
}

func (suite *DataTestSuite) TestQueryVolumesFiltered() {
	params := apiutil.QueryParams{
		Filter: make([]apiutil.Filter, 0),
	}
	volumes, err := QueryVolumes(suite.T().Context(), params)
	assert.Nil(suite.T(), err)
	assert.NotEmpty(suite.T(), volumes)
}

func (suite *DataTestSuite) TestQueryVolumesProjected() {
	params := apiutil.QueryParams{
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
