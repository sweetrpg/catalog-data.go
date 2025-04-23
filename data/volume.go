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
	"github.com/sweetrpg/common.go/util"
	"github.com/sweetrpg/db.go/database"
	modelcore "github.com/sweetrpg/model-core.go/models"
	modelcoreutil "github.com/sweetrpg/model-core.go/util"
	modelcorevo "github.com/sweetrpg/model-core.go/vo"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func AddVolume(c context.Context, volume *vo.VolumeVO) (*string, error) {
	_, span := otel.Tracer("volume").Start(c, "db-add-volume", oteltrace.WithAttributes())

	properties := modelcoreutil.ToPropertyModels(volume.Properties)
	tags := modelcoreutil.ToTagModels(volume.Tags)
	systemIds := util.Map[vo.SystemVO, string](volume.Systems, func(system vo.SystemVO) *string {
		return &system.ID
	})
	publisherIds := util.Map[vo.PublisherVO, string](volume.Publishers, func(publisher vo.PublisherVO) *string {
		return &publisher.ID
	})
	studioIds := util.Map[vo.StudioVO, string](volume.Studios, func(studio vo.StudioVO) *string {
		return &studio.ID
	})
	licenseIds := util.Map[vo.LicenseVO, string](volume.Licenses, func(license vo.LicenseVO) *string {
		return &license.ID
	})
	now := time.Now()
	model := models.Volume{
		Title:        volume.Title,
		Description:  volume.Description,
		Notes:        volume.Notes,
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
	id, err := database.Insert[models.Volume]("volumes", model)
	if err != nil {
		logging.Logger.Error("Error while inserting Volume object", "error", err)
		return nil, err
	}

	// TODO

	span.End()

	idStr := id.Hex()
	return &idStr, nil
}

func UpdateVolume(c context.Context, id string, volume *vo.VolumeVO) (*vo.VolumeVO, error) {
	_, span := otel.Tracer("volume").Start(c, "db-update-volume", oteltrace.WithAttributes(attribute.String("id", id)))

	// TODO

	span.End()

	return nil, nil
}

func DeleteVolume(c context.Context, id string) error {
	_, span := otel.Tracer("volume").Start(c, "db-delete-volume", oteltrace.WithAttributes(attribute.String("id", id)))

	// TODO

	span.End()

	return nil
}

func GetVolume(c context.Context, id string) (*vo.VolumeVO, error) {
	_, span := otel.Tracer("volume").Start(c, "db-get-volume", oteltrace.WithAttributes(attribute.String("id", id)))
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		logging.Logger.Error("Error while converting object ID for Volume", "error", err)
		return nil, err
	}
	model, err := database.Get[models.Volume]("volumes", objectId)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Volume: %+v", err))
		return nil, err
	}

	if model == nil {
		logging.Logger.Info(fmt.Sprintf("Volume not found for ID: %s", id))
		return nil, nil
	}

	systemVOs := util.Map[string, vo.SystemVO](model.SystemIds, func(id string) *vo.SystemVO {
		vo, err := GetSystem(c, id)
		if err != nil {
			logging.Logger.Error(fmt.Sprintf("No System found from Volume for ID %s: %s", id, err.Error()))
		}
		return vo
	})
	publisherVOs := util.Map[string, vo.PublisherVO](model.PublisherIds, func(id string) *vo.PublisherVO {
		vo, err := GetPublisher(c, id)
		if err != nil {
			logging.Logger.Error(fmt.Sprintf("No Publisher found from Volume for ID %s: %s", id, err.Error()))
		}
		return vo
	})
	studioVOs := util.Map[string, vo.StudioVO](model.StudioIds, func(id string) *vo.StudioVO {
		vo, err := GetStudio(c, id)
		if err != nil {
			logging.Logger.Error(fmt.Sprintf("No Studio found from Volume for ID %s: %s", id, err.Error()))
		}
		return vo
	})
	licenseVOs := util.Map[string, vo.LicenseVO](model.LicenseIds, func(id string) *vo.LicenseVO {
		vo, err := GetLicense(c, id)
		if err != nil {
			logging.Logger.Error(fmt.Sprintf("No License found from Volume for ID %s: %s", id, err.Error()))
		}
		return vo
	})

	return &vo.VolumeVO{
		ID:          model.ID,
		Title:       model.Title,
		Description: model.Description,
		Notes:       model.Notes,
		Systems:     systemVOs,
		Publishers:  publisherVOs,
		Studios:     studioVOs,
		Licenses:    licenseVOs,
		Properties:  modelcorevo.FromPropertyModels(model.Properties),
		Tags:        modelcorevo.FromTagModels(model.Tags),
		AuditableVO: modelcorevo.AuditableVO{
			CreatedAt: model.CreatedAt,
			CreatedBy: model.CreatedBy,
			UpdatedAt: model.UpdatedAt,
			UpdatedBy: model.UpdatedBy,
			DeletedAt: model.DeletedAt,
			DeletedBy: model.DeletedBy,
		},
	}, nil
}

func QueryVolumes(c context.Context, params apiutil.QueryParams) ([]*vo.VolumeVO, error) {
	span := tracing.BuildSpanWithParams(c, "contributions", "db-get-contributions", params)
	filter, sort, projection := apiutil.ConvertQueryParams(params)
	logging.Logger.Debug("query volumes", "filter", filter, "sort", sort, "projection", projection)
	models, err := database.Query[models.Volume]("volumes", filter, sort, projection, params.Start, params.Limit)
	logging.Logger.Debug("got volumes", "models", models, "err", err)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Volumes: %+v", err))
		return nil, err
	}

	modelCount := len(models)
	if modelCount == 0 {
		// short-circuit if there's nothing to do
		return make([]*vo.VolumeVO, 0), nil
	}

	var vos []*vo.VolumeVO
	for _, model := range models {
		logging.Logger.Debug("processing volume model", "model", model)
		vo, err := GetVolume(c, model.ID)
		logging.Logger.Debug("got volume", "model", model, "vo", vo, "err", err)
		if err != nil {
			logging.Logger.Error(fmt.Sprintf("No Volume found from item in array for ID: %s", model.ID))
			continue
		}
		vos = append(vos, vo)
	}

	logging.Logger.Debug("returning volume value objects", "vos", vos, "err", err)
	return vos, err
}
