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
	modelcore "github.com/sweetrpg/model-core.go/models"
	modelcoreutil "github.com/sweetrpg/model-core.go/util"
	modelcorevo "github.com/sweetrpg/model-core.go/vo"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func AddVolume(c context.Context, volume *vo.VolumeVO) (*string, error) {
	logging.Logger.Info("AddVolume", "c", c, "volume", volume)

	_, span := otel.Tracer("volume").Start(c, "db-add-volume", oteltrace.WithAttributes())

	properties := modelcoreutil.ToPropertyModels(volume.Properties)
	logging.Logger.Debug("ToPropertyModels", "properties", properties)
	tags := modelcoreutil.ToTagModels(volume.Tags)
	logging.Logger.Debug("ToTagModels", "tags", tags)
	systemIds := relationIDs(volume.Systems, func(s *vo.SystemVO) string { return s.ID })
	logging.Logger.Debug("map systems", "systemIds", systemIds)
	publisherIds := relationIDs(volume.Publishers, func(p *vo.PublisherVO) string { return p.ID })
	logging.Logger.Debug("map publishers", "publisherIds", publisherIds)
	studioIds := relationIDs(volume.Studios, func(s *vo.StudioVO) string { return s.ID })
	logging.Logger.Debug("map studios", "studioIds", studioIds)
	licenseIds := relationIDs(volume.Licenses, func(l *vo.LicenseVO) string { return l.ID })
	logging.Logger.Debug("map licenses", "licenseIds", licenseIds)

	now := time.Now()
	model := models.Volume{
		ID:           primitive.NewObjectID().Hex(),
		Title:        volume.Title,
		Description:  volume.Description,
		Notes:        volume.Notes,
		Format:       volume.Format,
		Properties:   properties,
		Tags:         tags,
		SystemIds:    systemIds,
		PublisherIds: publisherIds,
		StudioIds:    studioIds,
		LicenseIds:   licenseIds,
		Auditable: modelcore.Auditable{
			CreatedAt: now,
			CreatedBy: volume.CreatedBy,
			UpdatedAt: now,
			UpdatedBy: volume.UpdatedBy,
			DeletedAt: nil,
			DeletedBy: nil,
		},
	}
	logging.Logger.Debug("Volume model", "model", model)

	_, err := database.Insert[models.Volume]("volumes", model)
	logging.Logger.Debug("Inserted Volume", "id", model.ID, "err", err)
	if err != nil {
		logging.Logger.Error("Error while inserting Volume object", "error", err)
		return nil, err
	}

	span.End()

	return &model.ID, nil
}

func UpdateVolume(c context.Context, id string, volume *vo.VolumeVO) (*vo.VolumeVO, error) {
	logging.Logger.Info("UpdateVolume", "c", c, "id", id, "volume", volume)

	_, span := otel.Tracer("volume").Start(c, "db-update-volume", oteltrace.WithAttributes(attribute.String("id", id)))
	defer span.End()

	existing, err := GetVolume(c, id)
	if err != nil {
		logging.Logger.Error("Error while looking up existing Volume for update", "id", id, "error", err)
		return nil, err
	}
	if existing == nil {
		logging.Logger.Info(fmt.Sprintf("Volume not found for update, ID: %s", id))
		return nil, nil
	}

	properties := modelcoreutil.ToPropertyModels(volume.Properties)
	tags := modelcoreutil.ToTagModels(volume.Tags)
	systemIds := relationIDs(volume.Systems, func(s *vo.SystemVO) string { return s.ID })
	publisherIds := relationIDs(volume.Publishers, func(p *vo.PublisherVO) string { return p.ID })
	studioIds := relationIDs(volume.Studios, func(s *vo.StudioVO) string { return s.ID })
	licenseIds := relationIDs(volume.Licenses, func(l *vo.LicenseVO) string { return l.ID })

	model := models.Volume{
		ID:           id,
		Title:        volume.Title,
		Description:  volume.Description,
		Notes:        volume.Notes,
		Format:       volume.Format,
		Properties:   properties,
		Tags:         tags,
		SystemIds:    systemIds,
		PublisherIds: publisherIds,
		StudioIds:    studioIds,
		LicenseIds:   licenseIds,
		Auditable: modelcore.Auditable{
			CreatedAt: existing.CreatedAt,
			CreatedBy: existing.CreatedBy,
			UpdatedAt: time.Now(),
			UpdatedBy: volume.UpdatedBy,
			DeletedAt: nil,
			DeletedBy: nil,
		},
	}
	logging.Logger.Debug("Volume model for update", "model", model)

	data, err := bson.Marshal(model)
	if err != nil {
		logging.Logger.Error("Error while preparing Volume document for update", "error", err)
		return nil, err
	}
	var update bson.D
	if err := bson.Unmarshal(data, &update); err != nil {
		logging.Logger.Error("Error while unmarshaling Volume document for update", "error", err)
		return nil, err
	}

	result, err := database.Db.Collection("volumes").UpdateOne(
		c,
		bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$set", Value: update}},
	)
	logging.Logger.Debug("update result", "result", result, "err", err)
	if err != nil {
		logging.Logger.Error("Error while updating Volume in database", "id", id, "error", err)
		return nil, err
	}
	if result.MatchedCount == 0 {
		logging.Logger.Info(fmt.Sprintf("Volume not found for update, ID: %s", id))
		return nil, nil
	}

	return GetVolume(c, id)
}

func DeleteVolume(c context.Context, id string) error {
	_, span := otel.Tracer("volume").Start(c, "db-delete-volume", oteltrace.WithAttributes(attribute.String("id", id)))

	// TODO

	span.End()

	return nil
}

func GetVolume(c context.Context, id string) (*vo.VolumeVO, error) {
	_, span := otel.Tracer("volume").Start(c, "db-get-volume", oteltrace.WithAttributes(attribute.String("id", id)))
	results, err := database.Query[models.Volume]("volumes", bson.D{{Key: "_id", Value: id}}, nil, nil, 0, 1)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Volume: %+v", err))
		return nil, err
	}

	if len(results) == 0 {
		logging.Logger.Info(fmt.Sprintf("Volume not found for ID: %s", id))
		return nil, nil
	}

	return volumeModelToVO(c, results[0]), nil
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

func volumeModelToVO(c context.Context, model *models.Volume) *vo.VolumeVO {
	systemVOs := make([]*vo.SystemVO, 0, len(model.SystemIds))
	for _, id := range model.SystemIds {
		system, err := GetSystem(c, id)
		if err != nil {
			logging.Logger.Error(fmt.Sprintf("No System found from Volume for ID %s: %s", id, err.Error()))
			continue
		}
		if system != nil {
			systemVOs = append(systemVOs, system)
		}
	}
	publisherVOs := make([]*vo.PublisherVO, 0, len(model.PublisherIds))
	for _, id := range model.PublisherIds {
		publisher, err := GetPublisher(c, id)
		if err != nil {
			logging.Logger.Error(fmt.Sprintf("No Publisher found from Volume for ID %s: %s", id, err.Error()))
			continue
		}
		if publisher != nil {
			publisherVOs = append(publisherVOs, publisher)
		}
	}
	studioVOs := make([]*vo.StudioVO, 0, len(model.StudioIds))
	for _, id := range model.StudioIds {
		studio, err := GetStudio(c, id)
		if err != nil {
			logging.Logger.Error(fmt.Sprintf("No Studio found from Volume for ID %s: %s", id, err.Error()))
			continue
		}
		if studio != nil {
			studioVOs = append(studioVOs, studio)
		}
	}
	licenseVOs := make([]*vo.LicenseVO, 0, len(model.LicenseIds))
	for _, id := range model.LicenseIds {
		license, err := GetLicense(c, id)
		if err != nil {
			logging.Logger.Error(fmt.Sprintf("No License found from Volume for ID %s: %s", id, err.Error()))
			continue
		}
		if license != nil {
			licenseVOs = append(licenseVOs, license)
		}
	}

	return &vo.VolumeVO{
		ID:          model.ID,
		Title:       model.Title,
		Description: model.Description,
		Notes:       model.Notes,
		Format:      model.Format,
		Systems:     systemVOs,
		Publishers:  publisherVOs,
		Studios:     studioVOs,
		Licenses:    licenseVOs,
		Properties:  modelcoreutil.FromPropertyModels(model.Properties),
		Tags:        modelcoreutil.FromTagModels(model.Tags),
		AuditableVO: modelcorevo.AuditableVO{
			CreatedAt: model.CreatedAt,
			CreatedBy: model.CreatedBy,
			UpdatedAt: model.UpdatedAt,
			UpdatedBy: model.UpdatedBy,
			DeletedAt: model.DeletedAt,
			DeletedBy: model.DeletedBy,
		},
	}
}

func QueryVolumes(c context.Context, params apiutil.QueryParams) ([]*vo.VolumeVO, error) {
	logging.Logger.Info("QueryVolumes", "c", c, "params", params)

	span := tracing.BuildSpanWithParams(c, "volumes", "db-get-volumes", params)
	logging.Logger.Debug("query volumes", "span", span)

	filter, sort, projection := apiutil.ConvertQueryParams(params)
	logging.Logger.Debug("query volumes", "filter", filter, "sort", sort, "projection", projection)
	models, err := database.Query[models.Volume]("volumes", filter, sort, projection, params.Start, params.Limit)
	logging.Logger.Debug("got volumes", "models", models, "err", err)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Volumes: %+v", err))
		return nil, err
	}

	vos := make([]*vo.VolumeVO, 0, len(models))
	for _, model := range models {
		logging.Logger.Debug("processing volume model", "model", model)
		vos = append(vos, volumeModelToVO(c, model))
	}

	logging.Logger.Debug("returning volume value objects", "vos", vos)
	return vos, nil
}
