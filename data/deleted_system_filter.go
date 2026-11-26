package data

import (
	"context"
	"reflect"
	"slices"

	"github.com/sweetrpg/catalog-objects.go/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func indexOn(field string) mongo.IndexModel {
	return mongo.IndexModel{Keys: bson.D{{Key: field, Value: 1}}}
}

// stripSystems removes the dead references and their titles from v, without modifying the slice
// or map v started with. It reports whether anything changed.
func stripSystems(v *models.VolumeVersion, dead map[string]bool) bool {
	if !slices.ContainsFunc(v.SystemIds, func(id string) bool { return dead[id] }) {
		return false
	}
	kept := make([]string, 0, len(v.SystemIds))
	for _, id := range v.SystemIds {
		if !dead[id] {
			kept = append(kept, id)
		}
	}
	v.SystemIds = kept
	v.SystemTitles = pruneSystemTitles(v.SystemTitles, kept)
	return true
}

// filterDeletedSystems strips every tombstoned system from the given versions. It reports
// whether the first one changed.
func filterDeletedSystems(c context.Context, versions ...*models.VolumeVersion) (bool, error) {
	var refs []string
	for _, v := range versions {
		refs = append(refs, v.SystemIds...)
	}
	dead, err := deletedAmong(c, refs)
	if err != nil {
		return false, err
	}
	firstChanged := false
	for i, v := range versions {
		if stripSystems(v, dead) && i == 0 {
			firstChanged = true
		}
	}
	return firstChanged, nil
}

// sameFieldValue is reflect.DeepEqual, except that nil and empty slices or maps are equal:
// removing a version's last system must not look like a change against one that never had any.
func sameFieldValue(a, b any) bool {
	av, bv := reflect.ValueOf(a), reflect.ValueOf(b)
	if av.IsValid() && bv.IsValid() && av.Kind() == bv.Kind() &&
		(av.Kind() == reflect.Slice || av.Kind() == reflect.Map) && av.Len() == 0 && bv.Len() == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}
