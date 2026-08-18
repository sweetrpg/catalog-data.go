package data

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/sweetrpg/catalog-objects.go/models"
	modelcore "github.com/sweetrpg/model-core.go/models"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// entityFieldAccessor reads/writes one substantive field on a version record - the version-model
// counterpart of server/entity_patch.go's entityFieldAccessor, used here for the accept-selected
// diff/apply logic instead of a live-record patch.
type entityFieldAccessor[T any] struct {
	get func(*T) any
	set func(*T, any)
}

// entityVersioningConfig wires the generic meta+version engine below (shared across publisher,
// studio, person, license, and system - the same meta-record+version-record model volume.go/
// volume_versioning.go implement bespoke, predating this shared type) to one entity type's
// collections and field accessors. See design.md's "share a generic version-store engine"
// decision - this avoids reimplementing the version lifecycle five times. Volume itself is not
// migrated onto this engine - its bespoke implementation is already shipped and tested, and it
// alone needs staged-asset handling this generic engine doesn't support.
type entityVersioningConfig[T any] struct {
	metaCollection    string
	versionCollection string
	typeName          string // for error messages, e.g. "publisher"
	lifecycle         func(*T) *models.VersionLifecycle
	setID             func(*T, string)
	setRecordID       func(*T, string)
	setVersion        func(*T, int)
	// fields are the substantive fields a submission can change and a review can selectively
	// accept - everything on T except its version-lifecycle bookkeeping.
	fields map[string]entityFieldAccessor[T]
}

func (cfg entityVersioningConfig[T]) ensureIndexes(c context.Context) error {
	_, err := database.Db.Collection(cfg.versionCollection).Indexes().CreateOne(c, mongo.IndexModel{
		Keys:    bson.D{{Key: "record_id", Value: 1}, {Key: "version", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("%s: create record_id+version index: %w", cfg.typeName, err)
	}
	_, err = database.Db.Collection(cfg.versionCollection).Indexes().CreateOne(c, mongo.IndexModel{
		Keys: bson.D{{Key: "record_id", Value: 1}, {Key: "state", Value: 1}},
	})
	if err != nil {
		return fmt.Errorf("%s: create record_id+state index: %w", cfg.typeName, err)
	}
	return nil
}

func (cfg entityVersioningConfig[T]) getMeta(c context.Context, id string) (*models.EntityMeta, error) {
	results, err := database.Query[models.EntityMeta](cfg.metaCollection, bson.D{{Key: "_id", Value: id}}, nil, nil, 0, 1)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

func (cfg entityVersioningConfig[T]) getVersion(c context.Context, recordID string, version int) (*T, error) {
	filter := bson.D{{Key: "record_id", Value: recordID}, {Key: "version", Value: version}}
	results, err := database.Query[T](cfg.versionCollection, filter, nil, nil, 0, 1)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

func (cfg entityVersioningConfig[T]) setVersionState(c context.Context, recordID string, version int, fields bson.D) error {
	filter := bson.D{{Key: "record_id", Value: recordID}, {Key: "version", Value: version}}
	_, err := database.Db.Collection(cfg.versionCollection).UpdateOne(c, filter, bson.D{{Key: "$set", Value: fields}})
	return err
}

func (cfg entityVersioningConfig[T]) archiveVersion(c context.Context, recordID string, version int) error {
	return cfg.setVersionState(c, recordID, version, bson.D{{Key: "state", Value: string(models.VersionStateArchived)}})
}

func (cfg entityVersioningConfig[T]) setMetaCurrentVersion(c context.Context, recordID string, version int) error {
	_, err := database.Db.Collection(cfg.metaCollection).UpdateOne(
		c,
		bson.D{{Key: "_id", Value: recordID}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "current_version", Value: version}}}},
	)
	return err
}

// listVersions returns every version of a record, newest first.
func (cfg entityVersioningConfig[T]) listVersions(c context.Context, id string) ([]*T, error) {
	filter := bson.D{{Key: "record_id", Value: id}}
	sortOrder := bson.D{{Key: "version", Value: -1}}
	return database.Query[T](cfg.versionCollection, filter, sortOrder, nil, 0, 0)
}

func (cfg entityVersioningConfig[T]) fieldValue(v *T, field string) any {
	accessor, ok := cfg.fields[field]
	if !ok {
		return nil
	}
	return accessor.get(v)
}

func (cfg entityVersioningConfig[T]) setFieldValue(v *T, field string, value any) {
	if accessor, ok := cfg.fields[field]; ok {
		accessor.set(v, value)
	}
}

func (cfg entityVersioningConfig[T]) changedFields(submitted, base *T) []string {
	var changed []string
	for field := range cfg.fields {
		if !reflect.DeepEqual(cfg.fieldValue(submitted, field), cfg.fieldValue(base, field)) {
			changed = append(changed, field)
		}
	}
	sort.Strings(changed)
	return changed
}

// addEntity creates a record's meta record and its first (live) version. entity must already
// carry its substantive field values; addEntity sets id/record_id/version/lifecycle fields.
func (cfg entityVersioningConfig[T]) addEntity(c context.Context, entity *T, createdBy string) (*string, error) {
	now := time.Now()
	metaID := primitive.NewObjectID().Hex()
	meta := models.EntityMeta{ID: metaID, CurrentVersion: 1, CreatedAt: now, CreatedBy: createdBy}
	if _, err := database.Insert[models.EntityMeta](cfg.metaCollection, meta); err != nil {
		return nil, err
	}

	cfg.setID(entity, primitive.NewObjectID().Hex())
	cfg.setRecordID(entity, metaID)
	cfg.setVersion(entity, 1)
	lc := cfg.lifecycle(entity)
	lc.State = models.VersionStateLive
	lc.BaseVersion = nil
	lc.SubmittedBy = createdBy
	lc.SubmittedAt = now

	if _, err := database.Insert[T](cfg.versionCollection, *entity); err != nil {
		return nil, err
	}
	return &metaID, nil
}

// createVersion creates a new version for an existing record - editor/admin: state Live, goes
// current immediately and archives the previous current version; submitter: state Submitted,
// current pointer untouched. entity must already carry its desired substantive field values.
func (cfg entityVersioningConfig[T]) createVersion(c context.Context, id string, entity *T, state models.VersionState, submittedBy string) (*T, error) {
	return cfg.createVersionWithSubmission(c, id, entity, state, submittedBy, time.Now())
}

// createVersionWithSubmission is createVersion with a caller-supplied submittedAt instead of
// always stamping "now" - used by migration, which preserves a migrated proposal's original
// submission time rather than the time of the migration run itself.
func (cfg entityVersioningConfig[T]) createVersionWithSubmission(c context.Context, id string, entity *T, state models.VersionState, submittedBy string, submittedAt time.Time) (*T, error) {
	meta, err := cfg.getMeta(c, id)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, nil
	}

	nextVersion, err := cfg.nextVersionNumber(c, id)
	if err != nil {
		return nil, err
	}

	baseVersion := meta.CurrentVersion
	cfg.setID(entity, primitive.NewObjectID().Hex())
	cfg.setRecordID(entity, id)
	cfg.setVersion(entity, nextVersion)
	lc := cfg.lifecycle(entity)
	lc.State = state
	lc.BaseVersion = &baseVersion
	lc.SubmittedBy = submittedBy
	lc.SubmittedAt = submittedAt

	if _, err := database.Insert[T](cfg.versionCollection, *entity); err != nil {
		return nil, err
	}

	if state == models.VersionStateLive {
		if err := cfg.archiveVersion(c, id, meta.CurrentVersion); err != nil {
			return nil, err
		}
		if err := cfg.setMetaCurrentVersion(c, id, nextVersion); err != nil {
			return nil, err
		}
	}

	return entity, nil
}

// nextVersionNumber returns the next sequential version number for a record - 1 if it has none yet.
func (cfg entityVersioningConfig[T]) nextVersionNumber(c context.Context, recordID string) (int, error) {
	filter := bson.D{{Key: "record_id", Value: recordID}}
	sortOrder := bson.D{{Key: "version", Value: -1}}
	results, err := database.Query[T](cfg.versionCollection, filter, sortOrder, nil, 0, 1)
	if err != nil {
		return 0, err
	}
	if len(results) == 0 {
		return 1, nil
	}
	return cfg.versionOf(results[0]) + 1, nil
}

func (cfg entityVersioningConfig[T]) versionOf(v *T) int {
	// Version is always the second config-set field; read it back via the fields the caller
	// already exposed through setVersion's counterpart is unnecessary - version numbers are
	// looked up via the raw bson document's own "version" key by the query filter/sort, so a
	// dedicated getter is simplest here.
	rv := reflect.ValueOf(v).Elem().FieldByName("Version")
	return int(rv.Int())
}

// acceptVersion reviews a submitted version - see AcceptVolumeVersion's doc comment for the full
// accept-all/accept-selected/conflict semantics this mirrors exactly, minus staged-asset handling.
func (cfg entityVersioningConfig[T]) acceptVersion(c context.Context, id string, version int, selectedFields []string, reviewedBy string, reviewNote *string) (*T, []string, error) {
	submitted, err := cfg.getVersion(c, id, version)
	if err != nil {
		return nil, nil, err
	}
	if submitted == nil {
		return nil, nil, fmt.Errorf("%s %s: version %d not found", cfg.typeName, id, version)
	}
	submittedLC := cfg.lifecycle(submitted)
	if submittedLC.State != models.VersionStateSubmitted {
		return nil, nil, fmt.Errorf("%s %s: version %d is not submitted (state: %s)", cfg.typeName, id, version, submittedLC.State)
	}
	if submittedLC.BaseVersion == nil {
		return nil, nil, fmt.Errorf("%s %s: version %d has no base version to review against", cfg.typeName, id, version)
	}

	meta, err := cfg.getMeta(c, id)
	if err != nil {
		return nil, nil, err
	}
	if meta == nil {
		return nil, nil, fmt.Errorf("%s %s: meta record not found", cfg.typeName, id)
	}

	current, err := cfg.getVersion(c, id, meta.CurrentVersion)
	if err != nil {
		return nil, nil, err
	}
	if current == nil {
		return nil, nil, fmt.Errorf("%s %s: current version %d not found", cfg.typeName, id, meta.CurrentVersion)
	}

	now := time.Now()

	if selectedFields == nil && *submittedLC.BaseVersion == meta.CurrentVersion {
		if err := cfg.archiveVersion(c, id, meta.CurrentVersion); err != nil {
			return nil, nil, err
		}
		submittedLC.State = models.VersionStateLive
		submittedLC.ReviewedBy = &reviewedBy
		submittedLC.ReviewedAt = &now
		submittedLC.ReviewNote = reviewNote
		if err := cfg.setVersionState(c, id, version, bson.D{
			{Key: "state", Value: string(models.VersionStateLive)},
			{Key: "reviewed_by", Value: reviewedBy},
			{Key: "reviewed_at", Value: now},
			{Key: "review_note", Value: reviewNote},
		}); err != nil {
			return nil, nil, err
		}
		if err := cfg.setMetaCurrentVersion(c, id, version); err != nil {
			return nil, nil, err
		}
		return submitted, nil, nil
	}

	baseVersion, err := cfg.getVersion(c, id, *submittedLC.BaseVersion)
	if err != nil {
		return nil, nil, err
	}
	if baseVersion == nil {
		return nil, nil, fmt.Errorf("%s %s: base version %d not found", cfg.typeName, id, *submittedLC.BaseVersion)
	}

	changed := cfg.changedFields(submitted, baseVersion)
	target := changed
	if selectedFields != nil {
		selectedSet := make(map[string]bool, len(selectedFields))
		for _, f := range selectedFields {
			selectedSet[f] = true
		}
		var filtered []string
		for _, f := range changed {
			if selectedSet[f] {
				filtered = append(filtered, f)
			}
		}
		target = filtered
	}

	derived := *current
	var conflicts, accepted []string
	for _, field := range target {
		if !reflect.DeepEqual(cfg.fieldValue(current, field), cfg.fieldValue(baseVersion, field)) {
			conflicts = append(conflicts, field)
			continue
		}
		cfg.setFieldValue(&derived, field, cfg.fieldValue(submitted, field))
		accepted = append(accepted, field)
	}

	nextVersion, err := cfg.nextVersionNumber(c, id)
	if err != nil {
		return nil, nil, err
	}

	cfg.setID(&derived, primitive.NewObjectID().Hex())
	cfg.setRecordID(&derived, id)
	cfg.setVersion(&derived, nextVersion)
	derivedLC := cfg.lifecycle(&derived)
	derivedLC.State = models.VersionStateLive
	derivedLC.BaseVersion = &meta.CurrentVersion
	derivedLC.SubmittedBy = submittedLC.SubmittedBy
	derivedLC.SubmittedAt = now
	derivedLC.ReviewedBy = &reviewedBy
	derivedLC.ReviewedAt = &now
	derivedLC.ReviewNote = reviewNote
	derivedLC.ResultingVersion = nil

	if _, err := database.Insert[T](cfg.versionCollection, derived); err != nil {
		return nil, nil, err
	}
	if err := cfg.archiveVersion(c, id, meta.CurrentVersion); err != nil {
		return nil, nil, err
	}
	if err := cfg.setMetaCurrentVersion(c, id, nextVersion); err != nil {
		return nil, nil, err
	}
	if err := cfg.setVersionState(c, id, version, bson.D{
		{Key: "state", Value: string(models.VersionStatePartiallyAccepted)},
		{Key: "reviewed_by", Value: reviewedBy},
		{Key: "reviewed_at", Value: now},
		{Key: "review_note", Value: reviewNote},
		{Key: "resulting_version", Value: nextVersion},
	}); err != nil {
		return nil, nil, err
	}

	_ = accepted
	return &derived, conflicts, nil
}

// rejectVersion marks a submitted version rejected, with an optional note.
func (cfg entityVersioningConfig[T]) rejectVersion(c context.Context, id string, version int, reviewedBy string, reviewNote *string) error {
	submitted, err := cfg.getVersion(c, id, version)
	if err != nil {
		return err
	}
	if submitted == nil {
		return fmt.Errorf("%s %s: version %d not found", cfg.typeName, id, version)
	}
	if cfg.lifecycle(submitted).State != models.VersionStateSubmitted {
		return fmt.Errorf("%s %s: version %d is not submitted", cfg.typeName, id, version)
	}
	now := time.Now()
	return cfg.setVersionState(c, id, version, bson.D{
		{Key: "state", Value: string(models.VersionStateRejected)},
		{Key: "reviewed_by", Value: reviewedBy},
		{Key: "reviewed_at", Value: now},
		{Key: "review_note", Value: reviewNote},
	})
}

// retractVersion lets the original submitter withdraw their own pending submission.
func (cfg entityVersioningConfig[T]) retractVersion(c context.Context, id string, version int, submitterID string) (*T, error) {
	submitted, err := cfg.getVersion(c, id, version)
	if err != nil {
		return nil, err
	}
	if submitted == nil {
		return nil, fmt.Errorf("%s %s: version %d not found", cfg.typeName, id, version)
	}
	lc := cfg.lifecycle(submitted)
	if lc.State != models.VersionStateSubmitted {
		return nil, fmt.Errorf("%s %s: version %d is not submitted", cfg.typeName, id, version)
	}
	if lc.SubmittedBy != submitterID {
		return nil, fmt.Errorf("%s %s: version %d was not submitted by %s", cfg.typeName, id, version, submitterID)
	}
	if err := cfg.setVersionState(c, id, version, bson.D{{Key: "state", Value: string(models.VersionStateWithdrawn)}}); err != nil {
		return nil, err
	}
	lc.State = models.VersionStateWithdrawn
	return submitted, nil
}

// setCurrentVersion rolls a record back (or forward) to an arbitrary existing version.
func (cfg entityVersioningConfig[T]) setCurrentVersion(c context.Context, id string, version int) (*T, error) {
	meta, err := cfg.getMeta(c, id)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, nil
	}
	target, err := cfg.getVersion(c, id, version)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, fmt.Errorf("%s %s: version %d not found", cfg.typeName, id, version)
	}
	if version == meta.CurrentVersion {
		return target, nil
	}
	if err := cfg.archiveVersion(c, id, meta.CurrentVersion); err != nil {
		return nil, err
	}
	if err := cfg.setVersionState(c, id, version, bson.D{{Key: "state", Value: string(models.VersionStateLive)}}); err != nil {
		return nil, err
	}
	if err := cfg.setMetaCurrentVersion(c, id, version); err != nil {
		return nil, err
	}
	cfg.lifecycle(target).State = models.VersionStateLive
	return target, nil
}

// countSubmittedBySubmitter counts a submitter's currently-pending (state: submitted) versions.
func (cfg entityVersioningConfig[T]) countSubmittedBySubmitter(c context.Context, submittedBy string) (int64, error) {
	filter := bson.D{
		{Key: "submitted_by", Value: submittedBy},
		{Key: "state", Value: string(models.VersionStateSubmitted)},
	}
	return database.Db.Collection(cfg.versionCollection).CountDocuments(c, filter)
}

// migrationConfig wires the generic one-time backfill below (shared across publisher, studio,
// person, license, and system - see MigrateVolumes' doc comment for the bespoke original this
// mirrors) to one entity type's old flat collection and old/new model shapes.
type migrationConfig[Old any, New any] struct {
	oldCollection string
	versioning    entityVersioningConfig[New]
	id            func(*Old) string
	auditable     func(*Old) modelcore.Auditable
	toVersion     func(*Old) New // substantive fields only - lifecycle fields are set generically
}

// migrateEntity backfills every existing document in cfg.oldCollection (the pre-versioning flat
// model) into a meta record plus a single live version, per design.md's Migration Plan.
// Idempotent - a record that already has a meta record (recognized by the same ID) is left
// untouched, so this is safe to re-run after a partial failure.
func migrateEntity[Old any, New any](c context.Context, cfg migrationConfig[Old, New]) (int, error) {
	all, err := database.Query[Old](cfg.oldCollection, bson.D{}, nil, nil, 0, 0)
	if err != nil {
		return 0, fmt.Errorf("migrate %s: query existing documents: %w", cfg.oldCollection, err)
	}

	migrated := 0
	for _, old := range all {
		id := cfg.id(old)
		existing, err := cfg.versioning.getMeta(c, id)
		if err != nil {
			return migrated, err
		}
		if existing != nil {
			continue
		}

		aud := cfg.auditable(old)
		meta := models.EntityMeta{
			ID: id, CurrentVersion: 1, CreatedAt: aud.CreatedAt, CreatedBy: aud.CreatedBy,
			DeletedAt: aud.DeletedAt, DeletedBy: aud.DeletedBy,
		}
		if _, err := database.Insert[models.EntityMeta](cfg.versioning.metaCollection, meta); err != nil {
			return migrated, fmt.Errorf("migrate %s: insert meta for %s: %w", cfg.oldCollection, id, err)
		}

		version := cfg.toVersion(old)
		cfg.versioning.setID(&version, primitive.NewObjectID().Hex())
		cfg.versioning.setRecordID(&version, id)
		cfg.versioning.setVersion(&version, 1)
		lc := cfg.versioning.lifecycle(&version)
		lc.State = models.VersionStateLive
		lc.BaseVersion = nil
		lc.SubmittedBy = aud.UpdatedBy
		lc.SubmittedAt = aud.UpdatedAt
		if _, err := database.Insert[New](cfg.versioning.versionCollection, version); err != nil {
			return migrated, fmt.Errorf("migrate %s: insert version for %s: %w", cfg.oldCollection, id, err)
		}

		migrated++
	}

	return migrated, nil
}
