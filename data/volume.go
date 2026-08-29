package data

import (
	"context"
	"fmt"
	"strings"
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

	return createVolumeVersion(c, id, volume, state, volume.UpdatedBy, time.Now(), nil, nil)
}

// CreateSubmittedVolumeVersion creates a submitted version stamped with a caller-supplied
// submittedBy/submittedAt, for migrating a pre-versioning proposed_changes entry into the
// version model - unlike UpdateVolume, which always stamps "now", this preserves the original
// proposal's submission audit per design.md's Migration Plan.
func CreateSubmittedVolumeVersion(c context.Context, id string, volume *vo.VolumeVO, submittedBy string, submittedAt time.Time) (*vo.VolumeVersionVO, error) {
	return createVolumeVersion(c, id, volume, models.VersionStateSubmitted, submittedBy, submittedAt, nil, nil)
}

// CreateSubmittedVolumeVersionWithStagedAssets is CreateSubmittedVolumeVersion's edit-session
// counterpart (see design.md's "Staged edit-session assets ride on the submitted version as
// separate staged fields") - volume's own CoverAssetId/SampleAssetIds are expected to still be
// the current live values (nothing has been promoted yet); stagedCoverAssetId/
// stagedSampleAssetIds carry the session's not-yet-promoted uploads instead.
func CreateSubmittedVolumeVersionWithStagedAssets(c context.Context, id string, volume *vo.VolumeVO, submittedBy string, stagedCoverAssetId *string, stagedSampleAssetIds []string) (*vo.VolumeVersionVO, error) {
	return createVolumeVersion(c, id, volume, models.VersionStateSubmitted, submittedBy, time.Now(), stagedCoverAssetId, stagedSampleAssetIds)
}

func createVolumeVersion(c context.Context, id string, volume *vo.VolumeVO, state models.VersionState, submittedBy string, submittedAt time.Time, stagedCoverAssetId *string, stagedSampleAssetIds []string) (*vo.VolumeVersionVO, error) {
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
	newVersion.StagedCoverAssetId = stagedCoverAssetId
	newVersion.StagedSampleAssetIds = stagedSampleAssetIds

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

// SoftDeleteVolume sets the volume meta record's deleted_at/deleted_by, hiding it from
// QueryVolumes without touching its version history - Volume's own non-generic path mirroring
// entityVersioningConfig.softDelete, since Volume isn't on the shared engine (see design.md).
func SoftDeleteVolume(c context.Context, id string, deletedBy string) error {
	now := time.Now()
	_, err := database.Db.Collection(volumeMetaCollection).UpdateOne(
		c,
		bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "deleted_at", Value: now}, {Key: "deleted_by", Value: deletedBy}}}},
	)
	return err
}

// RestoreVolume clears a soft-deleted volume's deletion, returning it to QueryVolumes.
func RestoreVolume(c context.Context, id string) error {
	_, err := database.Db.Collection(volumeMetaCollection).UpdateOne(
		c,
		bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "deleted_at", Value: nil}, {Key: "deleted_by", Value: nil}}}},
	)
	return err
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

	return flattenVolume(c, meta, version, nil), nil
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

// resolveVolumeRelations resolves a volume's relationship IDs into their VOs. systemsMap, when
// non-nil, is used as a pre-fetched id->system lookup instead of one game-systems-api call per
// id - callers resolving many volumes at once (QueryVolumes) build it once via GetSystemsMap and
// share it across every volume; callers resolving a single volume pass nil and fall back to
// GetSystem's per-id call, which is cheap enough at that scale.
func resolveVolumeRelations(c context.Context, systemIds, publisherIds, studioIds, licenseIds []string, systemsMap map[string]*vo.SystemVO) (
	systems []*vo.SystemVO, publishers []*vo.PublisherVO, studios []*vo.StudioVO, licenses []*vo.LicenseVO,
) {
	systems = make([]*vo.SystemVO, 0, len(systemIds))
	for _, id := range systemIds {
		if systemsMap != nil {
			if system, ok := systemsMap[id]; ok {
				systems = append(systems, system)
			}
			continue
		}
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
func flattenVolume(c context.Context, meta *models.VolumeMeta, version *models.VolumeVersion, systemsMap map[string]*vo.SystemVO) *vo.VolumeVO {
	systems, publishers, studios, licenses := resolveVolumeRelations(c, version.SystemIds, version.PublisherIds, version.StudioIds, version.LicenseIds, systemsMap)

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
	systems, publishers, studios, licenses := resolveVolumeRelations(c, version.SystemIds, version.PublisherIds, version.StudioIds, version.LicenseIds, nil)

	return &vo.VolumeVersionVO{
		ID:                   version.ID,
		RecordID:             version.RecordID,
		Version:              version.Version,
		Title:                version.Title,
		Description:          version.Description,
		Notes:                version.Notes,
		Format:               version.Format,
		CoverAssetId:         version.CoverAssetId,
		SampleAssetIds:       version.SampleAssetIds,
		StagedCoverAssetId:   version.StagedCoverAssetId,
		StagedSampleAssetIds: version.StagedSampleAssetIds,
		Systems:              systems,
		Publishers:           publishers,
		Studios:              studios,
		Licenses:             licenses,
		Properties:           modelcoreutil.FromPropertyModels(version.Properties),
		Tags:                 modelcoreutil.FromTagModels(version.Tags),
		State:                vo.VersionState(version.State),
		BaseVersion:          version.BaseVersion,
		SubmittedBy:          version.SubmittedBy,
		SubmittedAt:          version.SubmittedAt,
		ReviewedBy:           version.ReviewedBy,
		ReviewedAt:           version.ReviewedAt,
		ReviewNote:           version.ReviewNote,
		ResultingVersion:     version.ResultingVersion,
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
	if len(sort) == 0 {
		// Without an explicit sort, Mongo returns natural (insertion) order - stable for an
		// untouched record, but an edit re-inserts that record's new live version, so it jumps
		// to the back of natural order and can fall outside a caller's default page size. A
		// browse list shouldn't reorder itself just because one record was edited.
		sort = bson.D{{Key: "title", Value: 1}}
	}
	logging.Logger.Debug("query volumes", "filter", filter, "sort", sort, "projection", projection)

	versions, err := database.Query[models.VolumeVersion](volumeVersionCollection, filter, sort, projection, params.Start, params.Limit)
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Volumes: %+v", err))
		return nil, err
	}

	// Resolve every volume's system reference against one shared map instead of one
	// game-systems-api call per volume - see GetSystemsMap. A page of volumes was measured
	// taking 10+ seconds under the old per-volume GetSystem calls.
	systemsMap, err := GetSystemsMap(c)
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while fetching systems map for Volumes: %+v", err))
		systemsMap = map[string]*vo.SystemVO{}
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
		if meta.DeletedAt != nil {
			continue
		}
		vos = append(vos, flattenVolume(c, meta, version, systemsMap))
	}

	logging.Logger.Debug("returning volume value objects", "vos", vos)
	return vos, nil
}

// volumeSearchScanLimit is far smaller than the shared searchScanLimit (5000, used by
// SearchPublishers/SearchPersons/etc.) - QueryVolumes resolves each volume's full relation set
// (systems, publishers, studios, licenses), so scanning 5000 volumes measured at 6-7s against a
// warm dev catalog, blowing past game-room-web's 5s request timeout. 500 keeps this well under
// that budget; raise only alongside a real fix (pushing the title match down to a Mongo query
// instead of an in-memory scan over fully-hydrated volumes).
const volumeSearchScanLimit = 500

// SearchVolumes finds live volumes whose title contains query (case-insensitive), scanning up to
// volumeSearchScanLimit volumes - see the comment there for why this doesn't use the shared
// searchScanLimit other entities' Search* functions use.
func SearchVolumes(c context.Context, query string) ([]*vo.VolumeVO, error) {
	all, err := QueryVolumes(c, apiutil.QueryParams{Limit: volumeSearchScanLimit})
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(query)
	matches := make([]*vo.VolumeVO, 0, len(all))
	for _, v := range all {
		if strings.Contains(strings.ToLower(v.Title), needle) {
			matches = append(matches, v)
		}
	}
	return matches, nil
}

// CatalogStats is a small aggregate over the live volume set - the total count and the most
// recent submission time - for callers that need a catalog-wide summary (e.g. a landing page)
// without paginating through every volume themselves.
type CatalogStats struct {
	VolumeCount int
	LastUpdated *time.Time
}

// GetCatalogStats computes CatalogStats over every live volume version. Uses an unlimited
// database.Query (limit 0) rather than a driver-level CountDocuments/aggregate, matching this
// package's existing pattern of going through database.Query rather than reaching past it to
// the raw mongo-driver collection - catalog sizes here are small enough (an indie/hobby
// catalog, not a high-volume one) that this isn't a real cost.
func GetCatalogStats(c context.Context) (*CatalogStats, error) {
	logging.Logger.Info("GetCatalogStats", "c", c)

	span := tracing.BuildSpanWithParams(c, "volumes", "db-get-catalog-stats", apiutil.QueryParams{})
	defer span.End()

	filter := bson.D{{Key: "state", Value: string(models.VersionStateLive)}}
	projection := bson.D{{Key: "submitted_at", Value: 1}}

	versions, err := database.Query[models.VolumeVersion](volumeVersionCollection, filter, nil, projection, 0, 0)
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for catalog stats: %+v", err))
		return nil, err
	}

	stats := &CatalogStats{VolumeCount: len(versions)}
	for _, version := range versions {
		if stats.LastUpdated == nil || version.SubmittedAt.After(*stats.LastUpdated) {
			submittedAt := version.SubmittedAt
			stats.LastUpdated = &submittedAt
		}
	}

	logging.Logger.Debug("returning catalog stats", "stats", stats)
	return stats, nil
}

// GetVolumeTypeStats returns the volumes landing-page-summary card's count/most-recent data -
// TypeStats-shaped like the other five entity types' Get*Stats (catalog-landing-page-summary),
// but implemented directly against volumeVersionCollection rather than entityVersioningConfig
// since Volume isn't on that shared engine (see entity_versioning.go's own doc comment).
func GetVolumeTypeStats(c context.Context) (*TypeStats, error) {
	span := tracing.BuildSpanWithParams(c, "volumes", "db-get-volume-type-stats", apiutil.QueryParams{})
	defer span.End()

	filter := bson.D{{Key: "state", Value: string(models.VersionStateLive)}}

	countProjection := bson.D{{Key: "record_id", Value: 1}}
	all, err := database.Query[models.VolumeVersion](volumeVersionCollection, filter, nil, countProjection, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("volume: count live versions: %w", err)
	}

	stats := &TypeStats{Count: len(all)}
	if stats.Count == 0 {
		return stats, nil
	}

	sortOrder := bson.D{{Key: "submitted_at", Value: -1}}
	recent, err := database.Query[models.VolumeVersion](volumeVersionCollection, filter, sortOrder, nil, 0, 1)
	if err != nil {
		return nil, fmt.Errorf("volume: query most recent version: %w", err)
	}
	if len(recent) == 0 {
		return stats, nil
	}

	submittedAt := recent[0].SubmittedAt
	stats.LastUpdated = &submittedAt
	stats.MostRecentID = recent[0].RecordID
	stats.MostRecentName = recent[0].Title
	return stats, nil
}
