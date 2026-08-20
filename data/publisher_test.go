package data

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	apiutil "github.com/sweetrpg/api-core.go/util"
	"github.com/sweetrpg/catalog-objects.go/models"
	"github.com/sweetrpg/catalog-objects.go/vo"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/mongodb.go/constants"
	"github.com/sweetrpg/mongodb.go/database"
)

type PublisherDataTestSuite struct {
	suite.Suite
	seedPublisherID string
}

func (suite *PublisherDataTestSuite) SetupTest() {
	_ = os.Setenv(constants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()
	assert.NoError(suite.T(), EnsurePublisherVersioningIndexes(suite.T().Context()))

	id, err := AddPublisher(suite.T().Context(), &vo.PublisherVO{
		Name: "Test Publisher", Address: "123 Test St",
	})
	assert.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), id)
	suite.seedPublisherID = *id
}

func (suite *PublisherDataTestSuite) TestUpdatePublisherLive() {
	updated, err := UpdatePublisher(suite.T().Context(), suite.seedPublisherID, &vo.PublisherVO{
		Name: "Updated Publisher", Address: "456 Updated Ave",
	}, models.VersionStateLive)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), updated)
	assert.Equal(suite.T(), "Updated Publisher", updated.Name)
	assert.Equal(suite.T(), string(models.VersionStateLive), string(updated.State))

	fetched, err := GetPublisher(suite.T().Context(), suite.seedPublisherID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), fetched)
	assert.Equal(suite.T(), "Updated Publisher", fetched.Name)
}

func (suite *PublisherDataTestSuite) TestUpdatePublisherSubmittedLeavesCurrentVersionUnchanged() {
	submitted, err := UpdatePublisher(suite.T().Context(), suite.seedPublisherID, &vo.PublisherVO{
		Name: "Proposed Publisher",
	}, models.VersionStateSubmitted)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), submitted)
	assert.Equal(suite.T(), string(models.VersionStateSubmitted), string(submitted.State))

	fetched, err := GetPublisher(suite.T().Context(), suite.seedPublisherID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Test Publisher", fetched.Name)
}

func (suite *PublisherDataTestSuite) TestUpdatePublisherNotFound() {
	updated, err := UpdatePublisher(suite.T().Context(), "does-not-exist", &vo.PublisherVO{
		Name: "Doesn't Matter",
	}, models.VersionStateLive)
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), updated)
}

func (suite *PublisherDataTestSuite) TestAcceptPublisherVersionFullAccept() {
	submitted, err := UpdatePublisher(suite.T().Context(), suite.seedPublisherID, &vo.PublisherVO{
		Name: "Proposed Publisher",
	}, models.VersionStateSubmitted)
	assert.NoError(suite.T(), err)

	accepted, conflicts, err := AcceptPublisherVersion(
		suite.T().Context(), suite.seedPublisherID, submitted.Version, nil, "editor-1", nil)
	assert.NoError(suite.T(), err)
	assert.Empty(suite.T(), conflicts)
	assert.Equal(suite.T(), string(models.VersionStateLive), string(accepted.State))

	fetched, err := GetPublisher(suite.T().Context(), suite.seedPublisherID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Proposed Publisher", fetched.Name)

	// Re-fetch the accepted version from Mongo (not the in-memory struct AcceptPublisherVersion
	// already mutated) - guards against VersionLifecycle's embedded bson fields silently landing
	// under a nested "versionlifecycle" subdocument instead of being inlined, which would let a
	// $set-based state transition appear to succeed in-memory while never actually persisting.
	refetched, err := GetPublisherVersion(suite.T().Context(), suite.seedPublisherID, submitted.Version)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), string(models.VersionStateLive), string(refetched.State))
}

func (suite *PublisherDataTestSuite) TestRejectPublisherVersion() {
	submitted, err := UpdatePublisher(suite.T().Context(), suite.seedPublisherID, &vo.PublisherVO{
		Name: "Proposed Publisher",
	}, models.VersionStateSubmitted)
	assert.NoError(suite.T(), err)

	note := "not good"
	err = RejectPublisherVersion(suite.T().Context(), suite.seedPublisherID, submitted.Version, "editor-1", &note)
	assert.NoError(suite.T(), err)

	fetched, err := GetPublisher(suite.T().Context(), suite.seedPublisherID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Test Publisher", fetched.Name)

	refetched, err := GetPublisherVersion(suite.T().Context(), suite.seedPublisherID, submitted.Version)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), string(models.VersionStateRejected), string(refetched.State))
	assert.Equal(suite.T(), &note, refetched.ReviewNote)
}

func (suite *PublisherDataTestSuite) TestRetractPublisherVersion() {
	proposed := &vo.PublisherVO{Name: "Proposed Publisher"}
	proposed.UpdatedBy = "submitter-1"
	submitted, err := UpdatePublisher(suite.T().Context(), suite.seedPublisherID, proposed,
		models.VersionStateSubmitted)
	assert.NoError(suite.T(), err)

	retracted, err := RetractPublisherVersion(suite.T().Context(), suite.seedPublisherID, submitted.Version, "submitter-1")
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), string(models.VersionStateWithdrawn), string(retracted.State))

	refetched, err := GetPublisherVersion(suite.T().Context(), suite.seedPublisherID, submitted.Version)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), string(models.VersionStateWithdrawn), string(refetched.State))
}

func (suite *PublisherDataTestSuite) TestSetCurrentPublisherVersion() {
	_, err := UpdatePublisher(suite.T().Context(), suite.seedPublisherID, &vo.PublisherVO{
		Name: "Updated Publisher",
	}, models.VersionStateLive)
	assert.NoError(suite.T(), err)

	rolledBack, err := SetCurrentPublisherVersion(suite.T().Context(), suite.seedPublisherID, 1)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Test Publisher", rolledBack.Name)

	fetched, err := GetPublisher(suite.T().Context(), suite.seedPublisherID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Test Publisher", fetched.Name)

	refetched, err := GetPublisherVersion(suite.T().Context(), suite.seedPublisherID, 1)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), string(models.VersionStateLive), string(refetched.State))
}

func (suite *PublisherDataTestSuite) TestWebsiteRoundTripsAsPlainString() {
	updated, err := UpdatePublisher(suite.T().Context(), suite.seedPublisherID, &vo.PublisherVO{
		Name: "Test Publisher", Website: "https://example.com/kobold",
	}, models.VersionStateLive)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), updated)
	assert.Equal(suite.T(), "https://example.com/kobold", updated.Website)

	fetched, err := GetPublisher(suite.T().Context(), suite.seedPublisherID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), fetched)
	assert.Equal(suite.T(), "https://example.com/kobold", fetched.Website)
}

// TestSoftDeletePublisherLifecycle exercises entityVersioningConfig's shared soft-delete engine -
// publisher, studio, person, license, and system all route through the identical generic code,
// so this one test covers that engine; Volume gets its own test since its path is bespoke.
func (suite *PublisherDataTestSuite) TestSoftDeletePublisherLifecycle() {
	err := SoftDeletePublisher(suite.T().Context(), suite.seedPublisherID, "admin-1")
	assert.NoError(suite.T(), err)

	results, err := QueryPublishers(suite.T().Context(), apiutil.QueryParams{Limit: 100})
	assert.NoError(suite.T(), err)
	for _, p := range results {
		assert.NotEqual(suite.T(), suite.seedPublisherID, p.ID, "deleted publisher should be excluded from QueryPublishers")
	}

	fetched, err := GetPublisher(suite.T().Context(), suite.seedPublisherID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), fetched, "GetPublisher must still return a soft-deleted record")
	assert.NotNil(suite.T(), fetched.DeletedAt)
	assert.Equal(suite.T(), "admin-1", *fetched.DeletedBy)

	err = RestorePublisher(suite.T().Context(), suite.seedPublisherID)
	assert.NoError(suite.T(), err)

	restored, err := GetPublisher(suite.T().Context(), suite.seedPublisherID)
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), restored.DeletedAt)

	results, err = QueryPublishers(suite.T().Context(), apiutil.QueryParams{Limit: 100})
	assert.NoError(suite.T(), err)
	found := false
	for _, p := range results {
		if p.ID == suite.seedPublisherID {
			found = true
		}
	}
	assert.True(suite.T(), found, "restored publisher should reappear in QueryPublishers")
}

func TestPublisherDbTestSuite(t *testing.T) {
	suite.Run(t, new(PublisherDataTestSuite))
}
