package data

import (
	"context"
	"net/url"
	"time"

	apiutil "github.com/sweetrpg/api-core.go/util"
	"github.com/sweetrpg/catalog-objects.go/models"
	"github.com/sweetrpg/catalog-objects.go/vo"
	modelcore "github.com/sweetrpg/model-core.go/models"
	modelcoreutil "github.com/sweetrpg/model-core.go/util"
	modelcorevo "github.com/sweetrpg/model-core.go/vo"
	"github.com/sweetrpg/mongodb.go/database"
)

const (
	studioMetaCollection    = "studios_meta"
	studioVersionCollection = "studios_versions"
)

var studioVersioning = entityVersioningConfig[models.StudioVersion]{
	metaCollection:    studioMetaCollection,
	versionCollection: studioVersionCollection,
	typeName:          "studio",
	lifecycle:         func(v *models.StudioVersion) *models.VersionLifecycle { return &v.VersionLifecycle },
	setID:             func(v *models.StudioVersion, id string) { v.ID = id },
	setRecordID:       func(v *models.StudioVersion, id string) { v.RecordID = id },
	setVersion:        func(v *models.StudioVersion, n int) { v.Version = n },
	recordID:          func(v *models.StudioVersion) string { return v.RecordID },
	displayName:       func(v *models.StudioVersion) string { return v.Name },
	fields: map[string]entityFieldAccessor[models.StudioVersion]{
		"name":       {get: func(v *models.StudioVersion) any { return v.Name }, set: func(v *models.StudioVersion, val any) { v.Name = val.(string) }},
		"website":    {get: func(v *models.StudioVersion) any { return v.Website }, set: func(v *models.StudioVersion, val any) { v.Website = val.(url.URL) }},
		"notes":      {get: func(v *models.StudioVersion) any { return v.Notes }, set: func(v *models.StudioVersion, val any) { v.Notes = val.(string) }},
		"properties": {get: func(v *models.StudioVersion) any { return v.Properties }, set: func(v *models.StudioVersion, val any) { v.Properties = val.([]modelcore.Property) }},
		"tags":       {get: func(v *models.StudioVersion) any { return v.Tags }, set: func(v *models.StudioVersion, val any) { v.Tags = val.([]modelcore.Tag) }},
	},
}

// EnsureStudioVersioningIndexes creates the indexes studio version queries rely on. Safe to call
// on every startup.
func EnsureStudioVersioningIndexes(c context.Context) error {
	return studioVersioning.ensureIndexes(c)
}

func studioVersionToVO(version *models.StudioVersion) *vo.StudioVersionVO {
	return &vo.StudioVersionVO{
		ID: version.ID, RecordID: version.RecordID, Version: version.Version,
		Name: version.Name, Website: version.Website.String(), Notes: version.Notes,
		Properties: modelcoreutil.FromPropertyModels(version.Properties),
		Tags:       modelcoreutil.FromTagModels(version.Tags),
		VersionLifecycleVO: vo.VersionLifecycleVO{
			State: vo.VersionState(version.State), BaseVersion: version.BaseVersion,
			SubmittedBy: version.SubmittedBy, SubmittedAt: version.SubmittedAt,
			ReviewedBy: version.ReviewedBy, ReviewedAt: version.ReviewedAt,
			ReviewNote: version.ReviewNote, ResultingVersion: version.ResultingVersion,
		},
	}
}

func studioVersionFields(studio *vo.StudioVO) models.StudioVersion {
	return models.StudioVersion{
		Name: studio.Name, Website: parseWebsite(studio.Website), Notes: studio.Notes,
		Properties: modelcoreutil.ToPropertyModels(studio.Properties),
		Tags:       modelcoreutil.ToTagModels(studio.Tags),
	}
}

func flattenStudio(meta *models.EntityMeta, version *models.StudioVersion) *vo.StudioVO {
	return &vo.StudioVO{
		ID: meta.ID, Name: version.Name, Website: version.Website.String(), Notes: version.Notes,
		Properties: modelcoreutil.FromPropertyModels(version.Properties),
		Tags:       modelcoreutil.FromTagModels(version.Tags),
		AuditableVO: modelcorevo.AuditableVO{
			CreatedAt: meta.CreatedAt, CreatedBy: meta.CreatedBy,
			UpdatedAt: version.SubmittedAt, UpdatedBy: version.SubmittedBy,
			DeletedAt: meta.DeletedAt, DeletedBy: meta.DeletedBy,
		},
	}
}

// AddStudio creates a studio's meta record and its first (live) version.
func AddStudio(c context.Context, studio *vo.StudioVO) (*string, error) {
	version := studioVersionFields(studio)
	return studioVersioning.addEntity(c, &version, studio.CreatedBy)
}

// GetStudio returns the flattened view of a studio.
func GetStudio(c context.Context, id string) (*vo.StudioVO, error) {
	meta, err := studioVersioning.getMeta(c, id)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, nil
	}
	version, err := studioVersioning.getVersion(c, id, meta.CurrentVersion)
	if err != nil {
		return nil, err
	}
	if version == nil {
		return nil, nil
	}
	return flattenStudio(meta, version), nil
}

// UpdateStudio creates a new version of the studio rather than mutating in place.
func UpdateStudio(c context.Context, id string, studio *vo.StudioVO, state models.VersionState) (*vo.StudioVersionVO, error) {
	version := studioVersionFields(studio)
	result, err := studioVersioning.createVersion(c, id, &version, state, studio.UpdatedBy)
	if err != nil || result == nil {
		return nil, err
	}
	return studioVersionToVO(result), nil
}

// ListStudioVersions returns every version of a studio, newest first.
func ListStudioVersions(c context.Context, id string) ([]*vo.StudioVersionVO, error) {
	versions, err := studioVersioning.listVersions(c, id)
	if err != nil {
		return nil, err
	}
	vos := make([]*vo.StudioVersionVO, 0, len(versions))
	for _, v := range versions {
		vos = append(vos, studioVersionToVO(v))
	}
	return vos, nil
}

// GetStudioVersion returns one version's full snapshot.
func GetStudioVersion(c context.Context, id string, version int) (*vo.StudioVersionVO, error) {
	result, err := studioVersioning.getVersion(c, id, version)
	if err != nil || result == nil {
		return nil, err
	}
	return studioVersionToVO(result), nil
}

// AcceptStudioVersion reviews a submitted version - see AcceptVolumeVersion's doc comment.
func AcceptStudioVersion(c context.Context, id string, version int, selectedFields []string, reviewedBy string, reviewNote *string) (*vo.StudioVersionVO, []string, error) {
	result, conflicts, err := studioVersioning.acceptVersion(c, id, version, selectedFields, reviewedBy, reviewNote)
	if err != nil || result == nil {
		return nil, conflicts, err
	}
	return studioVersionToVO(result), conflicts, nil
}

// RejectStudioVersion marks a submitted version rejected.
func RejectStudioVersion(c context.Context, id string, version int, reviewedBy string, reviewNote *string) error {
	return studioVersioning.rejectVersion(c, id, version, reviewedBy, reviewNote)
}

// RetractStudioVersion lets the original submitter withdraw their own pending submission.
func RetractStudioVersion(c context.Context, id string, version int, submitterID string) (*vo.StudioVersionVO, error) {
	result, err := studioVersioning.retractVersion(c, id, version, submitterID)
	if err != nil || result == nil {
		return nil, err
	}
	return studioVersionToVO(result), nil
}

// SetCurrentStudioVersion rolls a studio back (or forward) to an arbitrary existing version.
func SetCurrentStudioVersion(c context.Context, id string, version int) (*vo.StudioVersionVO, error) {
	result, err := studioVersioning.setCurrentVersion(c, id, version)
	if err != nil || result == nil {
		return nil, err
	}
	return studioVersionToVO(result), nil
}

// CountSubmittedStudioVersionsBySubmitter counts a submitter's currently-pending versions.
func CountSubmittedStudioVersionsBySubmitter(c context.Context, submittedBy string) (int64, error) {
	return studioVersioning.countSubmittedBySubmitter(c, submittedBy)
}

// QueryStudios lists the current (live) version of every studio matching params.
func QueryStudios(c context.Context, params apiutil.QueryParams) ([]*vo.StudioVO, error) {
	filter, sort, projection := apiutil.ConvertQueryParams(params)
	metas, err := database.Query[models.EntityMeta](studioMetaCollection, filter, sort, projection, params.Start, params.Limit)
	if err != nil {
		return nil, err
	}
	vos := make([]*vo.StudioVO, 0, len(metas))
	for _, meta := range metas {
		version, err := studioVersioning.getVersion(c, meta.ID, meta.CurrentVersion)
		if err != nil {
			return nil, err
		}
		if version == nil {
			continue
		}
		vos = append(vos, flattenStudio(meta, version))
	}
	return vos, nil
}

// CreateSubmittedStudioVersion creates a submitted version stamped with a caller-supplied
// submittedBy/submittedAt - see CreateSubmittedPublisherVersion's doc comment.
func CreateSubmittedStudioVersion(c context.Context, id string, studio *vo.StudioVO, submittedBy string, submittedAt time.Time) (*vo.StudioVersionVO, error) {
	version := studioVersionFields(studio)
	result, err := studioVersioning.createVersionWithSubmission(c, id, &version, models.VersionStateSubmitted, submittedBy, submittedAt)
	if err != nil || result == nil {
		return nil, err
	}
	return studioVersionToVO(result), nil
}

// MigrateStudios backfills every existing "studios" document into a meta record plus a single
// live version - see MigratePublishers' doc comment.
func MigrateStudios(c context.Context) (int, error) {
	return migrateEntity(c, migrationConfig[models.Studio, models.StudioVersion]{
		oldCollection: "studios",
		versioning:    studioVersioning,
		id:            func(s *models.Studio) string { return s.ID },
		auditable:     func(s *models.Studio) modelcore.Auditable { return s.Auditable },
		toVersion: func(s *models.Studio) models.StudioVersion {
			return models.StudioVersion{
				Name: s.Name, Website: s.Website, Notes: s.Notes,
				Properties: s.Properties, Tags: s.Tags,
			}
		},
	})
}

// GetStudioStats returns the studios landing-page-summary card's count/most-recent data.
func GetStudioStats(c context.Context) (*TypeStats, error) {
	return studioVersioning.stats(c)
}
