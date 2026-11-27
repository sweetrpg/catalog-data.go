package data

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/mongodb.go/constants"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func errOnly[V any](retract func(context.Context, string, int, string) (V, error)) func(context.Context, string, int, string) error {
	return func(c context.Context, id string, version int, caller string) error {
		_, err := retract(c, id, version, caller)
		return err
	}
}

// Another user's version must look exactly like a missing one, whatever its state.
func TestRetractHidesOtherUsersVersions(t *testing.T) {
	_ = os.Setenv(constants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()

	types := map[string]struct {
		collection string
		retract    func(context.Context, string, int, string) error
	}{
		"volume":    {volumeVersionCollection, errOnly(RetractVolumeVersion)},
		"publisher": {publisherVersioning.versionCollection, errOnly(RetractPublisherVersion)},
		"studio":    {studioVersioning.versionCollection, errOnly(RetractStudioVersion)},
		"person":    {personVersioning.versionCollection, errOnly(RetractPersonVersion)},
		"license":   {licenseVersioning.versionCollection, errOnly(RetractLicenseVersion)},
	}
	const missing = 99

	for name, tc := range types {
		t.Run(name, func(t *testing.T) {
			recordID := "retract-" + primitive.NewObjectID().Hex()
			seedReaderVersions(t, tc.collection, recordID)
			absent := tc.retract(t.Context(), recordID, missing, "bob")
			require.ErrorIs(t, absent, ErrVersionNotFound)

			for n := 1; n <= len(readerStates); n++ {
				for _, caller := range []string{"bob", ""} {
					err := tc.retract(t.Context(), recordID, n, caller)
					require.ErrorIs(t, err, ErrVersionNotFound, "version %d caller %q", n, caller)
					// Only the requested version number may differ from the missing case.
					assert.Equal(t, strings.Replace(absent.Error(), "99", "N", 1),
						strings.Replace(err.Error(), "version "+string(rune('0'+n)), "version N", 1))
				}
			}

			// The owner still learns why a non-submitted version can't be retracted.
			err := tc.retract(t.Context(), recordID, 1, "alice")
			require.ErrorContains(t, err, "is not submitted")
			assert.NotErrorIs(t, err, ErrVersionNotFound)
			// Version 3 is the submitted one: untouched by the refusals above.
			assert.NoError(t, tc.retract(t.Context(), recordID, 3, "alice"))
		})
	}
}
