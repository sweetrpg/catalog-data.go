package data

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/sweetrpg/catalog-objects.go/models"
	"github.com/sweetrpg/catalog-objects.go/vo"
	"github.com/sweetrpg/common.go/logging"
	modelcore "github.com/sweetrpg/model-core.go/models"
	"github.com/sweetrpg/mongodb.go/constants"
	"github.com/sweetrpg/mongodb.go/database"
)

type VolumeMigrationTestSuite struct {
	suite.Suite
}

func (suite *VolumeMigrationTestSuite) SetupTest() {
	_ = os.Setenv(constants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()
	assert.NoError(suite.T(), EnsureVolumeVersioningIndexes(suite.T().Context()))
}

func (suite *VolumeMigrationTestSuite) TestMigrateVolumesBackfillsMetaAndLiveVersion() {
	legacy := models.Volume{
		ID:          "legacy-volume-1",
		Title:       "Legacy Volume",
		Description: "Predates versioning",
		Auditable: modelcore.Auditable{
			CreatedAt: time.Now().Add(-48 * time.Hour),
			CreatedBy: "auth0|original-creator",
			UpdatedAt: time.Now().Add(-24 * time.Hour),
			UpdatedBy: "auth0|last-editor",
		},
	}
	_, err := database.Insert[models.Volume]("volumes", legacy)
	assert.NoError(suite.T(), err)

	migrated, err := MigrateVolumes(suite.T().Context())
	assert.NoError(suite.T(), err)
	assert.GreaterOrEqual(suite.T(), migrated, 1)

	fetched, err := GetVolume(suite.T().Context(), legacy.ID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), fetched)
	assert.Equal(suite.T(), "Legacy Volume", fetched.Title)
	assert.Equal(suite.T(), "auth0|original-creator", fetched.CreatedBy)

	version, err := GetVolumeVersion(suite.T().Context(), legacy.ID, 1)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), version)
	assert.Equal(suite.T(), models.VersionStateLive, models.VersionState(version.State))
	assert.Nil(suite.T(), version.BaseVersion)
}

func (suite *VolumeMigrationTestSuite) TestMigrateVolumesIsIdempotent() {
	legacy := models.Volume{ID: "legacy-volume-2", Title: "Legacy Volume Two"}
	_, err := database.Insert[models.Volume]("volumes", legacy)
	assert.NoError(suite.T(), err)

	first, err := MigrateVolumes(suite.T().Context())
	assert.NoError(suite.T(), err)
	assert.GreaterOrEqual(suite.T(), first, 1)

	second, err := MigrateVolumes(suite.T().Context())
	assert.NoError(suite.T(), err)

	versions, err := ListVolumeVersions(suite.T().Context(), legacy.ID)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), versions, 1, "re-running the migration must not create a second version")
	_ = second
}

func (suite *VolumeMigrationTestSuite) TestCreateSubmittedVolumeVersionPreservesHistoricalSubmission() {
	id, err := AddVolume(suite.T().Context(), &vo.VolumeVO{Title: "Live Title"})
	assert.NoError(suite.T(), err)

	historicalSubmittedAt := time.Now().Add(-72 * time.Hour)
	version, err := CreateSubmittedVolumeVersion(
		suite.T().Context(), *id, &vo.VolumeVO{Title: "Proposed From Legacy System"},
		"auth0|legacy-submitter", historicalSubmittedAt)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), version)
	assert.Equal(suite.T(), models.VersionStateSubmitted, models.VersionState(version.State))
	assert.Equal(suite.T(), "auth0|legacy-submitter", version.SubmittedBy)
	assert.WithinDuration(suite.T(), historicalSubmittedAt, version.SubmittedAt, time.Second)

	fetched, err := GetVolume(suite.T().Context(), *id)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Live Title", fetched.Title, "a submitted version must not move the live record")
}

func TestVolumeMigrationTestSuite(t *testing.T) {
	suite.Run(t, new(VolumeMigrationTestSuite))
}
