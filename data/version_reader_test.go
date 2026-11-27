package data

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sweetrpg/catalog-objects.go/models"
	"github.com/sweetrpg/catalog-objects.go/vo"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/mongodb.go/constants"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var readerStates = []models.VersionState{
	models.VersionStateArchived, models.VersionStateLive, models.VersionStateSubmitted,
	models.VersionStateRejected, models.VersionStatePartiallyAccepted, models.VersionStateWithdrawn,
}

// seeds one version per state: version n is in readerStates[n-1], all submitted by "alice".
func seedReaderVersions(t *testing.T, collection, recordID string) {
	t.Helper()
	for i, state := range readerStates {
		_, err := database.Db.Collection(collection).InsertOne(t.Context(), bson.M{
			"_id": primitive.NewObjectID().Hex(), "record_id": recordID, "version": i + 1,
			"state": string(state), "submitted_by": "alice", "reviewed_by": "rita",
			"review_note": "note", "staged_cover_asset_id": "staged-cover",
			"staged_sample_asset_ids": []string{"staged-sample"},
		})
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		_, _ = database.Db.Collection(collection).DeleteMany(context.Background(), bson.M{"record_id": recordID})
	})
}

type readVersion struct {
	number    int
	lifecycle vo.VersionLifecycleVO
	staged    bool
}

func entityView(number int, l vo.VersionLifecycleVO) readVersion {
	return readVersion{number: number, lifecycle: l}
}

func volumeView(v *vo.VolumeVersionVO) readVersion {
	return readVersion{
		number: v.Version,
		lifecycle: vo.VersionLifecycleVO{
			SubmittedBy: v.SubmittedBy, ReviewedBy: v.ReviewedBy, ReviewNote: v.ReviewNote,
		},
		staged: v.StagedCoverAssetId != nil || len(v.StagedSampleAssetIds) > 0,
	}
}

type readerType struct {
	collection string
	hasStaged  bool
	list       func(context.Context, string, VersionReader) ([]readVersion, error)
	get        func(context.Context, string, int, VersionReader) (*readVersion, error)
}

func typed[V any](
	collection string, hasStaged bool,
	list func(context.Context, string, VersionReader) ([]*V, error),
	get func(context.Context, string, int, VersionReader) (*V, error),
	view func(*V) readVersion,
) readerType {
	return readerType{
		collection: collection, hasStaged: hasStaged,
		list: func(c context.Context, id string, r VersionReader) ([]readVersion, error) {
			found, err := list(c, id, r)
			out := []readVersion{}
			for _, v := range found {
				out = append(out, view(v))
			}
			return out, err
		},
		get: func(c context.Context, id string, n int, r VersionReader) (*readVersion, error) {
			found, err := get(c, id, n, r)
			if err != nil || found == nil {
				return nil, err
			}
			out := view(found)
			return &out, nil
		},
	}
}

func TestVersionReadsAreScopedToTheReader(t *testing.T) {
	_ = os.Setenv(constants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()

	types := map[string]readerType{
		"volume": typed(volumeVersionCollection, true, ListVolumeVersions, GetVolumeVersion, volumeView),
		"publisher": typed(publisherVersioning.versionCollection, false, ListPublisherVersions, GetPublisherVersion,
			func(v *vo.PublisherVersionVO) readVersion { return entityView(v.Version, v.VersionLifecycleVO) }),
		"studio": typed(studioVersioning.versionCollection, false, ListStudioVersions, GetStudioVersion,
			func(v *vo.StudioVersionVO) readVersion { return entityView(v.Version, v.VersionLifecycleVO) }),
		"person": typed(personVersioning.versionCollection, false, ListPersonVersions, GetPersonVersion,
			func(v *vo.PersonVersionVO) readVersion { return entityView(v.Version, v.VersionLifecycleVO) }),
		"license": typed(licenseVersioning.versionCollection, false, ListLicenseVersions, GetLicenseVersion,
			func(v *vo.LicenseVersionVO) readVersion { return entityView(v.Version, v.VersionLifecycleVO) }),
	}
	readers := map[string]struct {
		reader  VersionReader
		visible []int
	}{
		"signed out":    {VersionReader{}, []int{2, 1}},
		"another user":  {VersionReader{UserID: "bob"}, []int{2, 1}},
		"the submitter": {VersionReader{UserID: "alice"}, []int{6, 5, 4, 3, 2, 1}},
		"a review role": {VersionReader{UserID: "rita", Reviewer: true}, []int{6, 5, 4, 3, 2, 1}},
	}

	for typeName, rt := range types {
		recordID := "reader-" + primitive.NewObjectID().Hex()
		seedReaderVersions(t, rt.collection, recordID)
		for readerName, tc := range readers {
			t.Run(typeName+"/"+readerName, func(t *testing.T) {
				listed, err := rt.list(t.Context(), recordID, tc.reader)
				require.NoError(t, err)
				numbers := []int{}
				for _, v := range listed {
					numbers = append(numbers, v.number)
					assertRedaction(t, v, tc.reader, rt.hasStaged)
				}
				assert.Equal(t, tc.visible, numbers)

				for n := 1; n <= len(readerStates); n++ {
					got, err := rt.get(t.Context(), recordID, n, tc.reader)
					require.NoError(t, err)
					if !assert.Equal(t, contains(tc.visible, n), got != nil, "version %d", n) || got == nil {
						continue
					}
					assertRedaction(t, *got, tc.reader, rt.hasStaged)
				}
			})
		}
	}
}

func assertRedaction(t *testing.T, v readVersion, r VersionReader, hasStaged bool) {
	t.Helper()
	own := r.UserID == "alice"
	assert.Equal(t, r.Reviewer, v.lifecycle.ReviewedBy != nil, "reviewedBy")
	assert.Equal(t, r.Reviewer || own, v.lifecycle.SubmittedBy != "", "submittedBy")
	assert.Equal(t, r.Reviewer || own, v.lifecycle.ReviewNote != nil, "reviewNote")
	if hasStaged {
		assert.Equal(t, r.Reviewer || own, v.staged, "staged assets")
	}
}

func contains(numbers []int, n int) bool {
	for _, v := range numbers {
		if v == n {
			return true
		}
	}
	return false
}

func TestSignedOutReaderOwnsNothing(t *testing.T) {
	assert.False(t, VersionReader{}.owns(""))
}
