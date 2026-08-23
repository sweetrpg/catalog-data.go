package data

import (
	"github.com/stretchr/testify/assert"
	"github.com/sweetrpg/catalog-objects.go/vo"
)

// These live as methods on VolumeDataTestSuite (defined in volume_test.go) rather than
// standalone Test* funcs so they pick up SetupTest's logging.Init()/database.SetupDatabase()
// call - this package has no package-level TestMain of its own.

func (suite *VolumeDataTestSuite) TestAddContributionAndQueryByVolume() {
	ctx := suite.T().Context()

	personID, err := AddPerson(ctx, &vo.PersonVO{Name: "Test Author"})
	assert.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), personID)

	id, err := AddContribution(ctx, *personID, suite.seedVolumeID, "Author", "auth0|editor")
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), id)
	assert.NotEmpty(suite.T(), *id)

	contributions, err := QueryContributionsByVolume(ctx, suite.seedVolumeID)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), contributions, 1)
	assert.NotNil(suite.T(), contributions[0].Person)
	assert.Equal(suite.T(), *personID, contributions[0].Person.ID)
	assert.Equal(suite.T(), "Author", contributions[0].Role)

	got, err := GetContribution(ctx, *id)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), got)
	assert.Equal(suite.T(), *id, got.ID)
}

func (suite *VolumeDataTestSuite) TestDeleteContribution() {
	ctx := suite.T().Context()

	personID, err := AddPerson(ctx, &vo.PersonVO{Name: "Another Author"})
	assert.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), personID)

	id, err := AddContribution(ctx, *personID, suite.seedVolumeID, "Editor", "auth0|editor")
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
