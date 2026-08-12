package data

import (
	"github.com/stretchr/testify/assert"
	catalogmodels "github.com/sweetrpg/catalog-objects.go/models"
	"github.com/sweetrpg/mongodb.go/database"
)

// These live as methods on VolumeDataTestSuite (defined in volume_test.go) rather than
// standalone Test* funcs so they pick up SetupTest's logging.Init()/database.SetupDatabase()
// call - this package has no package-level TestMain of its own.

func (suite *VolumeDataTestSuite) TestAddContributionAndQueryByVolume() {
	ctx := suite.T().Context()

	_, err := database.Insert("persons", catalogmodels.Person{ID: "person-contrib-1", Name: "Test Author"})
	assert.NoError(suite.T(), err)

	id, err := AddContribution(ctx, "person-contrib-1", suite.seedVolumeID, []string{"Author"}, "auth0|editor")
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), id)
	assert.NotEmpty(suite.T(), *id)

	contributions, err := QueryContributionsByVolume(ctx, suite.seedVolumeID)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), contributions, 1)
	assert.Equal(suite.T(), "person-contrib-1", contributions[0].Person.ID)
	assert.Equal(suite.T(), []string{"Author"}, contributions[0].Roles)

	got, err := GetContribution(ctx, *id)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), got)
	assert.Equal(suite.T(), *id, got.ID)
}

func (suite *VolumeDataTestSuite) TestDeleteContribution() {
	ctx := suite.T().Context()

	_, err := database.Insert("persons", catalogmodels.Person{ID: "person-contrib-2", Name: "Another Author"})
	assert.NoError(suite.T(), err)

	id, err := AddContribution(ctx, "person-contrib-2", suite.seedVolumeID, []string{"Editor"}, "auth0|editor")
	assert.NoError(suite.T(), err)

	deleted, err := DeleteContribution(ctx, *id)
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), deleted)

	got, err := GetContribution(ctx, *id)
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), got)

	deletedAgain, err := DeleteContribution(ctx, *id)
	assert.NoError(suite.T(), err)
	assert.False(suite.T(), deletedAgain)
}
