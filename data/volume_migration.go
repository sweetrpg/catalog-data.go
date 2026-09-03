package data

import (
	"context"

	"github.com/sweetrpg/catalog-objects.go/models"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MigrateVolumes backfills every existing "volumes" document (the pre-versioning flat model)
// into a meta record plus a single live version, per design.md's Migration Plan. Idempotent -
// a record that already has a meta record (recognized by the same ID) is left untouched, so
// this is safe to re-run after a partial failure.
func MigrateVolumes(c context.Context) (int, error) {
	all, err := database.Query[models.Volume]("volumes", bson.D{}, nil, nil, 0, 0)
	if err != nil {
		logging.Logger.Error("MigrateVolumes: query existing volumes", "error", err)
		return 0, err
	}

	migrated := 0
	for _, v := range all {
		existing, err := getVolumeMeta(c, v.ID)
		if err != nil {
			return migrated, err
		}
		if existing != nil {
			continue
		}

		meta := models.VolumeMeta{
			ID:             v.ID,
			CurrentVersion: 1,
			CreatedAt:      v.CreatedAt,
			CreatedBy:      v.CreatedBy,
			UpdatedAt:      v.UpdatedAt,
			UpdatedBy:      v.UpdatedBy,
			DeletedAt:      v.DeletedAt,
			DeletedBy:      v.DeletedBy,
		}
		if _, err := database.Insert[models.VolumeMeta](volumeMetaCollection, meta); err != nil {
			logging.Logger.Error("MigrateVolumes: insert meta", "id", v.ID, "error", err)
			return migrated, err
		}

		version := models.VolumeVersion{
			ID:             primitive.NewObjectID().Hex(),
			RecordID:       v.ID,
			Version:        1,
			Title:          v.Title,
			Description:    v.Description,
			Notes:          v.Notes,
			Format:         v.Format,
			CoverAssetId:   v.CoverAssetId,
			SampleAssetIds: v.SampleAssetIds,
			SystemIds:      v.SystemIds,
			SystemTitles:   v.SystemTitles,
			PublisherIds:   v.PublisherIds,
			StudioIds:      v.StudioIds,
			LicenseIds:     v.LicenseIds,
			Properties:     v.Properties,
			Tags:           v.Tags,
			State:          models.VersionStateLive,
			BaseVersion:    nil,
			SubmittedBy:    v.UpdatedBy,
			SubmittedAt:    v.UpdatedAt,
		}
		if _, err := database.Insert[models.VolumeVersion](volumeVersionCollection, version); err != nil {
			logging.Logger.Error("MigrateVolumes: insert version", "id", v.ID, "error", err)
			return migrated, err
		}

		migrated++
	}

	return migrated, nil
}
