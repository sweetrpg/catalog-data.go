package data

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sweetrpg/catalog-objects.go/models"
	"github.com/sweetrpg/catalog-objects.go/vo"
	modelcore "github.com/sweetrpg/model-core.go/models"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// newSystem returns identifiers unique to one test: the database is shared across tests, and
// tombstones are permanent.
func newSystem() DeletedSystem {
	id := primitive.NewObjectID().Hex()
	return DeletedSystem{ID: id, SystemID: "slug-" + id}
}

func (suite *VolumeDataTestSuite) volumeWith(systems ...string) string {
	titles := map[string]string{}
	for _, id := range systems {
		titles[id] = "Title " + id
	}
	id, err := AddVolume(suite.T().Context(), &vo.VolumeVO{Title: "V", Systems: systemRefs(systems...), SystemTitles: titles})
	require.NoError(suite.T(), err)
	return *id
}

func (suite *VolumeDataTestSuite) record(system DeletedSystem) {
	require.NoError(suite.T(), RecordDeletedSystem(suite.T().Context(), system, modelcore.SystemActor))
}

func (suite *VolumeDataTestSuite) remove(system DeletedSystem) SystemRemoval {
	result, err := RemoveDeletedSystemReferences(suite.T().Context(), system, modelcore.SystemActor)
	require.NoError(suite.T(), err)
	return result
}

func (suite *VolumeDataTestSuite) versions(recordID string) map[int]*models.VolumeVersion {
	found, err := database.Query[models.VolumeVersion](volumeVersionCollection, bson.D{{Key: "record_id", Value: recordID}}, nil, nil, 0, 0)
	require.NoError(suite.T(), err)
	byNumber := map[int]*models.VolumeVersion{}
	for _, v := range found {
		byNumber[v.Version] = v
	}
	return byNumber
}

// assertOneLiveCurrent checks the current version is the only live one and has exactly systems.
func (suite *VolumeDataTestSuite) assertOneLiveCurrent(recordID string, systems ...string) {
	t := suite.T()
	meta, err := getVolumeMeta(t.Context(), recordID)
	require.NoError(t, err)
	for number, v := range suite.versions(recordID) {
		assert.Equal(t, number == meta.CurrentVersion, v.State == models.VersionStateLive, "version %d state %s", number, v.State)
	}
	assert.ElementsMatch(t, systems, suite.versions(recordID)[meta.CurrentVersion].SystemIds)
}

func (suite *VolumeDataTestSuite) TestRecordDeletedSystemKeepsFirstRecord() {
	t := suite.T()
	system := newSystem()
	require.NoError(t, RecordDeletedSystem(t.Context(), system, "first"))
	require.NoError(t, RecordDeletedSystem(t.Context(), system, "second"))

	all, err := ListDeletedSystems(t.Context())
	require.NoError(t, err)
	var matches []DeletedSystem
	for _, s := range all {
		if s.ID == system.ID {
			matches = append(matches, s)
		}
	}
	require.Len(t, matches, 1)
	assert.Equal(t, "first", matches[0].CreatedBy)
	assert.Error(t, RecordDeletedSystem(t.Context(), DeletedSystem{}, "x"))
}

func (suite *VolumeDataTestSuite) TestRemoveReplacesLiveVersionByIDAndSlug() {
	t := suite.T()
	for _, bySlug := range []bool{false, true} {
		system, other := newSystem(), newSystem()
		ref := system.ID
		if bySlug {
			ref = system.SystemID
		}
		id := suite.volumeWith(ref, other.ID)
		unrelated := suite.volumeWith(other.ID)
		suite.record(system)

		result := suite.remove(system)

		assert.Equal(t, []ChangedVolume{{RecordID: id, Title: "V", Version: 2}}, result.Changed)
		suite.assertOneLiveCurrent(id, other.ID)
		all := suite.versions(id)
		assert.ElementsMatch(t, []string{ref, other.ID}, all[1].SystemIds, "history unchanged")
		assert.Equal(t, map[string]string{other.ID: "Title " + other.ID}, all[2].SystemTitles)
		assert.Equal(t, modelcore.SystemActor, all[2].SubmittedBy)
		assert.Equal(t, 1, *all[2].BaseVersion)
		meta, err := getVolumeMeta(t.Context(), id)
		require.NoError(t, err)
		assert.Equal(t, modelcore.SystemActor, meta.UpdatedBy)
		assert.Len(t, suite.versions(unrelated), 1)

		again := suite.remove(system)
		assert.Empty(t, again.Changed, "redelivery")
		assert.Len(t, suite.versions(id), 2, "redelivery creates nothing")
	}
}

func (suite *VolumeDataTestSuite) TestRemoveEditsPendingInPlace() {
	t := suite.T()
	system, added := newSystem(), newSystem()
	id := suite.volumeWith()
	pending, err := UpdateVolume(t.Context(), id, &vo.VolumeVO{
		Title: "Proposed", Systems: systemRefs(system.ID, added.ID),
		SystemTitles: map[string]string{system.ID: "Gone", added.ID: "Added"},
	}, models.VersionStateSubmitted)
	require.NoError(t, err)
	suite.record(system)

	result := suite.remove(system)

	assert.Empty(t, result.Changed)
	assert.Equal(t, []string{id}, result.PendingRecordIDs)
	stored := suite.versions(id)[pending.Version]
	assert.Equal(t, []string{added.ID}, stored.SystemIds)
	assert.Equal(t, map[string]string{added.ID: "Added"}, stored.SystemTitles)
	assert.Equal(t, "Proposed", stored.Title)
	assert.Equal(t, models.VersionStateSubmitted, stored.State)
}

func (suite *VolumeDataTestSuite) TestRemoveCoversSoftDeletedVolume() {
	t := suite.T()
	system := newSystem()
	id := suite.volumeWith(system.ID)
	require.NoError(t, SoftDeleteVolume(t.Context(), id, "someone"))
	suite.record(system)

	suite.remove(system)

	suite.assertOneLiveCurrent(id)
	meta, err := getVolumeMeta(t.Context(), id)
	require.NoError(t, err)
	assert.NotNil(t, meta.DeletedAt, "still soft deleted")
}

// Each case leaves the database as a run that died at that step would.
func (suite *VolumeDataTestSuite) TestRemoveCompletesAnInterruptedRun() {
	t := suite.T()
	cases := map[string]struct {
		movePointer, markLive bool
		announced             int
	}{
		"after insert":       {announced: 1},
		"after pointer move": {movePointer: true, announced: 1},
		"after mark live":    {movePointer: true, markLive: true},
	}
	for name, step := range cases {
		system := newSystem()
		id := suite.volumeWith(system.ID)
		next := *suite.versions(id)[1]
		asDerivedVersion(&next, 2, 1, modelcore.SystemActor)
		next.SystemIds, next.SystemTitles = []string{}, nil
		_, err := database.Db.Collection(volumeVersionCollection).InsertOne(t.Context(), next)
		require.NoError(t, err, name)
		if step.movePointer {
			require.NoError(t, setVolumeMetaCurrentVersion(t.Context(), id, 2, modelcore.SystemActor), name)
		}
		if step.markLive {
			require.NoError(t, markVolumeVersionLive(t.Context(), id, 2), name)
		}
		suite.record(system)

		result := suite.remove(system)

		assert.Len(t, result.Changed, step.announced, name)
		suite.assertOneLiveCurrent(id)
	}
}

func (suite *VolumeDataTestSuite) TestReplaceYieldsToConcurrentEdit() {
	t := suite.T()
	id := suite.volumeWith()
	stale := *suite.versions(id)[1]
	_, err := UpdateVolume(t.Context(), id, &vo.VolumeVO{Title: "Edited"}, models.VersionStateLive)
	require.NoError(t, err)

	done, err := replaceCurrentVersion(t.Context(), &stale, 1, modelcore.SystemActor)

	require.NoError(t, err)
	assert.False(t, done)
	assert.Len(t, suite.versions(id), 2, "losing copy removed")
	current, err := GetVolume(t.Context(), id)
	require.NoError(t, err)
	assert.Equal(t, "Edited", current.Title)
}

func (suite *VolumeDataTestSuite) TestWritesDropTombstonedSystems() {
	t := suite.T()
	system, kept := newSystem(), newSystem()
	suite.record(system)

	created := suite.volumeWith(system.ID, kept.ID)
	suite.assertOneLiveCurrent(created, kept.ID)

	for _, state := range []models.VersionState{models.VersionStateLive, models.VersionStateSubmitted} {
		v, err := UpdateVolume(t.Context(), created, &vo.VolumeVO{Title: "V", Systems: systemRefs(system.SystemID, kept.ID)}, state)
		require.NoError(t, err)
		assert.Equal(t, []string{kept.ID}, suite.versions(created)[v.Version].SystemIds, state)
	}
}

// A submission made before the deletion must not report a conflict caused only by cleanup.
func (suite *VolumeDataTestSuite) TestAcceptAfterRemovalHasNoFalseConflict() {
	t := suite.T()
	cases := map[string]bool{"submission changes systems": true, "submission leaves systems": false}
	for name, changesSystems := range cases {
		system, added := newSystem(), newSystem()
		id := suite.volumeWith(system.ID)
		proposed := []string{system.ID}
		if changesSystems {
			proposed = append(proposed, added.ID)
		}
		pending, err := UpdateVolume(t.Context(), id, &vo.VolumeVO{Title: "New title", Systems: systemRefs(proposed...)}, models.VersionStateSubmitted)
		require.NoError(t, err, name)
		suite.record(system)
		suite.remove(system)

		accepted, conflicts, err := AcceptVolumeVersion(t.Context(), id, pending.Version, nil, "reviewer", nil, nil, nil)

		require.NoError(t, err, name)
		assert.NotContains(t, conflicts, "system_ids", name)
		assert.Equal(t, "New title", accepted.Title, name)
		want := []string{}
		if changesSystems {
			want = []string{added.ID}
		}
		suite.assertOneLiveCurrent(id, want...)
	}
}

// The tombstone is recorded but cleanup hasn't reached the pending submission yet.
func (suite *VolumeDataTestSuite) TestCleanAcceptFiltersPromotedSubmission() {
	t := suite.T()
	system, added := newSystem(), newSystem()
	id := suite.volumeWith()
	pending, err := UpdateVolume(t.Context(), id, &vo.VolumeVO{Title: "V", Systems: systemRefs(system.ID, added.ID)}, models.VersionStateSubmitted)
	require.NoError(t, err)
	suite.record(system)

	_, _, err = AcceptVolumeVersion(t.Context(), id, pending.Version, nil, "reviewer", nil, nil, nil)

	require.NoError(t, err)
	suite.assertOneLiveCurrent(id, added.ID)
}

func (suite *VolumeDataTestSuite) TestSetCurrentCopiesRatherThanRestoringDeletedSystem() {
	t := suite.T()
	system, kept := newSystem(), newSystem()
	id := suite.volumeWith(system.ID, kept.ID)
	suite.record(system)
	suite.remove(system)

	result, err := SetCurrentVolumeVersion(t.Context(), id, 1, "editor")

	require.NoError(t, err)
	assert.Equal(t, 3, result.Version)
	suite.assertOneLiveCurrent(id, kept.ID)
	assert.ElementsMatch(t, []string{system.ID, kept.ID}, suite.versions(id)[1].SystemIds)
}
