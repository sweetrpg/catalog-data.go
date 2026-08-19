package data

import (
	"context"

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
	systemMetaCollection    = "systems_meta"
	systemVersionCollection = "systems_versions"
)

// systemVersioning gives system the same meta+version model as the other four generic types,
// even though system has no write path today (see proposal.md's Impact section) - task 7.5
// explicitly repeats task groups 1-4 (model/data-access/API endpoints/SDK) for system, giving it
// a write path for the first time as part of this change; only catalog-web pages are out of
// scope, per design.md's Non-Goals.
var systemVersioning = entityVersioningConfig[models.SystemVersion]{
	metaCollection:    systemMetaCollection,
	versionCollection: systemVersionCollection,
	typeName:          "system",
	lifecycle:         func(v *models.SystemVersion) *models.VersionLifecycle { return &v.VersionLifecycle },
	setID:             func(v *models.SystemVersion, id string) { v.ID = id },
	setRecordID:       func(v *models.SystemVersion, id string) { v.RecordID = id },
	setVersion:        func(v *models.SystemVersion, n int) { v.Version = n },
	recordID:          func(v *models.SystemVersion) string { return v.RecordID },
	displayName:       func(v *models.SystemVersion) string { return v.GameSystem },
	fields: map[string]entityFieldAccessor[models.SystemVersion]{
		"game_system": {get: func(v *models.SystemVersion) any { return v.GameSystem }, set: func(v *models.SystemVersion, val any) { v.GameSystem = val.(string) }},
		"edition":     {get: func(v *models.SystemVersion) any { return v.Edition }, set: func(v *models.SystemVersion, val any) { v.Edition = val.(string) }},
		"notes":       {get: func(v *models.SystemVersion) any { return v.Notes }, set: func(v *models.SystemVersion, val any) { v.Notes = val.(string) }},
		"tags":        {get: func(v *models.SystemVersion) any { return v.Tags }, set: func(v *models.SystemVersion, val any) { v.Tags = val.([]modelcore.Tag) }},
	},
}

// EnsureSystemVersioningIndexes creates the indexes system version queries rely on. Safe to call
// on every startup.
func EnsureSystemVersioningIndexes(c context.Context) error {
	return systemVersioning.ensureIndexes(c)
}

// SoftDeleteSystem hides a system from every Query*/List* read without touching its version
// history - see design.md's soft-delete decision.
func SoftDeleteSystem(c context.Context, id string, deletedBy string) error {
	return systemVersioning.softDelete(c, id, deletedBy)
}

// RestoreSystem clears a soft-deleted system's deletion, returning it to every normal read path.
func RestoreSystem(c context.Context, id string) error {
	return systemVersioning.restore(c, id)
}

func systemVersionToVO(version *models.SystemVersion) *vo.SystemVersionVO {
	return &vo.SystemVersionVO{
		ID: version.ID, RecordID: version.RecordID, Version: version.Version,
		GameSystem: version.GameSystem, Edition: version.Edition, Notes: version.Notes,
		Tags: modelcoreutil.FromTagModels(version.Tags),
		VersionLifecycleVO: vo.VersionLifecycleVO{
			State: vo.VersionState(version.State), BaseVersion: version.BaseVersion,
			SubmittedBy: version.SubmittedBy, SubmittedAt: version.SubmittedAt,
			ReviewedBy: version.ReviewedBy, ReviewedAt: version.ReviewedAt,
			ReviewNote: version.ReviewNote, ResultingVersion: version.ResultingVersion,
		},
	}
}

func systemVersionFields(system *vo.SystemVO) models.SystemVersion {
	return models.SystemVersion{
		GameSystem: system.GameSystem, Edition: system.Edition, Notes: system.Notes,
		Tags: modelcoreutil.ToTagModels(system.Tags),
	}
}

func flattenSystem(meta *models.EntityMeta, version *models.SystemVersion) *vo.SystemVO {
	return &vo.SystemVO{
		ID: meta.ID, GameSystem: version.GameSystem, Edition: version.Edition, Notes: version.Notes,
		Tags: modelcoreutil.FromTagModels(version.Tags),
		AuditableVO: modelcorevo.AuditableVO{
			CreatedAt: meta.CreatedAt, CreatedBy: meta.CreatedBy,
			UpdatedAt: version.SubmittedAt, UpdatedBy: version.SubmittedBy,
			DeletedAt: meta.DeletedAt, DeletedBy: meta.DeletedBy,
		},
	}
}

// AddSystem creates a system's meta record and its first (live) version.
func AddSystem(c context.Context, system *vo.SystemVO) (*string, error) {
	version := systemVersionFields(system)
	return systemVersioning.addEntity(c, &version, system.CreatedBy)
}

// GetSystem returns the flattened view of a system.
func GetSystem(c context.Context, id string) (*vo.SystemVO, error) {
	meta, err := systemVersioning.getMeta(c, id)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, nil
	}
	version, err := systemVersioning.getVersion(c, id, meta.CurrentVersion)
	if err != nil {
		return nil, err
	}
	if version == nil {
		return nil, nil
	}
	return flattenSystem(meta, version), nil
}

// UpdateSystem creates a new version of the system rather than mutating in place.
func UpdateSystem(c context.Context, id string, system *vo.SystemVO, state models.VersionState) (*vo.SystemVersionVO, error) {
	version := systemVersionFields(system)
	result, err := systemVersioning.createVersion(c, id, &version, state, system.UpdatedBy)
	if err != nil || result == nil {
		return nil, err
	}
	return systemVersionToVO(result), nil
}

// ListSystemVersions returns every version of a system, newest first.
func ListSystemVersions(c context.Context, id string) ([]*vo.SystemVersionVO, error) {
	versions, err := systemVersioning.listVersions(c, id)
	if err != nil {
		return nil, err
	}
	vos := make([]*vo.SystemVersionVO, 0, len(versions))
	for _, v := range versions {
		vos = append(vos, systemVersionToVO(v))
	}
	return vos, nil
}

// GetSystemVersion returns one version's full snapshot.
func GetSystemVersion(c context.Context, id string, version int) (*vo.SystemVersionVO, error) {
	result, err := systemVersioning.getVersion(c, id, version)
	if err != nil || result == nil {
		return nil, err
	}
	return systemVersionToVO(result), nil
}

// AcceptSystemVersion reviews a submitted version - see AcceptVolumeVersion's doc comment.
func AcceptSystemVersion(c context.Context, id string, version int, selectedFields []string, reviewedBy string, reviewNote *string) (*vo.SystemVersionVO, []string, error) {
	result, conflicts, err := systemVersioning.acceptVersion(c, id, version, selectedFields, reviewedBy, reviewNote)
	if err != nil || result == nil {
		return nil, conflicts, err
	}
	return systemVersionToVO(result), conflicts, nil
}

// RejectSystemVersion marks a submitted version rejected.
func RejectSystemVersion(c context.Context, id string, version int, reviewedBy string, reviewNote *string) error {
	return systemVersioning.rejectVersion(c, id, version, reviewedBy, reviewNote)
}

// RetractSystemVersion lets the original submitter withdraw their own pending submission.
func RetractSystemVersion(c context.Context, id string, version int, submitterID string) (*vo.SystemVersionVO, error) {
	result, err := systemVersioning.retractVersion(c, id, version, submitterID)
	if err != nil || result == nil {
		return nil, err
	}
	return systemVersionToVO(result), nil
}

// SetCurrentSystemVersion rolls a system back (or forward) to an arbitrary existing version.
func SetCurrentSystemVersion(c context.Context, id string, version int) (*vo.SystemVersionVO, error) {
	result, err := systemVersioning.setCurrentVersion(c, id, version)
	if err != nil || result == nil {
		return nil, err
	}
	return systemVersionToVO(result), nil
}

// CountSubmittedSystemVersionsBySubmitter counts a submitter's currently-pending versions.
func CountSubmittedSystemVersionsBySubmitter(c context.Context, submittedBy string) (int64, error) {
	return systemVersioning.countSubmittedBySubmitter(c, submittedBy)
}

// QuerySystems lists the current (live) version of every system matching params.
func QuerySystems(c context.Context, params apiutil.QueryParams) ([]*vo.SystemVO, error) {
	filter, sort, projection := apiutil.ConvertQueryParams(params)
	filter = append(filter, bson.E{Key: "deleted_at", Value: nil})
	metas, err := database.Query[models.EntityMeta](systemMetaCollection, filter, sort, projection, params.Start, params.Limit)
	if err != nil {
		return nil, err
	}
	vos := make([]*vo.SystemVO, 0, len(metas))
	for _, meta := range metas {
		version, err := systemVersioning.getVersion(c, meta.ID, meta.CurrentVersion)
		if err != nil {
			return nil, err
		}
		if version == nil {
			continue
		}
		vos = append(vos, flattenSystem(meta, version))
	}
	return vos, nil
}

// MigrateSystems backfills every existing "systems" document into a meta record plus a single
// live version - see MigratePublishers' doc comment. System never had a write path before this
// change, so there's no pending-proposal migration counterpart to run alongside this.
func MigrateSystems(c context.Context) (int, error) {
	return migrateEntity(c, migrationConfig[models.System, models.SystemVersion]{
		oldCollection: "systems",
		versioning:    systemVersioning,
		id:            func(s *models.System) string { return s.ID },
		auditable:     func(s *models.System) modelcore.Auditable { return s.Auditable },
		toVersion: func(s *models.System) models.SystemVersion {
			return models.SystemVersion{GameSystem: s.GameSystem, Edition: s.Edition, Notes: s.Notes, Tags: s.Tags}
		},
	})
}

// GetSystemStats returns the systems landing-page-summary card's count/most-recent data.
func GetSystemStats(c context.Context) (*TypeStats, error) {
	return systemVersioning.stats(c)
}
