package data

import (
	"context"
	"fmt"
	"time"

	"github.com/sweetrpg/api-core.go/tracing"
	apiutil "github.com/sweetrpg/api-core.go/util"
	"github.com/sweetrpg/catalog-objects.go/models"
	"github.com/sweetrpg/catalog-objects.go/vo"
	"github.com/sweetrpg/common.go/logging"
	modelcoreutil "github.com/sweetrpg/model-core.go/util"
	modelcorevo "github.com/sweetrpg/model-core.go/vo"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// AddVolume creates a volume's meta record and its first (live) version.
func AddVolume(c context.Context, volume *vo.VolumeVO) (*string, error) {
	logging.Logger.Info("AddVolume", "c", c, "volume", volume)

	_, span := otel.Tracer("volume").Start(c, "db-add-volume", oteltrace.WithAttributes())
	defer span.End()

	now := time.Now()
	metaID := primitive.NewObjectID().Hex()
	meta := models.VolumeMeta{
		ID:             metaID,
		CurrentVersion: 1,
		CreatedAt:      now,
		CreatedBy:      volume.CreatedBy,
	}
	if _, err := database.Insert[models.VolumeMeta](volumeMetaCollection, meta); err != nil {
		logging.Logger.Error("Error while inserting VolumeMeta object", "error", err)
		return nil, err
	}

	version := volumeVOToVersionFields(volume)
	version.ID = primitive.NewObjectID().Hex()
	version.RecordID = metaID
	version.Version = 1
	version.State = models.VersionStateLive
	version.BaseVersion = nil
	version.SubmittedBy = volume.CreatedBy
	version.SubmittedAt = now
	if _, err := database.Insert[models.VolumeVersion](volumeVersionCollection, version); err != nil {
		logging.Logger.Error("Error while inserting VolumeVersion object", "error", err)
		return nil, err
	}

	return &metaID, nil
}

// UpdateVolume creates a new version of the volume rather than mutating the record in place.
// For state VersionStateLive, the new version becomes current immediately and the previously
// current version is archived; any other state (VersionStateSubmitted) leaves the current
// pointer untouched. Returns the created version, or nil if the record doesn't exist.
func UpdateVolume(c context.Context, id string, volume *vo.VolumeVO, state models.VersionState) (*vo.VolumeVersionVO, error) {
	logging.Logger.Info("UpdateVolume", "c", c, "id", id, "volume", volume, "state", state)

	_, span := otel.Tracer("volume").Start(c, "db-update-volume", oteltrace.WithAttributes(attribute.String("id", id)))
	defer span.End()

	return createVolumeVersion(c, id, volume, state, volume.UpdatedBy, time.Now())
}

// CreateSubmittedVolumeVersion creates a submitted version stamped with a caller-supplied
// submittedBy/submittedAt, for migrating a pre-versioning proposed_changes entry into the
// version model - unlike UpdateVolume, which always stamps "now", this preserves the original
// proposal's submission audit per design.md's Migration Plan.
func CreateSubmittedVolumeVersion(c context.Context, id string, volume *vo.VolumeVO, submittedBy string, submittedAt time.Time) (*vo.VolumeVersionVO, error) {
	return createVolumeVersion(c, id, volume, models.VersionStateSubmitted, submittedBy, submittedAt)
}

func createVolumeVersion(c context.Context, id string, volume *vo.VolumeVO, state models.VersionState, submittedBy string, submittedAt time.Time) (*vo.VolumeVersionVO, error) {
	meta, err := getVolumeMeta(c, id)
	if err != nil {
		logging.Logger.Error("Error while looking up VolumeMeta for update", "id", id, "error", err)
		return nil, err
	}
	if meta == nil {
		logging.Logger.Info(fmt.Sprintf("Volume not found for update, ID: %s", id))
		return nil, nil
	}

	nextVersion, err := nextVolumeVersionNumber(c, id)
	if err != nil {
		return nil, err
	}

	baseVersion := meta.CurrentVersion
	newVersion := volumeVOToVersionFields(volume)
	newVersion.ID = primitive.NewObjectID().Hex()
	newVersion.RecordID = id
	newVersion.Version = nextVersion
	newVersion.State = state
	newVersion.BaseVersion = &baseVersion
	newVersion.SubmittedBy = submittedBy
	newVersion.SubmittedAt = submittedAt

	if _, err := database.Insert[models.VolumeVersion](volumeVersionCollection, newVersion); err != nil {
		logging.Logger.Error("Error while inserting VolumeVersion object", "error", err)
		return nil, err
	}

	if state == models.VersionStateLive {
		if err := archiveVolumeVersion(c, id, meta.CurrentVersion); err != nil {
			return nil, err
		}
		if err := setVolumeMetaCurrentVersion(c, id, nextVersion); err != nil {
			return nil, err
		}
	}

	return volumeVersionModelToVO(c, &newVersion), nil
}

func DeleteVolume(c context.Context, id string) error {
	_, span := otel.Tracer("volume").Start(c, "db-delete-volume", oteltrace.WithAttributes(attribute.String("id", id)))

	// TODO

	span.End()

	return nil
}

// GetVolume returns the flattened view of a volume - its meta record merged with its current
// version's data - matching the shape this function returned before meta/version were split.
func GetVolume(c context.Context, id string) (*vo.VolumeVO, error) {
	_, span := otel.Tracer("volume").Start(c, "db-get-volume", oteltrace.WithAttributes(attribute.String("id", id)))
	defer span.End()

	meta, err := getVolumeMeta(c, id)
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for VolumeMeta: %+v", err))
		return nil, err
	}
	if meta == nil {
		logging.Logger.Info(fmt.Sprintf("Volume not found for ID: %s", id))
		return nil, nil
	}

	version, err := getVolumeVersion(c, id, meta.CurrentVersion)
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for VolumeVersion: %+v", err))
		return nil, err
	}
	if version == nil {
		err := fmt.Errorf("volume %s: meta points at missing current version %d", id, meta.CurrentVersion)
		logging.Logger.Error(err.Error())
		return nil, err
	}

	return flattenVolume(c, meta, version), nil
}

// relationIDs extracts each element's ID from a pointer-slice relationship field - the
// counterpart to the *ByIDs functions below, which resolve IDs back into VOs. Both directions
// live here rather than using util.Map (which always dereferences into a value slice) since
// VolumeVO's relationship fields are pointer slices - see that struct's own doc comment for why.
func relationIDs[T any](relations []*T, id func(*T) string) []string {
	ids := make([]string, 0, len(relations))
	for _, r := range relations {
		if r != nil {
			ids = append(ids, id(r))
		}
	}
	return ids
}

func resolveVolumeRelations(c context.Context, systemIds, publisherIds, studioIds, licenseIds []string) (
	systems []*vo.SystemVO, publishers []*vo.PublisherVO, studios []*vo.StudioVO, licenses []*vo.LicenseVO,
) {
	systems = make([]*vo.SystemVO, 0, len(systemIds))
	for _, id := range systemIds {
		system, err := GetSystem(c, id)
		if err != nil {
			logging.Logger.Error(fmt.Sprintf("No System found from Volume for ID %s: %s", id, err.Error()))
			continue
		}
		if system != nil {
			systems = append(systems, system)
		}
	}
	publishers = make([]*vo.PublisherVO, 0, len(publisherIds))
	for _, id := range publisherIds {
		publisher, err := GetPublisher(c, id)
		if err != nil {
			logging.Logger.Error(fmt.Sprintf("No Publisher found from Volume for ID %s: %s", id, err.Error()))
			continue
		}
		if publisher != nil {
			publishers = append(publishers, publisher)
		}
	}
	studios = make([]*vo.StudioVO, 0, len(studioIds))
	for _, id := range studioIds {
		studio, err := GetStudio(c, id)
		if err != nil {
			logging.Logger.Error(fmt.Sprintf("No Studio found from Volume for ID %s: %s", id, err.Error()))
			continue
		}
		if studio != nil {
			studios = append(studios, studio)
		}
	}
	licenses = make([]*vo.LicenseVO, 0, len(licenseIds))
	for _, id := range licenseIds {
		license, err := GetLicense(c, id)
		if err != nil {
			logging.Logger.Error(fmt.Sprintf("No License found from Volume for ID %s: %s", id, err.Error()))
			continue
		}
		if license != nil {
			licenses = append(licenses, license)
		}
	}
	return
}

// flattenVolume merges a meta record with its current version into the pre-versioning VolumeVO
// shape: creation/deletion audit comes from meta, the "last updated" audit comes from the
// version's own submission audit (its most recent live edit).
func flattenVolume(c context.Context, meta *models.VolumeMeta, version *models.VolumeVersion) *vo.VolumeVO {
	systems, publishers, studios, licenses := resolveVolumeRelations(c, version.SystemIds, version.PublisherIds, version.StudioIds, version.LicenseIds)

	return &vo.VolumeVO{
		ID:             meta.ID,
		Title:          version.Title,
		Description:    version.Description,
		Notes:          version.Notes,
		Format:         version.Format,
		CoverAssetId:   version.CoverAssetId,
		SampleAssetIds: version.SampleAssetIds,
		Systems:        systems,
		Publishers:     publishers,
		Studios:        studios,
		Licenses:       licenses,
		Properties:     modelcoreutil.FromPropertyModels(version.Properties),
		Tags:           modelcoreutil.FromTagModels(version.Tags),
		AuditableVO: modelcorevo.AuditableVO{
			CreatedAt: meta.CreatedAt,
			CreatedBy: meta.CreatedBy,
			UpdatedAt: version.SubmittedAt,
			UpdatedBy: version.SubmittedBy,
			DeletedAt: meta.DeletedAt,
			DeletedBy: meta.DeletedBy,
		},
	}
}

// volumeVersionModelToVO converts a version record on its own (no meta) into the version-history
// API shape, resolving its relationship IDs the same way a flattened read does.
func volumeVersionModelToVO(c context.Context, version *models.VolumeVersion) *vo.VolumeVersionVO {
	systems, publishers, studios, licenses := resolveVolumeRelations(c, version.SystemIds, version.PublisherIds, version.StudioIds, version.LicenseIds)

	return &vo.VolumeVersionVO{
		ID:               version.ID,
		RecordID:         version.RecordID,
		Version:          version.Version,
		Title:            version.Title,
		Description:      version.Description,
		Notes:            version.Notes,
		Format:           version.Format,
		CoverAssetId:     version.CoverAssetId,
		SampleAssetIds:   version.SampleAssetIds,
		Systems:          systems,
		Publishers:       publishers,
		Studios:          studios,
		Licenses:         licenses,
		Properties:       modelcoreutil.FromPropertyModels(version.Properties),
		Tags:             modelcoreutil.FromTagModels(version.Tags),
		State:            vo.VersionState(version.State),
		BaseVersion:      version.BaseVersion,
		SubmittedBy:      version.SubmittedBy,
		SubmittedAt:      version.SubmittedAt,
		ReviewedBy:       version.ReviewedBy,
		ReviewedAt:       version.ReviewedAt,
		ReviewNote:       version.ReviewNote,
		ResultingVersion: version.ResultingVersion,
	}
}

// volumeVOToVersionFields maps a request VolumeVO's substantive fields onto a new
// models.VolumeVersion - the caller fills in the version-lifecycle fields (ID, RecordID,
// Version, State, BaseVersion, SubmittedBy/At).
func volumeVOToVersionFields(volume *vo.VolumeVO) models.VolumeVersion {
	return models.VolumeVersion{
		Title:          volume.Title,
		Description:    volume.Description,
		Notes:          volume.Notes,
		Format:         volume.Format,
		CoverAssetId:   volume.CoverAssetId,
		SampleAssetIds: volume.SampleAssetIds,
		Properties:     modelcoreutil.ToPropertyModels(volume.Properties),
		Tags:           modelcoreutil.ToTagModels(volume.Tags),
		SystemIds:      relationIDs(volume.Systems, func(s *vo.SystemVO) string { return s.ID }),
		PublisherIds:   relationIDs(volume.Publishers, func(p *vo.PublisherVO) string { return p.ID }),
		StudioIds:      relationIDs(volume.Studios, func(s *vo.StudioVO) string { return s.ID }),
		LicenseIds:     relationIDs(volume.Licenses, func(l *vo.LicenseVO) string { return l.ID }),
	}
}

// QueryVolumes lists the current (live) version of every volume matching params - the live
// version's data is exactly today's flat volume shape, since exactly one version per record is
// ever live at a time.
func QueryVolumes(c context.Context, params apiutil.QueryParams) ([]*vo.VolumeVO, error) {
	logging.Logger.Info("QueryVolumes", "c", c, "params", params)

	span := tracing.BuildSpanWithParams(c, "volumes", "db-get-volumes", params)
	defer span.End()

	filter, sort, projection := apiutil.ConvertQueryParams(params)
	filter = append(filter, bson.E{Key: "state", Value: string(models.VersionStateLive)})
	logging.Logger.Debug("query volumes", "filter", filter, "sort", sort, "projection", projection)

	versions, err := database.Query[models.VolumeVersion](volumeVersionCollection, filter, sort, projection, params.Start, params.Limit)
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Volumes: %+v", err))
		return nil, err
	}

	vos := make([]*vo.VolumeVO, 0, len(versions))
	for _, version := range versions {
		meta, err := getVolumeMeta(c, version.RecordID)
		if err != nil {
			logging.Logger.Error(fmt.Sprintf("No VolumeMeta found for VolumeVersion record %s: %s", version.RecordID, err.Error()))
			continue
		}
		if meta == nil {
			logging.Logger.Error(fmt.Sprintf("No VolumeMeta found for VolumeVersion record %s", version.RecordID))
			continue
		}
		vos = append(vos, flattenVolume(c, meta, version))
	}

	logging.Logger.Debug("returning volume value objects", "vos", vos)
	return vos, nil
}
