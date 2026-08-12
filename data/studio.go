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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func GetStudio(c context.Context, id string) (*vo.StudioVO, error) {
	_, span := otel.Tracer("studio").Start(c, "db-get-studio", oteltrace.WithAttributes(attribute.String("id", id)))
	results, err := database.Query[models.Studio]("studios", bson.D{{Key: "_id", Value: id}}, nil, nil, 0, 1)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Studio: %v", err))
		return nil, err
	}

	if len(results) == 0 {
		logging.Logger.Info(fmt.Sprintf("Studio not found for ID: %s", id))
		return nil, nil
	}

	return studioModelToVO(results[0]), nil
}

func studioModelToVO(model *models.Studio) *vo.StudioVO {
	return &vo.StudioVO{
		ID:         model.ID,
		Name:       model.Name,
		Website:    model.Website,
		Notes:      model.Notes,
		Properties: modelcoreutil.FromPropertyModels(model.Properties),
		Tags:       modelcoreutil.FromTagModels(model.Tags),
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

// UpdateStudio merges the provided fields into the existing studio and writes it, mirroring
// UpdatePublisher's raw-marshal $set approach.
func UpdateStudio(c context.Context, id string, studio *vo.StudioVO) (*vo.StudioVO, error) {
	_, span := otel.Tracer("studio").Start(c, "db-update-studio", oteltrace.WithAttributes(attribute.String("id", id)))
	defer span.End()

	existing, err := GetStudio(c, id)
	if err != nil {
		logging.Logger.Error("Error while looking up existing Studio for update", "id", id, "error", err)
		return nil, err
	}
	if existing == nil {
		logging.Logger.Info(fmt.Sprintf("Studio not found for update, ID: %s", id))
		return nil, nil
	}

	model := models.Studio{
		ID:         id,
		Name:       studio.Name,
		Website:    studio.Website,
		Notes:      studio.Notes,
		Properties: modelcoreutil.ToPropertyModels(studio.Properties),
		Tags:       modelcoreutil.ToTagModels(studio.Tags),
		Auditable: modelcore.Auditable{
			CreatedAt: existing.CreatedAt,
			CreatedBy: existing.CreatedBy,
			UpdatedAt: time.Now(),
			UpdatedBy: studio.UpdatedBy,
			DeletedAt: nil,
			DeletedBy: nil,
		},
	}

	data, err := bson.Marshal(model)
	if err != nil {
		logging.Logger.Error("Error while preparing Studio document for update", "error", err)
		return nil, err
	}
	var update bson.D
	if err := bson.Unmarshal(data, &update); err != nil {
		logging.Logger.Error("Error while unmarshaling Studio document for update", "error", err)
		return nil, err
	}

	result, err := database.Db.Collection("studios").UpdateOne(
		c,
		bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$set", Value: update}},
	)
	if err != nil {
		logging.Logger.Error("Error while updating Studio in database", "id", id, "error", err)
		return nil, err
	}
	if result.MatchedCount == 0 {
		logging.Logger.Info(fmt.Sprintf("Studio not found for update, ID: %s", id))
		return nil, nil
	}

	return GetStudio(c, id)
}

func QueryStudios(c context.Context, params apiutil.QueryParams) ([]*vo.StudioVO, error) {
	span := tracing.BuildSpanWithParams(c, "contributions", "db-get-contributions", params)
	filter, sort, projection := apiutil.ConvertQueryParams(params)
	models, err := database.Query[models.Studio]("studios", filter, sort, projection, params.Start, params.Limit)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Studios: %v", err))
		return nil, err
	}

	vos := make([]*vo.StudioVO, 0, len(models))
	for _, model := range models {
		vos = append(vos, studioModelToVO(model))
	}

	return vos, nil
}
