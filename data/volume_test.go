package data

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/sweetrpg/common.go/logging"
)

type DataTestSuite struct {
	suite.Suite
}

func (suite *DataTestSuite) SetupTest() {
}

func (suite *DataTestSuite) TestGetVolume() {
	logging.Init()

	// volume, err := GetVolume(suite.T().Context(), "60c72b2f9b1d4c001c8e4a5f")
}

func TestDbTestSuite(t *testing.T) {
	suite.Run(t, new(DataTestSuite))
}
