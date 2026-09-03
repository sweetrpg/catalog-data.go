package data

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/sweetrpg/catalog-objects.go/models"
	"github.com/sweetrpg/catalog-objects.go/vo"
	"github.com/sweetrpg/common.go/logging"
	modelcore "github.com/sweetrpg/model-core.go/models"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	volumeMetaCollection    = "volumes_meta"
	volumeVersionCollection = "volumes_versions"
)

// EnsureVolumeVersioningIndexes creates the indexes volume version queries rely on: a unique
// (record_id, version) index so a version number can never be reused for a record, and a
// (record_id, state) index for the pending-submission lookup. Safe to call on every startup.
func EnsureVolumeVersioningIndexes(ctx context.Context) error {
	_, err := database.Db.Collection(volumeVersionCollection).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "record_id", Value: 1}, {Key: "version", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("volumes: create record_id+version index: %w", err)
	}

	_, err = database.Db.Collection(volumeVersionCollection).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "record_id", Value: 1}, {Key: "state", Value: 1}},
	})
	if err != nil {
		return fmt.Errorf("volumes: create record_id+state index: %w", err)
	}

	return nil
}

func getVolumeMeta(c context.Context, id string) (*models.VolumeMeta, error) {
	results, err := database.Query[models.VolumeMeta](volumeMetaCollection, bson.D{{Key: "_id", Value: id}}, nil, nil, 0, 1)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

func getVolumeVersion(c context.Context, recordID string, version int) (*models.VolumeVersion, error) {
	filter := bson.D{{Key: "record_id", Value: recordID}, {Key: "version", Value: version}}
	results, err := database.Query[models.VolumeVersion](volumeVersionCollection, filter, nil, nil, 0, 1)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

func nextVolumeVersionNumber(c context.Context, recordID string) (int, error) {
	filter := bson.D{{Key: "record_id", Value: recordID}}
	sortOrder := bson.D{{Key: "version", Value: -1}}
	results, err := database.Query[models.VolumeVersion](volumeVersionCollection, filter, sortOrder, nil, 0, 1)
	if err != nil {
		return 0, err
	}
	if len(results) == 0 {
		return 1, nil
	}
	return results[0].Version + 1, nil
}

func setVolumeVersionState(c context.Context, recordID string, version int, fields bson.D) error {
	filter := bson.D{{Key: "record_id", Value: recordID}, {Key: "version", Value: version}}
	_, err := database.Db.Collection(volumeVersionCollection).UpdateOne(c, filter, bson.D{{Key: "$set", Value: fields}})
	return err
}

func archiveVolumeVersion(c context.Context, recordID string, version int) error {
	return setVolumeVersionState(c, recordID, version, bson.D{{Key: "state", Value: string(models.VersionStateArchived)}})
}

func setVolumeMetaCurrentVersion(c context.Context, recordID string, version int, actingUserID string) error {
	_, err := database.Db.Collection(volumeMetaCollection).UpdateOne(
		c,
		bson.D{{Key: "_id", Value: recordID}},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "current_version", Value: version},
			{Key: "updated_at", Value: time.Now()},
			{Key: "updated_by", Value: actingUserID},
		}}},
	)
	return err
}

// ListVolumeVersions returns every version of a volume, newest first.
func ListVolumeVersions(c context.Context, id string) ([]*vo.VolumeVersionVO, error) {
	filter := bson.D{{Key: "record_id", Value: id}}
	sortOrder := bson.D{{Key: "version", Value: -1}}
	versions, err := database.Query[models.VolumeVersion](volumeVersionCollection, filter, sortOrder, nil, 0, 0)
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for VolumeVersions: %+v", err))
		return nil, err
	}

	vos := make([]*vo.VolumeVersionVO, 0, len(versions))
	for _, version := range versions {
		vos = append(vos, volumeVersionModelToVO(c, version))
	}
	return vos, nil
}

// GetVolumeVersion returns one version's full snapshot, regardless of whether it's current.
func GetVolumeVersion(c context.Context, id string, version int) (*vo.VolumeVersionVO, error) {
	result, err := getVolumeVersion(c, id, version)
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for VolumeVersion: %+v", err))
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return volumeVersionModelToVO(c, result), nil
}

// volumeVersionSubstantiveFields are the fields a submission can change and a review can
// selectively accept - everything on VolumeVersion except its version-lifecycle bookkeeping.
var volumeVersionSubstantiveFields = []string{
	"title", "description", "notes", "format", "cover_asset_id", "sample_asset_ids",
	"system_ids", "system_titles", "publisher_ids", "studio_ids", "license_ids", "properties", "tags",
}

func volumeVersionFieldValue(v *models.VolumeVersion, field string) any {
	switch field {
	case "title":
		return v.Title
	case "description":
		return v.Description
	case "notes":
		return v.Notes
	case "format":
		return v.Format
	case "cover_asset_id":
		return v.CoverAssetId
	case "sample_asset_ids":
		return v.SampleAssetIds
	case "system_ids":
		return v.SystemIds
	case "system_titles":
		return v.SystemTitles
	case "publisher_ids":
		return v.PublisherIds
	case "studio_ids":
		return v.StudioIds
	case "license_ids":
		return v.LicenseIds
	case "properties":
		return v.Properties
	case "tags":
		return v.Tags
	default:
		return nil
	}
}

func setVolumeVersionFieldValue(v *models.VolumeVersion, field string, value any) {
	switch field {
	case "title":
		v.Title = value.(string)
	case "description":
		v.Description = value.(string)
	case "notes":
		v.Notes = value.(string)
	case "format":
		v.Format = value.(string)
	case "cover_asset_id":
		v.CoverAssetId = value.(string)
	case "sample_asset_ids":
		v.SampleAssetIds = value.([]string)
	case "system_ids":
		v.SystemIds = value.([]string)
	case "system_titles":
		v.SystemTitles, _ = value.(map[string]string)
	case "publisher_ids":
		v.PublisherIds = value.([]string)
	case "studio_ids":
		v.StudioIds = value.([]string)
	case "license_ids":
		v.LicenseIds = value.([]string)
	case "properties":
		v.Properties = value.([]modelcore.Property)
	case "tags":
		v.Tags = value.([]modelcore.Tag)
	}
}

// volumeVersionChangedFields returns the substantive fields where submitted differs from base.
func volumeVersionChangedFields(submitted, base *models.VolumeVersion) []string {
	var changed []string
	for _, field := range volumeVersionSubstantiveFields {
		if !reflect.DeepEqual(volumeVersionFieldValue(submitted, field), volumeVersionFieldValue(base, field)) {
			changed = append(changed, field)
		}
	}
	return changed
}

func intersectFields(fields, selected []string) []string {
	selectedSet := make(map[string]bool, len(selected))
	for _, f := range selected {
		selectedSet[f] = true
	}
	var out []string
	for _, f := range fields {
		if selectedSet[f] {
			out = append(out, f)
		}
	}
	return out
}

// AcceptVolumeVersion reviews a submitted version. selectedFields == nil accepts every field the
// submission changed (full accept); a non-nil slice accepts only that subset. If the submission's
// baseVersion no longer matches the record's actual current version, a field being accepted whose
// actual-current value has itself diverged from baseVersion is excluded as a conflict rather than
// applied - reported back via the returned conflicts slice.
//
// A clean full accept (no drift, no conflicts) promotes the submitted version itself to live. Any
// other outcome (a selected-fields accept, or a full accept with conflicts excluded) derives a new
// version from the actual current version with only the accepted fields overlaid, promotes that
// derived version to live, and marks the submitted version partially_accepted, referencing it.
//
// liveCoverAssetId/liveSampleAssetIds are an optional override (nil/nil for the common case) -
// see design.md's "Staged edit-session assets ride on the submitted version as separate staged
// fields": when the submitted version being accepted carries staged (unpromoted) assets, the
// caller promotes them via the asset store first and passes the resulting live ids here, so the
// version that goes live carries live ids rather than the version's own (still-unpromoted)
// coverAssetId/sampleAssetIds.
func AcceptVolumeVersion(c context.Context, id string, version int, selectedFields []string, reviewedBy string, reviewNote *string, liveCoverAssetId *string, liveSampleAssetIds []string) (*vo.VolumeVersionVO, []string, error) {
	submitted, err := getVolumeVersion(c, id, version)
	if err != nil {
		return nil, nil, err
	}
	if submitted == nil {
		return nil, nil, fmt.Errorf("volume %s: version %d not found", id, version)
	}
	if submitted.State != models.VersionStateSubmitted {
		return nil, nil, fmt.Errorf("volume %s: version %d is not submitted (state: %s)", id, version, submitted.State)
	}
	if submitted.BaseVersion == nil {
		return nil, nil, fmt.Errorf("volume %s: version %d has no base version to review against", id, version)
	}

	meta, err := getVolumeMeta(c, id)
	if err != nil {
		return nil, nil, err
	}
	if meta == nil {
		return nil, nil, fmt.Errorf("volume %s: meta record not found", id)
	}

	current, err := getVolumeVersion(c, id, meta.CurrentVersion)
	if err != nil {
		return nil, nil, err
	}
	if current == nil {
		return nil, nil, fmt.Errorf("volume %s: current version %d not found", id, meta.CurrentVersion)
	}

	now := time.Now()

	if selectedFields == nil && *submitted.BaseVersion == meta.CurrentVersion {
		if err := archiveVolumeVersion(c, id, meta.CurrentVersion); err != nil {
			return nil, nil, err
		}
		submitted.State = models.VersionStateLive
		submitted.ReviewedBy = &reviewedBy
		submitted.ReviewedAt = &now
		submitted.ReviewNote = reviewNote
		update := bson.D{
			{Key: "state", Value: string(models.VersionStateLive)},
			{Key: "reviewed_by", Value: reviewedBy},
			{Key: "reviewed_at", Value: now},
			{Key: "review_note", Value: reviewNote},
			{Key: "staged_cover_asset_id", Value: nil},
			{Key: "staged_sample_asset_ids", Value: nil},
		}
		submitted.StagedCoverAssetId = nil
		submitted.StagedSampleAssetIds = nil
		if liveCoverAssetId != nil {
			update = append(update, bson.E{Key: "cover_asset_id", Value: *liveCoverAssetId})
			submitted.CoverAssetId = *liveCoverAssetId
		}
		if liveSampleAssetIds != nil {
			update = append(update, bson.E{Key: "sample_asset_ids", Value: liveSampleAssetIds})
			submitted.SampleAssetIds = liveSampleAssetIds
		}
		if err := setVolumeVersionState(c, id, version, update); err != nil {
			return nil, nil, err
		}
		if err := setVolumeMetaCurrentVersion(c, id, version, reviewedBy); err != nil {
			return nil, nil, err
		}
		return volumeVersionModelToVO(c, submitted), nil, nil
	}

	baseVersion, err := getVolumeVersion(c, id, *submitted.BaseVersion)
	if err != nil {
		return nil, nil, err
	}
	if baseVersion == nil {
		return nil, nil, fmt.Errorf("volume %s: base version %d not found", id, *submitted.BaseVersion)
	}

	changed := volumeVersionChangedFields(submitted, baseVersion)
	target := changed
	if selectedFields != nil {
		target = intersectFields(changed, selectedFields)
	}
	sort.Strings(target)

	derived := *current
	var conflicts []string
	var accepted []string
	for _, field := range target {
		if !reflect.DeepEqual(volumeVersionFieldValue(current, field), volumeVersionFieldValue(baseVersion, field)) {
			conflicts = append(conflicts, field)
			continue
		}
		setVolumeVersionFieldValue(&derived, field, volumeVersionFieldValue(submitted, field))
		accepted = append(accepted, field)
	}

	nextVersion, err := nextVolumeVersionNumber(c, id)
	if err != nil {
		return nil, nil, err
	}

	derived.ID = primitive.NewObjectID().Hex()
	derived.RecordID = id
	derived.Version = nextVersion
	derived.State = models.VersionStateLive
	derived.BaseVersion = &meta.CurrentVersion
	derived.SubmittedBy = submitted.SubmittedBy
	derived.SubmittedAt = now
	derived.ReviewedBy = &reviewedBy
	derived.ReviewedAt = &now
	derived.ReviewNote = reviewNote
	derived.ResultingVersion = nil
	derived.StagedCoverAssetId = nil
	derived.StagedSampleAssetIds = nil
	if liveCoverAssetId != nil {
		derived.CoverAssetId = *liveCoverAssetId
	}
	if liveSampleAssetIds != nil {
		derived.SampleAssetIds = liveSampleAssetIds
	}

	if _, err := database.Insert[models.VolumeVersion](volumeVersionCollection, derived); err != nil {
		return nil, nil, err
	}
	if err := archiveVolumeVersion(c, id, meta.CurrentVersion); err != nil {
		return nil, nil, err
	}
	if err := setVolumeMetaCurrentVersion(c, id, nextVersion, reviewedBy); err != nil {
		return nil, nil, err
	}
	if err := setVolumeVersionState(c, id, version, bson.D{
		{Key: "state", Value: string(models.VersionStatePartiallyAccepted)},
		{Key: "reviewed_by", Value: reviewedBy},
		{Key: "reviewed_at", Value: now},
		{Key: "review_note", Value: reviewNote},
		{Key: "resulting_version", Value: nextVersion},
	}); err != nil {
		return nil, nil, err
	}

	logging.Logger.Info("AcceptVolumeVersion: derived version", "id", id, "version", nextVersion, "accepted", accepted, "conflicts", conflicts)

	return volumeVersionModelToVO(c, &derived), conflicts, nil
}

// CountSubmittedVolumeVersionsBySubmitter counts a submitter's currently-pending (state:
// submitted) volume versions - the version-model replacement for
// proposedchanges.CountPendingBySubmitter, used by submissioncap's cap check.
func CountSubmittedVolumeVersionsBySubmitter(c context.Context, submittedBy string) (int64, error) {
	filter := bson.D{
		{Key: "submitted_by", Value: submittedBy},
		{Key: "state", Value: string(models.VersionStateSubmitted)},
	}
	return database.Db.Collection(volumeVersionCollection).CountDocuments(c, filter)
}

// PendingStagedAssetIds is the set of staged cover/sample asset ids currently referenced by a
// pending (state: submitted) volume version - used by assets-web's reclaim job to avoid
// deleting a staged file a pending submission still needs.
type PendingStagedAssetIds struct {
	CoverAssetIds  []string `json:"coverAssetIds"`
	SampleAssetIds []string `json:"sampleAssetIds"`
}

// ListPendingStagedAssetIds scans every submitted volume version for a staged cover/sample
// reference. Cheap at current data volumes (small pending-submission counts, matching the
// platform's existing "no new index/search endpoint needed yet" precedent for similarly-sized
// collections) - a full collection scan, not a lookup by id, since the reclaim job needs the
// whole referenced set each run.
func ListPendingStagedAssetIds(c context.Context) (*PendingStagedAssetIds, error) {
	filter := bson.D{{Key: "state", Value: string(models.VersionStateSubmitted)}}
	projection := bson.D{
		{Key: "staged_cover_asset_id", Value: 1},
		{Key: "staged_sample_asset_ids", Value: 1},
	}
	versions, err := database.Query[models.VolumeVersion](volumeVersionCollection, filter, nil, projection, 0, 0)
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for pending staged asset ids: %+v", err))
		return nil, err
	}

	result := &PendingStagedAssetIds{CoverAssetIds: []string{}, SampleAssetIds: []string{}}
	for _, version := range versions {
		if version.StagedCoverAssetId != nil && *version.StagedCoverAssetId != "" {
			result.CoverAssetIds = append(result.CoverAssetIds, *version.StagedCoverAssetId)
		}
		result.SampleAssetIds = append(result.SampleAssetIds, version.StagedSampleAssetIds...)
	}
	return result, nil
}

// RejectVolumeVersion marks a submitted version rejected, with an optional note. The record's
// current-version pointer is unchanged.
func RejectVolumeVersion(c context.Context, id string, version int, reviewedBy string, reviewNote *string) error {
	submitted, err := getVolumeVersion(c, id, version)
	if err != nil {
		return err
	}
	if submitted == nil {
		return fmt.Errorf("volume %s: version %d not found", id, version)
	}
	if submitted.State != models.VersionStateSubmitted {
		return fmt.Errorf("volume %s: version %d is not submitted (state: %s)", id, version, submitted.State)
	}

	now := time.Now()
	return setVolumeVersionState(c, id, version, bson.D{
		{Key: "state", Value: string(models.VersionStateRejected)},
		{Key: "reviewed_by", Value: reviewedBy},
		{Key: "reviewed_at", Value: now},
		{Key: "review_note", Value: reviewNote},
	})
}

// RetractVolumeVersion lets the original submitter withdraw their own pending submission,
// setting its state to withdrawn - see design.md's "Retract/pull-back need a `withdrawn` version
// state". The record's current-version pointer is unchanged. Returns an error if the version
// isn't submitted, or wasn't submitted by submitterID.
func RetractVolumeVersion(c context.Context, id string, version int, submitterID string) (*vo.VolumeVersionVO, error) {
	submitted, err := getVolumeVersion(c, id, version)
	if err != nil {
		return nil, err
	}
	if submitted == nil {
		return nil, fmt.Errorf("volume %s: version %d not found", id, version)
	}
	if submitted.State != models.VersionStateSubmitted {
		return nil, fmt.Errorf("volume %s: version %d is not submitted (state: %s)", id, version, submitted.State)
	}
	if submitted.SubmittedBy != submitterID {
		return nil, fmt.Errorf("volume %s: version %d was not submitted by %s", id, version, submitterID)
	}

	if err := setVolumeVersionState(c, id, version, bson.D{
		{Key: "state", Value: string(models.VersionStateWithdrawn)},
	}); err != nil {
		return nil, err
	}
	submitted.State = models.VersionStateWithdrawn
	return volumeVersionModelToVO(c, submitted), nil
}

// SetCurrentVolumeVersion rolls a record back (or forward) to an arbitrary existing version,
// independent of the submit/review flow - marking that version live and archiving whichever
// version was previously current.
func SetCurrentVolumeVersion(c context.Context, id string, version int, actingUserID string) (*vo.VolumeVersionVO, error) {
	meta, err := getVolumeMeta(c, id)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, nil
	}

	target, err := getVolumeVersion(c, id, version)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, fmt.Errorf("volume %s: version %d not found", id, version)
	}

	if version == meta.CurrentVersion {
		return volumeVersionModelToVO(c, target), nil
	}

	if err := archiveVolumeVersion(c, id, meta.CurrentVersion); err != nil {
		return nil, err
	}
	if err := setVolumeVersionState(c, id, version, bson.D{{Key: "state", Value: string(models.VersionStateLive)}}); err != nil {
		return nil, err
	}
	if err := setVolumeMetaCurrentVersion(c, id, version, actingUserID); err != nil {
		return nil, err
	}

	target.State = models.VersionStateLive
	return volumeVersionModelToVO(c, target), nil
}
