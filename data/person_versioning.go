package data

import (
	"context"
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
	personMetaCollection    = "persons_meta"
	personVersionCollection = "persons_versions"
)

var personVersioning = entityVersioningConfig[models.PersonVersion]{
	metaCollection:    personMetaCollection,
	versionCollection: personVersionCollection,
	typeName:          "person",
	lifecycle:         func(v *models.PersonVersion) *models.VersionLifecycle { return &v.VersionLifecycle },
	setID:             func(v *models.PersonVersion, id string) { v.ID = id },
	setRecordID:       func(v *models.PersonVersion, id string) { v.RecordID = id },
	setVersion:        func(v *models.PersonVersion, n int) { v.Version = n },
	recordID:          func(v *models.PersonVersion) string { return v.RecordID },
	displayName:       func(v *models.PersonVersion) string { return v.Name },
	fields: map[string]entityFieldAccessor[models.PersonVersion]{
		"name":       {get: func(v *models.PersonVersion) any { return v.Name }, set: func(v *models.PersonVersion, val any) { v.Name = val.(string) }},
		"notes":      {get: func(v *models.PersonVersion) any { return v.Notes }, set: func(v *models.PersonVersion, val any) { v.Notes = val.(string) }},
		"properties": {get: func(v *models.PersonVersion) any { return v.Properties }, set: func(v *models.PersonVersion, val any) { v.Properties = val.([]modelcore.Property) }},
		"tags":       {get: func(v *models.PersonVersion) any { return v.Tags }, set: func(v *models.PersonVersion, val any) { v.Tags = val.([]modelcore.Tag) }},
	},
}

// EnsurePersonVersioningIndexes creates the indexes person version queries rely on. Safe to call
// on every startup.
func EnsurePersonVersioningIndexes(c context.Context) error {
	return personVersioning.ensureIndexes(c)
}

func personVersionToVO(version *models.PersonVersion) *vo.PersonVersionVO {
	return &vo.PersonVersionVO{
		ID: version.ID, RecordID: version.RecordID, Version: version.Version,
		Name: version.Name, Notes: version.Notes,
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

func personVersionFields(person *vo.PersonVO) models.PersonVersion {
	return models.PersonVersion{
		Name: person.Name, Notes: person.Notes,
		Properties: modelcoreutil.ToPropertyModels(person.Properties),
		Tags:       modelcoreutil.ToTagModels(person.Tags),
	}
}

func flattenPerson(meta *models.EntityMeta, version *models.PersonVersion) *vo.PersonVO {
	return &vo.PersonVO{
		ID: meta.ID, Name: version.Name, Notes: version.Notes,
		Properties: modelcoreutil.FromPropertyModels(version.Properties),
		Tags:       modelcoreutil.FromTagModels(version.Tags),
		AuditableVO: modelcorevo.AuditableVO{
			CreatedAt: meta.CreatedAt, CreatedBy: meta.CreatedBy,
			UpdatedAt: version.SubmittedAt, UpdatedBy: version.SubmittedBy,
			DeletedAt: meta.DeletedAt, DeletedBy: meta.DeletedBy,
		},
	}
}

// AddPerson creates a person's meta record and its first (live) version.
func AddPerson(c context.Context, person *vo.PersonVO) (*string, error) {
	version := personVersionFields(person)
	return personVersioning.addEntity(c, &version, person.CreatedBy)
}

// GetPerson returns the flattened view of a person.
func GetPerson(c context.Context, id string) (*vo.PersonVO, error) {
	meta, err := personVersioning.getMeta(c, id)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, nil
	}
	version, err := personVersioning.getVersion(c, id, meta.CurrentVersion)
	if err != nil {
		return nil, err
	}
	if version == nil {
		return nil, nil
	}
	return flattenPerson(meta, version), nil
}

// UpdatePerson creates a new version of the person rather than mutating in place.
func UpdatePerson(c context.Context, id string, person *vo.PersonVO, state models.VersionState) (*vo.PersonVersionVO, error) {
	version := personVersionFields(person)
	result, err := personVersioning.createVersion(c, id, &version, state, person.UpdatedBy)
	if err != nil || result == nil {
		return nil, err
	}
	return personVersionToVO(result), nil
}

// ListPersonVersions returns every version of a person, newest first.
func ListPersonVersions(c context.Context, id string) ([]*vo.PersonVersionVO, error) {
	versions, err := personVersioning.listVersions(c, id)
	if err != nil {
		return nil, err
	}
	vos := make([]*vo.PersonVersionVO, 0, len(versions))
	for _, v := range versions {
		vos = append(vos, personVersionToVO(v))
	}
	return vos, nil
}

// GetPersonVersion returns one version's full snapshot.
func GetPersonVersion(c context.Context, id string, version int) (*vo.PersonVersionVO, error) {
	result, err := personVersioning.getVersion(c, id, version)
	if err != nil || result == nil {
		return nil, err
	}
	return personVersionToVO(result), nil
}

// AcceptPersonVersion reviews a submitted version - see AcceptVolumeVersion's doc comment.
func AcceptPersonVersion(c context.Context, id string, version int, selectedFields []string, reviewedBy string, reviewNote *string) (*vo.PersonVersionVO, []string, error) {
	result, conflicts, err := personVersioning.acceptVersion(c, id, version, selectedFields, reviewedBy, reviewNote)
	if err != nil || result == nil {
		return nil, conflicts, err
	}
	return personVersionToVO(result), conflicts, nil
}

// RejectPersonVersion marks a submitted version rejected.
func RejectPersonVersion(c context.Context, id string, version int, reviewedBy string, reviewNote *string) error {
	return personVersioning.rejectVersion(c, id, version, reviewedBy, reviewNote)
}

// RetractPersonVersion lets the original submitter withdraw their own pending submission.
func RetractPersonVersion(c context.Context, id string, version int, submitterID string) (*vo.PersonVersionVO, error) {
	result, err := personVersioning.retractVersion(c, id, version, submitterID)
	if err != nil || result == nil {
		return nil, err
	}
	return personVersionToVO(result), nil
}

// SetCurrentPersonVersion rolls a person back (or forward) to an arbitrary existing version.
func SetCurrentPersonVersion(c context.Context, id string, version int) (*vo.PersonVersionVO, error) {
	result, err := personVersioning.setCurrentVersion(c, id, version)
	if err != nil || result == nil {
		return nil, err
	}
	return personVersionToVO(result), nil
}

// CountSubmittedPersonVersionsBySubmitter counts a submitter's currently-pending versions.
func CountSubmittedPersonVersionsBySubmitter(c context.Context, submittedBy string) (int64, error) {
	return personVersioning.countSubmittedBySubmitter(c, submittedBy)
}

// QueryPersons lists the current (live) version of every person matching params.
func QueryPersons(c context.Context, params apiutil.QueryParams) ([]*vo.PersonVO, error) {
	filter, sort, projection := apiutil.ConvertQueryParams(params)
	metas, err := database.Query[models.EntityMeta](personMetaCollection, filter, sort, projection, params.Start, params.Limit)
	if err != nil {
		return nil, err
	}
	vos := make([]*vo.PersonVO, 0, len(metas))
	for _, meta := range metas {
		version, err := personVersioning.getVersion(c, meta.ID, meta.CurrentVersion)
		if err != nil {
			return nil, err
		}
		if version == nil {
			continue
		}
		vos = append(vos, flattenPerson(meta, version))
	}
	return vos, nil
}

// CreateSubmittedPersonVersion creates a submitted version stamped with a caller-supplied
// submittedBy/submittedAt - see CreateSubmittedPublisherVersion's doc comment.
func CreateSubmittedPersonVersion(c context.Context, id string, person *vo.PersonVO, submittedBy string, submittedAt time.Time) (*vo.PersonVersionVO, error) {
	version := personVersionFields(person)
	result, err := personVersioning.createVersionWithSubmission(c, id, &version, models.VersionStateSubmitted, submittedBy, submittedAt)
	if err != nil || result == nil {
		return nil, err
	}
	return personVersionToVO(result), nil
}

// MigratePersons backfills every existing "persons" document into a meta record plus a single
// live version - see MigratePublishers' doc comment.
func MigratePersons(c context.Context) (int, error) {
	return migrateEntity(c, migrationConfig[models.Person, models.PersonVersion]{
		oldCollection: "persons",
		versioning:    personVersioning,
		id:            func(p *models.Person) string { return p.ID },
		auditable:     func(p *models.Person) modelcore.Auditable { return p.Auditable },
		toVersion: func(p *models.Person) models.PersonVersion {
			return models.PersonVersion{
				Name: p.Name, Notes: p.Notes, Properties: p.Properties, Tags: p.Tags,
			}
		},
	})
}

// GetPersonStats returns the persons landing-page-summary card's count/most-recent data.
func GetPersonStats(c context.Context) (*TypeStats, error) {
	return personVersioning.stats(c)
}
