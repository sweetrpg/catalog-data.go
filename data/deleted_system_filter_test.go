package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/sweetrpg/catalog-objects.go/models"
)

func TestSameFieldValueTreatsNilAndEmptyAlike(t *testing.T) {
	assert.True(t, sameFieldValue([]string(nil), []string{}))
	assert.True(t, sameFieldValue(map[string]string(nil), map[string]string{}))
	assert.False(t, sameFieldValue([]string{"a"}, []string{}))
	assert.False(t, sameFieldValue("a", "b"))
	assert.True(t, sameFieldValue(nil, nil))
}

func TestStripSystemsLeavesTheOriginalAlone(t *testing.T) {
	original := models.VolumeVersion{SystemIds: []string{"dead", "ok"}, SystemTitles: map[string]string{"dead": "D", "ok": "O"}}
	copied := original

	assert.True(t, stripSystems(&copied, map[string]bool{"dead": true}))
	assert.False(t, stripSystems(&copied, map[string]bool{"dead": true}))

	assert.Equal(t, []string{"ok"}, copied.SystemIds)
	assert.Equal(t, map[string]string{"ok": "O"}, copied.SystemTitles)
	assert.Equal(t, []string{"dead", "ok"}, original.SystemIds)
	assert.Len(t, original.SystemTitles, 2)
}

func TestIsFieldNameSafe(t *testing.T) {
	assert.True(t, isFieldNameSafe("dnd-5e"))
	for _, ref := range []string{"", "a.b", "$x"} {
		assert.False(t, isFieldNameSafe(ref), ref)
	}
}
