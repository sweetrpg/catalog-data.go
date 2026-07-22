package data

import (
	"context"
	"fmt"

	"github.com/sweetrpg/api-core.go/tracing"
	apiutil "github.com/sweetrpg/api-core.go/util"
	"github.com/sweetrpg/catalog-objects.go/models"
	"github.com/sweetrpg/catalog-objects.go/vo"
	"github.com/sweetrpg/common.go/logging"
	modelcoreutil "github.com/sweetrpg/model-core.go/util"
	modelcorevo "github.com/sweetrpg/model-core.go/vo"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func GetSystem(c context.Context, id string) (*vo.SystemVO, error) {
	_, span := otel.Tracer("system").Start(c, "db-get-system", oteltrace.WithAttributes(attribute.String("id", id)))
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		logging.Logger.Error("Error while converting object ID for Contribution", "error", err)
		return nil, err
	}
	model, err := database.Get[models.System]("systems", objectId)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for System: %v", err))
		return nil, err
	}

	if model == nil {
		logging.Logger.Info(fmt.Sprintf("System not found for ID: %s", id))
		return nil, nil
	}

	return systemModelToVO(model), nil
}

func systemModelToVO(model *models.System) *vo.SystemVO {
	return &vo.SystemVO{
		ID:         model.ID,
		GameSystem: model.GameSystem,
		Edition:    model.Edition,
		Notes:      model.Notes,
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

func QuerySystems(c context.Context, params apiutil.QueryParams) ([]*vo.SystemVO, error) {
	span := tracing.BuildSpanWithParams(c, "contributions", "db-get-contributions", params)
	filter, sort, projection := apiutil.ConvertQueryParams(params)
	models, err := database.Query[models.System]("systems", filter, sort, projection, params.Start, params.Limit)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Systems: %v", err))
		return nil, err
	}

	vos := make([]*vo.SystemVO, 0, len(models))
	for _, model := range models {
		vos = append(vos, systemModelToVO(model))
	}

	return vos, nil
}
