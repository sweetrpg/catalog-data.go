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
	publisherMetaCollection    = "publishers_meta"
	publisherVersionCollection = "publishers_versions"
)

var publisherVersioning = entityVersioningConfig[models.PublisherVersion]{
	metaCollection:    publisherMetaCollection,
	versionCollection: publisherVersionCollection,
	typeName:          "publisher",
	lifecycle:         func(v *models.PublisherVersion) *models.VersionLifecycle { return &v.VersionLifecycle },
	setID:             func(v *models.PublisherVersion, id string) { v.ID = id },
	setRecordID:       func(v *models.PublisherVersion, id string) { v.RecordID = id },
	setVersion:        func(v *models.PublisherVersion, n int) { v.Version = n },
	fields: map[string]entityFieldAccessor[models.PublisherVersion]{
		"name":       {get: func(v *models.PublisherVersion) any { return v.Name }, set: func(v *models.PublisherVersion, val any) { v.Name = val.(string) }},
		"address":    {get: func(v *models.PublisherVersion) any { return v.Address }, set: func(v *models.PublisherVersion, val any) { v.Address = val.(string) }},
		"website":    {get: func(v *models.PublisherVersion) any { return v.Website }, set: func(v *models.PublisherVersion, val any) { v.Website = val.(url.URL) }},
		"notes":      {get: func(v *models.PublisherVersion) any { return v.Notes }, set: func(v *models.PublisherVersion, val any) { v.Notes = val.(string) }},
		"properties": {get: func(v *models.PublisherVersion) any { return v.Properties }, set: func(v *models.PublisherVersion, val any) { v.Properties = val.([]modelcore.Property) }},
		"tags":       {get: func(v *models.PublisherVersion) any { return v.Tags }, set: func(v *models.PublisherVersion, val any) { v.Tags = val.([]modelcore.Tag) }},
	},
}

// EnsurePublisherVersioningIndexes creates the indexes publisher version queries rely on. Safe to
// call on every startup.
func EnsurePublisherVersioningIndexes(c context.Context) error {
	return publisherVersioning.ensureIndexes(c)
}

func publisherVersionToVO(version *models.PublisherVersion) *vo.PublisherVersionVO {
	return &vo.PublisherVersionVO{
		ID: version.ID, RecordID: version.RecordID, Version: version.Version,
		Name: version.Name, Address: version.Address, Website: version.Website.String(), Notes: version.Notes,
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

func publisherVersionFields(publisher *vo.PublisherVO) models.PublisherVersion {
	return models.PublisherVersion{
		Name: publisher.Name, Address: publisher.Address, Website: parseWebsite(publisher.Website), Notes: publisher.Notes,
		Properties: modelcoreutil.ToPropertyModels(publisher.Properties),
		Tags:       modelcoreutil.ToTagModels(publisher.Tags),
	}
}

func flattenPublisher(meta *models.EntityMeta, version *models.PublisherVersion) *vo.PublisherVO {
	return &vo.PublisherVO{
		ID: meta.ID, Name: version.Name, Address: version.Address, Website: version.Website.String(), Notes: version.Notes,
		Properties: modelcoreutil.FromPropertyModels(version.Properties),
		Tags:       modelcoreutil.FromTagModels(version.Tags),
		AuditableVO: modelcorevo.AuditableVO{
			CreatedAt: meta.CreatedAt, CreatedBy: meta.CreatedBy,
			UpdatedAt: version.SubmittedAt, UpdatedBy: version.SubmittedBy,
			DeletedAt: meta.DeletedAt, DeletedBy: meta.DeletedBy,
		},
	}
}

// AddPublisher creates a publisher's meta record and its first (live) version.
func AddPublisher(c context.Context, publisher *vo.PublisherVO) (*string, error) {
	version := publisherVersionFields(publisher)
	return publisherVersioning.addEntity(c, &version, publisher.CreatedBy)
}

// GetPublisher returns the flattened view of a publisher - matching the shape this function
// returned before meta/version were split.
func GetPublisher(c context.Context, id string) (*vo.PublisherVO, error) {
	meta, err := publisherVersioning.getMeta(c, id)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, nil
	}
	version, err := publisherVersioning.getVersion(c, id, meta.CurrentVersion)
	if err != nil {
		return nil, err
	}
	if version == nil {
		return nil, nil
	}
	return flattenPublisher(meta, version), nil
}

// UpdatePublisher creates a new version of the publisher rather than mutating in place - state
// VersionStateLive goes current immediately (editor/admin); VersionStateSubmitted leaves the
// current pointer untouched (submitter).
func UpdatePublisher(c context.Context, id string, publisher *vo.PublisherVO, state models.VersionState) (*vo.PublisherVersionVO, error) {
	version := publisherVersionFields(publisher)
	result, err := publisherVersioning.createVersion(c, id, &version, state, publisher.UpdatedBy)
	if err != nil || result == nil {
		return nil, err
	}
	return publisherVersionToVO(result), nil
}

// ListPublisherVersions returns every version of a publisher, newest first.
func ListPublisherVersions(c context.Context, id string) ([]*vo.PublisherVersionVO, error) {
	versions, err := publisherVersioning.listVersions(c, id)
	if err != nil {
		return nil, err
	}
	vos := make([]*vo.PublisherVersionVO, 0, len(versions))
	for _, v := range versions {
		vos = append(vos, publisherVersionToVO(v))
	}
	return vos, nil
}

// GetPublisherVersion returns one version's full snapshot, regardless of whether it's current.
func GetPublisherVersion(c context.Context, id string, version int) (*vo.PublisherVersionVO, error) {
	result, err := publisherVersioning.getVersion(c, id, version)
	if err != nil || result == nil {
		return nil, err
	}
	return publisherVersionToVO(result), nil
}

// AcceptPublisherVersion reviews a submitted version - see AcceptVolumeVersion's doc comment.
func AcceptPublisherVersion(c context.Context, id string, version int, selectedFields []string, reviewedBy string, reviewNote *string) (*vo.PublisherVersionVO, []string, error) {
	result, conflicts, err := publisherVersioning.acceptVersion(c, id, version, selectedFields, reviewedBy, reviewNote)
	if err != nil || result == nil {
		return nil, conflicts, err
	}
	return publisherVersionToVO(result), conflicts, nil
}

// RejectPublisherVersion marks a submitted version rejected.
func RejectPublisherVersion(c context.Context, id string, version int, reviewedBy string, reviewNote *string) error {
	return publisherVersioning.rejectVersion(c, id, version, reviewedBy, reviewNote)
}

// RetractPublisherVersion lets the original submitter withdraw their own pending submission.
func RetractPublisherVersion(c context.Context, id string, version int, submitterID string) (*vo.PublisherVersionVO, error) {
	result, err := publisherVersioning.retractVersion(c, id, version, submitterID)
	if err != nil || result == nil {
		return nil, err
	}
	return publisherVersionToVO(result), nil
}

// SetCurrentPublisherVersion rolls a publisher back (or forward) to an arbitrary existing version.
func SetCurrentPublisherVersion(c context.Context, id string, version int) (*vo.PublisherVersionVO, error) {
	result, err := publisherVersioning.setCurrentVersion(c, id, version)
	if err != nil || result == nil {
		return nil, err
	}
	return publisherVersionToVO(result), nil
}

// CountSubmittedPublisherVersionsBySubmitter counts a submitter's currently-pending versions.
func CountSubmittedPublisherVersionsBySubmitter(c context.Context, submittedBy string) (int64, error) {
	return publisherVersioning.countSubmittedBySubmitter(c, submittedBy)
}

// QueryPublishers lists the current (live) version of every publisher matching params.
func QueryPublishers(c context.Context, params apiutil.QueryParams) ([]*vo.PublisherVO, error) {
	filter, sort, projection := apiutil.ConvertQueryParams(params)
	metas, err := database.Query[models.EntityMeta](publisherMetaCollection, filter, sort, projection, params.Start, params.Limit)
	if err != nil {
		return nil, err
	}
	vos := make([]*vo.PublisherVO, 0, len(metas))
	for _, meta := range metas {
		version, err := publisherVersioning.getVersion(c, meta.ID, meta.CurrentVersion)
		if err != nil {
			return nil, err
		}
		if version == nil {
			continue
		}
		vos = append(vos, flattenPublisher(meta, version))
	}
	return vos, nil
}

// CreateSubmittedPublisherVersion creates a submitted version stamped with a caller-supplied
// submittedBy/submittedAt - for migrating a pre-versioning proposed_changes entry into the
// version model, preserving the original proposal's submission audit (see
// data.CreateSubmittedVolumeVersion, the bespoke original this mirrors).
func CreateSubmittedPublisherVersion(c context.Context, id string, publisher *vo.PublisherVO, submittedBy string, submittedAt time.Time) (*vo.PublisherVersionVO, error) {
	version := publisherVersionFields(publisher)
	result, err := publisherVersioning.createVersionWithSubmission(c, id, &version, models.VersionStateSubmitted, submittedBy, submittedAt)
	if err != nil || result == nil {
		return nil, err
	}
	return publisherVersionToVO(result), nil
}

// MigratePublishers backfills every existing "publishers" document (the pre-versioning flat
// model) into a meta record plus a single live version, per design.md's Migration Plan.
// Idempotent - see migrateEntity's doc comment.
func MigratePublishers(c context.Context) (int, error) {
	return migrateEntity(c, migrationConfig[models.Publisher, models.PublisherVersion]{
		oldCollection: "publishers",
		versioning:    publisherVersioning,
		id:            func(p *models.Publisher) string { return p.ID },
		auditable:     func(p *models.Publisher) modelcore.Auditable { return p.Auditable },
		toVersion: func(p *models.Publisher) models.PublisherVersion {
			return models.PublisherVersion{
				Name: p.Name, Address: p.Address, Website: p.Website, Notes: p.Notes,
				Properties: p.Properties, Tags: p.Tags,
			}
		},
	})
}
