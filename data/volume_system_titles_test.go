package data

import (
	"github.com/stretchr/testify/assert"
	apiutil "github.com/sweetrpg/api-core.go/util"
	"github.com/sweetrpg/catalog-objects.go/models"
	"github.com/sweetrpg/catalog-objects.go/vo"
)

func systemRefs(ids ...string) []*vo.SystemVO {
	refs := make([]*vo.SystemVO, len(ids))
	for i, id := range ids {
		refs[i] = &vo.SystemVO{ID: id}
	}
	return refs
}

// 3.2: create with titles -> the stored map and the read VO carry them.
func (suite *VolumeDataTestSuite) TestAddVolumeStoresSystemTitles() {
	t := suite.T()
	id, err := AddVolume(t.Context(), &vo.VolumeVO{
		Title:        "Titled Systems",
		Systems:      systemRefs("sysA", "sysB"),
		SystemTitles: map[string]string{"sysA": "Numenera", "sysB": "The Strange"},
	})
	assert.NoError(t, err)

	v, err := GetVolume(t.Context(), *id)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"sysA": "Numenera", "sysB": "The Strange"}, v.SystemTitles)
	names := map[string]string{}
	for _, s := range v.Systems {
		names[s.ID] = s.GameSystem
	}
	assert.Equal(t, "Numenera", names["sysA"])
	assert.Equal(t, "The Strange", names["sysB"])
}

// 3.2: an empty title for a referenced system stores no map entry (renders the ID).
func (suite *VolumeDataTestSuite) TestAddVolumeEmptyTitleNoMapEntry() {
	t := suite.T()
	id, err := AddVolume(t.Context(), &vo.VolumeVO{
		Title:        "Empty Title",
		Systems:      systemRefs("sysA"),
		SystemTitles: map[string]string{"sysA": ""},
	})
	assert.NoError(t, err)

	v, err := GetVolume(t.Context(), *id)
	assert.NoError(t, err)
	assert.Empty(t, v.SystemTitles["sysA"])
	assert.Equal(t, "sysA", v.Systems[0].GameSystem, "absent title falls back to the system ID")
}

// 3.2: editing to swap a system reference drops the removed system's stored title.
func (suite *VolumeDataTestSuite) TestUpdateVolumeSwapsSystemTitleKeys() {
	t := suite.T()
	id, err := AddVolume(t.Context(), &vo.VolumeVO{
		Title:        "Swap Systems",
		Systems:      systemRefs("sysA"),
		SystemTitles: map[string]string{"sysA": "Numenera"},
	})
	assert.NoError(t, err)

	_, err = UpdateVolume(t.Context(), *id, &vo.VolumeVO{
		Title:        "Swap Systems",
		Systems:      systemRefs("sysB"),
		SystemTitles: map[string]string{"sysB": "The Strange"},
	}, models.VersionStateLive)
	assert.NoError(t, err)

	v, err := GetVolume(t.Context(), *id)
	assert.NoError(t, err)
	_, hadOld := v.SystemTitles["sysA"]
	assert.False(t, hadOld, "removed system's title key should be gone")
	assert.Equal(t, "The Strange", v.SystemTitles["sysB"])
}

// 3.4: sets the title on every live volume referencing the system, leaves others untouched,
// returns the affected record IDs, and is a no-op on a second identical call.
func (suite *VolumeDataTestSuite) TestUpdateVolumeSystemTitleBySystem() {
	t := suite.T()
	a, err := AddVolume(t.Context(), &vo.VolumeVO{Title: "Refs sysX #1", Systems: systemRefs("sysX"), SystemTitles: map[string]string{"sysX": "Old"}})
	assert.NoError(t, err)
	b, err := AddVolume(t.Context(), &vo.VolumeVO{Title: "Refs sysX #2", Systems: systemRefs("sysX"), SystemTitles: map[string]string{"sysX": "Old"}})
	assert.NoError(t, err)
	c, err := AddVolume(t.Context(), &vo.VolumeVO{Title: "Refs sysY", Systems: systemRefs("sysY"), SystemTitles: map[string]string{"sysY": "Other"}})
	assert.NoError(t, err)

	changed, err := UpdateVolumeSystemTitleBySystem(t.Context(), "sysX", "New Name")
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{*a, *b}, changed)

	va, _ := GetVolume(t.Context(), *a)
	vb, _ := GetVolume(t.Context(), *b)
	vc, _ := GetVolume(t.Context(), *c)
	assert.Equal(t, "New Name", va.SystemTitles["sysX"])
	assert.Equal(t, "New Name", vb.SystemTitles["sysX"])
	assert.Equal(t, "Other", vc.SystemTitles["sysY"], "unrelated volume untouched")

	again, err := UpdateVolumeSystemTitleBySystem(t.Context(), "sysX", "New Name")
	assert.NoError(t, err)
	assert.Empty(t, again, "redelivered event with the same title is a no-op")
}

// 3.4: no matching volumes -> nothing changed, no error.
func (suite *VolumeDataTestSuite) TestUpdateVolumeSystemTitleBySystemNoMatches() {
	t := suite.T()
	changed, err := UpdateVolumeSystemTitleBySystem(t.Context(), "system-nobody-references", "Whatever")
	assert.NoError(t, err)
	assert.Empty(t, changed)
}

// 3.4: a system ID containing "." is skipped (MongoDB forbids "." in field names).
func (suite *VolumeDataTestSuite) TestUpdateVolumeSystemTitleBySystemDottedIDSkipped() {
	t := suite.T()
	id, err := AddVolume(t.Context(), &vo.VolumeVO{Title: "Dotted", Systems: systemRefs("a.b"), SystemTitles: map[string]string{}})
	assert.NoError(t, err)

	changed, err := UpdateVolumeSystemTitleBySystem(t.Context(), "a.b", "Nope")
	assert.NoError(t, err)
	assert.Empty(t, changed)

	v, _ := GetVolume(t.Context(), *id)
	assert.Empty(t, v.SystemTitles["a.b"])
}

// 3.3: a volume list read renders system names from stored titles with no game-systems-api
// client configured (GameSystemsClient stays nil in tests).
func (suite *VolumeDataTestSuite) TestQueryVolumesUsesStoredSystemTitles() {
	t := suite.T()
	_, err := AddVolume(t.Context(), &vo.VolumeVO{
		Title:        "In The List",
		Systems:      systemRefs("sysL"),
		SystemTitles: map[string]string{"sysL": "Listed System"},
	})
	assert.NoError(t, err)

	results, err := QueryVolumes(t.Context(), apiutil.QueryParams{Limit: 200})
	assert.NoError(t, err)
	var seen bool
	for _, v := range results {
		if v.Title == "In The List" {
			seen = true
			assert.Equal(t, "Listed System", v.Systems[0].GameSystem)
		}
	}
	assert.True(t, seen, "the seeded volume should be in the list")
}
