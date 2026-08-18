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

// EntityMigrationTestSuite exercises the generic migrateEntity engine (data/entity_versioning.go)
// via publisher as the vehicle - the same generic-mechanism test rationale as
// server/entity_version_patch_test.go's studio vehicle: all five types (publisher/studio/person/
// license/system) share this engine, so one type end to end covers the shared code path. The
// per-type `Migrate<Type>` wrappers (MigrateStudios etc.) are the only per-type surface left
// untested here.
type EntityMigrationTestSuite struct {
	suite.Suite
}

func (suite *EntityMigrationTestSuite) SetupTest() {
	_ = os.Setenv(constants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()
	assert.NoError(suite.T(), EnsurePublisherVersioningIndexes(suite.T().Context()))
}

func (suite *EntityMigrationTestSuite) TestMigratePublishersBackfillsMetaAndLiveVersion() {
	legacy := models.Publisher{
		ID: "legacy-publisher-1", Name: "Legacy Publisher", Address: "1 Old Row",
		Auditable: modelcore.Auditable{
			CreatedAt: time.Now().Add(-48 * time.Hour), CreatedBy: "auth0|original-creator",
			UpdatedAt: time.Now().Add(-24 * time.Hour), UpdatedBy: "auth0|last-editor",
		},
	}
	_, err := database.Insert[models.Publisher]("publishers", legacy)
	assert.NoError(suite.T(), err)

	migrated, err := MigratePublishers(suite.T().Context())
	assert.NoError(suite.T(), err)
	assert.GreaterOrEqual(suite.T(), migrated, 1)

	fetched, err := GetPublisher(suite.T().Context(), legacy.ID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), fetched)
	assert.Equal(suite.T(), "Legacy Publisher", fetched.Name)
	assert.Equal(suite.T(), "auth0|original-creator", fetched.CreatedBy)

	version, err := GetPublisherVersion(suite.T().Context(), legacy.ID, 1)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), version)
	assert.Equal(suite.T(), models.VersionStateLive, models.VersionState(version.State))
	assert.Nil(suite.T(), version.BaseVersion)
}

func (suite *EntityMigrationTestSuite) TestMigratePublishersIsIdempotent() {
	legacy := models.Publisher{ID: "legacy-publisher-2", Name: "Legacy Publisher Two"}
	_, err := database.Insert[models.Publisher]("publishers", legacy)
	assert.NoError(suite.T(), err)

	first, err := MigratePublishers(suite.T().Context())
	assert.NoError(suite.T(), err)
	assert.GreaterOrEqual(suite.T(), first, 1)

	_, err = MigratePublishers(suite.T().Context())
	assert.NoError(suite.T(), err)

	versions, err := ListPublisherVersions(suite.T().Context(), legacy.ID)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), versions, 1, "re-running the migration must not create a second version")
}

func (suite *EntityMigrationTestSuite) TestCreateSubmittedPublisherVersionPreservesHistoricalSubmission() {
	id, err := AddPublisher(suite.T().Context(), &vo.PublisherVO{Name: "Live Name"})
	assert.NoError(suite.T(), err)

	historicalSubmittedAt := time.Now().Add(-72 * time.Hour)
	version, err := CreateSubmittedPublisherVersion(
		suite.T().Context(), *id, &vo.PublisherVO{Name: "Proposed From Legacy System"},
		"auth0|legacy-submitter", historicalSubmittedAt)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), version)
	assert.Equal(suite.T(), models.VersionStateSubmitted, models.VersionState(version.State))
	assert.Equal(suite.T(), "auth0|legacy-submitter", version.SubmittedBy)
	assert.WithinDuration(suite.T(), historicalSubmittedAt, version.SubmittedAt, time.Second)

	fetched, err := GetPublisher(suite.T().Context(), *id)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Live Name", fetched.Name, "a submitted version must not move the live record")
}

func TestEntityMigrationTestSuite(t *testing.T) {
	suite.Run(t, new(EntityMigrationTestSuite))
}
