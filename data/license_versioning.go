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
	"go.mongodb.org/mongo-driver/bson"
)

const (
	licenseMetaCollection    = "licenses_meta"
	licenseVersionCollection = "licenses_versions"
)

var licenseVersioning = entityVersioningConfig[models.LicenseVersion]{
	metaCollection:    licenseMetaCollection,
	versionCollection: licenseVersionCollection,
	typeName:          "license",
	lifecycle:         func(v *models.LicenseVersion) *models.VersionLifecycle { return &v.VersionLifecycle },
	setID:             func(v *models.LicenseVersion, id string) { v.ID = id },
	setRecordID:       func(v *models.LicenseVersion, id string) { v.RecordID = id },
	setVersion:        func(v *models.LicenseVersion, n int) { v.Version = n },
	recordID:          func(v *models.LicenseVersion) string { return v.RecordID },
	displayName:       func(v *models.LicenseVersion) string { return v.Title },
	fields: map[string]entityFieldAccessor[models.LicenseVersion]{
		"title":         {get: func(v *models.LicenseVersion) any { return v.Title }, set: func(v *models.LicenseVersion, val any) { v.Title = val.(string) }},
		"short_title":   {get: func(v *models.LicenseVersion) any { return v.ShortTitle }, set: func(v *models.LicenseVersion, val any) { v.ShortTitle = val.(string) }},
		"version_label": {get: func(v *models.LicenseVersion) any { return v.LicenseVer }, set: func(v *models.LicenseVersion, val any) { v.LicenseVer = val.(string) }},
		"deed":          {get: func(v *models.LicenseVersion) any { return v.Deed }, set: func(v *models.LicenseVersion, val any) { v.Deed = val.(string) }},
		"legal_code":    {get: func(v *models.LicenseVersion) any { return v.LegalCode }, set: func(v *models.LicenseVersion, val any) { v.LegalCode = val.(string) }},
		"website":       {get: func(v *models.LicenseVersion) any { return v.Website }, set: func(v *models.LicenseVersion, val any) { v.Website = val.(url.URL) }},
		"status":        {get: func(v *models.LicenseVersion) any { return v.Status }, set: func(v *models.LicenseVersion, val any) { v.Status = val.(string) }},
		"availability":  {get: func(v *models.LicenseVersion) any { return v.Availability }, set: func(v *models.LicenseVersion, val any) { v.Availability = val.(string) }},
		"notes":         {get: func(v *models.LicenseVersion) any { return v.Notes }, set: func(v *models.LicenseVersion, val any) { v.Notes = val.(string) }},
		"properties":    {get: func(v *models.LicenseVersion) any { return v.Properties }, set: func(v *models.LicenseVersion, val any) { v.Properties = val.([]modelcore.Property) }},
		"tags":          {get: func(v *models.LicenseVersion) any { return v.Tags }, set: func(v *models.LicenseVersion, val any) { v.Tags = val.([]modelcore.Tag) }},
	},
}

// EnsureLicenseVersioningIndexes creates the indexes license version queries rely on. Safe to
// call on every startup.
func EnsureLicenseVersioningIndexes(c context.Context) error {
	return licenseVersioning.ensureIndexes(c)
}

// SoftDeleteLicense hides a license from every Query*/List* read without touching its version
// history - see design.md's soft-delete decision.
func SoftDeleteLicense(c context.Context, id string, deletedBy string) error {
	return licenseVersioning.softDelete(c, id, deletedBy)
}

// RestoreLicense clears a soft-deleted license's deletion, returning it to every normal read path.
func RestoreLicense(c context.Context, id string) error {
	return licenseVersioning.restore(c, id)
}

func licenseVersionToVO(version *models.LicenseVersion) *vo.LicenseVersionVO {
	return &vo.LicenseVersionVO{
		ID: version.ID, RecordID: version.RecordID, Version: version.Version,
		Title: version.Title, ShortTitle: version.ShortTitle, LicenseVer: version.LicenseVer,
		Deed: version.Deed, LegalCode: version.LegalCode, Website: version.Website.String(),
		Status: version.Status, Availability: version.Availability, Notes: version.Notes,
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

func licenseVersionFields(license *vo.LicenseVO) models.LicenseVersion {
	return models.LicenseVersion{
		Title: license.Title, ShortTitle: license.ShortTitle, LicenseVer: license.Version,
		Deed: license.Deed, LegalCode: license.LegalCode, Website: parseWebsite(license.Website),
		Status: license.Status, Availability: license.Availability, Notes: license.Notes,
		Properties: modelcoreutil.ToPropertyModels(license.Properties),
		Tags:       modelcoreutil.ToTagModels(license.Tags),
	}
}

func flattenLicense(meta *models.EntityMeta, version *models.LicenseVersion) *vo.LicenseVO {
	return &vo.LicenseVO{
		ID: meta.ID, Title: version.Title, ShortTitle: version.ShortTitle, Version: version.LicenseVer,
		Deed: version.Deed, LegalCode: version.LegalCode, Website: version.Website.String(),
		Status: version.Status, Availability: version.Availability, Notes: version.Notes,
		Properties: modelcoreutil.FromPropertyModels(version.Properties),
		Tags:       modelcoreutil.FromTagModels(version.Tags),
		AuditableVO: modelcorevo.AuditableVO{
			CreatedAt: meta.CreatedAt, CreatedBy: meta.CreatedBy,
			UpdatedAt: version.SubmittedAt, UpdatedBy: version.SubmittedBy,
			DeletedAt: meta.DeletedAt, DeletedBy: meta.DeletedBy,
		},
	}
}

// AddLicense creates a license's meta record and its first (live) version.
func AddLicense(c context.Context, license *vo.LicenseVO) (*string, error) {
	version := licenseVersionFields(license)
	return licenseVersioning.addEntity(c, &version, license.CreatedBy)
}

// GetLicense returns the flattened view of a license.
func GetLicense(c context.Context, id string) (*vo.LicenseVO, error) {
	meta, err := licenseVersioning.getMeta(c, id)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, nil
	}
	version, err := licenseVersioning.getVersion(c, id, meta.CurrentVersion)
	if err != nil {
		return nil, err
	}
	if version == nil {
		return nil, nil
	}
	return flattenLicense(meta, version), nil
}

// UpdateLicense creates a new version of the license rather than mutating in place.
func UpdateLicense(c context.Context, id string, license *vo.LicenseVO, state models.VersionState) (*vo.LicenseVersionVO, error) {
	version := licenseVersionFields(license)
	result, err := licenseVersioning.createVersion(c, id, &version, state, license.UpdatedBy)
	if err != nil || result == nil {
		return nil, err
	}
	return licenseVersionToVO(result), nil
}

// ListLicenseVersions returns every version of a license, newest first.
func ListLicenseVersions(c context.Context, id string) ([]*vo.LicenseVersionVO, error) {
	versions, err := licenseVersioning.listVersions(c, id)
	if err != nil {
		return nil, err
	}
	vos := make([]*vo.LicenseVersionVO, 0, len(versions))
	for _, v := range versions {
		vos = append(vos, licenseVersionToVO(v))
	}
	return vos, nil
}

// GetLicenseVersion returns one version's full snapshot.
func GetLicenseVersion(c context.Context, id string, version int) (*vo.LicenseVersionVO, error) {
	result, err := licenseVersioning.getVersion(c, id, version)
	if err != nil || result == nil {
		return nil, err
	}
	return licenseVersionToVO(result), nil
}

// AcceptLicenseVersion reviews a submitted version - see AcceptVolumeVersion's doc comment.
func AcceptLicenseVersion(c context.Context, id string, version int, selectedFields []string, reviewedBy string, reviewNote *string) (*vo.LicenseVersionVO, []string, error) {
	result, conflicts, err := licenseVersioning.acceptVersion(c, id, version, selectedFields, reviewedBy, reviewNote)
	if err != nil || result == nil {
		return nil, conflicts, err
	}
	return licenseVersionToVO(result), conflicts, nil
}

// RejectLicenseVersion marks a submitted version rejected.
func RejectLicenseVersion(c context.Context, id string, version int, reviewedBy string, reviewNote *string) error {
	return licenseVersioning.rejectVersion(c, id, version, reviewedBy, reviewNote)
}

// RetractLicenseVersion lets the original submitter withdraw their own pending submission.
func RetractLicenseVersion(c context.Context, id string, version int, submitterID string) (*vo.LicenseVersionVO, error) {
	result, err := licenseVersioning.retractVersion(c, id, version, submitterID)
	if err != nil || result == nil {
		return nil, err
	}
	return licenseVersionToVO(result), nil
}

// SetCurrentLicenseVersion rolls a license back (or forward) to an arbitrary existing version.
func SetCurrentLicenseVersion(c context.Context, id string, version int) (*vo.LicenseVersionVO, error) {
	result, err := licenseVersioning.setCurrentVersion(c, id, version)
	if err != nil || result == nil {
		return nil, err
	}
	return licenseVersionToVO(result), nil
}

// CountSubmittedLicenseVersionsBySubmitter counts a submitter's currently-pending versions.
func CountSubmittedLicenseVersionsBySubmitter(c context.Context, submittedBy string) (int64, error) {
	return licenseVersioning.countSubmittedBySubmitter(c, submittedBy)
}

// QueryLicenses lists the current (live) version of every license matching params.
func QueryLicenses(c context.Context, params apiutil.QueryParams) ([]*vo.LicenseVO, error) {
	filter, sort, projection := apiutil.ConvertQueryParams(params)
	filter = append(filter, bson.E{Key: "deleted_at", Value: nil})
	metas, err := database.Query[models.EntityMeta](licenseMetaCollection, filter, sort, projection, params.Start, params.Limit)
	if err != nil {
		return nil, err
	}
	vos := make([]*vo.LicenseVO, 0, len(metas))
	for _, meta := range metas {
		version, err := licenseVersioning.getVersion(c, meta.ID, meta.CurrentVersion)
		if err != nil {
			return nil, err
		}
		if version == nil {
			continue
		}
		vos = append(vos, flattenLicense(meta, version))
	}
	return vos, nil
}

// CreateSubmittedLicenseVersion creates a submitted version stamped with a caller-supplied
// submittedBy/submittedAt - see CreateSubmittedPublisherVersion's doc comment.
func CreateSubmittedLicenseVersion(c context.Context, id string, license *vo.LicenseVO, submittedBy string, submittedAt time.Time) (*vo.LicenseVersionVO, error) {
	version := licenseVersionFields(license)
	result, err := licenseVersioning.createVersionWithSubmission(c, id, &version, models.VersionStateSubmitted, submittedBy, submittedAt)
	if err != nil || result == nil {
		return nil, err
	}
	return licenseVersionToVO(result), nil
}

// MigrateLicenses backfills every existing "licenses" document into a meta record plus a single
// live version - see MigratePublishers' doc comment.
func MigrateLicenses(c context.Context) (int, error) {
	return migrateEntity(c, migrationConfig[models.License, models.LicenseVersion]{
		oldCollection: "licenses",
		versioning:    licenseVersioning,
		id:            func(l *models.License) string { return l.ID },
		auditable:     func(l *models.License) modelcore.Auditable { return l.Auditable },
		toVersion: func(l *models.License) models.LicenseVersion {
			return models.LicenseVersion{
				Title: l.Title, ShortTitle: l.ShortTitle, LicenseVer: l.Version, Deed: l.Deed,
				LegalCode: l.LegalCode, Website: l.Website, Status: l.Status,
				Availability: l.Availability, Notes: l.Notes,
				Properties: l.Properties, Tags: l.Tags,
			}
		},
	})
}

// GetLicenseStats returns the licenses landing-page-summary card's count/most-recent data.
func GetLicenseStats(c context.Context) (*TypeStats, error) {
	return licenseVersioning.stats(c)
}
