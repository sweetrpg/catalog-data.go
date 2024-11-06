package data

import (
	"context"
	"fmt"

	"github.com/sweetrpg/api-core/tracing"
	apiutil "github.com/sweetrpg/api-core/util"
	"github.com/sweetrpg/catalog-objects/models"
	"github.com/sweetrpg/catalog-objects/vo"
	"github.com/sweetrpg/common/logging"
	"github.com/sweetrpg/db/database"
	modelcorevo "github.com/sweetrpg/model-core/vo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func GetSystem(c context.Context, id string) (*vo.SystemVO, error) {
	_, span := otel.Tracer("system").Start(c, "db-get-system", oteltrace.WithAttributes(attribute.String("id", id)))
	model, err := database.Get[models.System]("systems", id)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for System: %v", err))
		return nil, err
	}

	if model == nil {
		logging.Logger.Info(fmt.Sprintf("System not found for ID: %s", id))
		return nil, nil
	}

	return &vo.SystemVO{
		ID:         model.ID,
		GameSystem: model.GameSystem,
		Edition:    model.Edition,
		Notes:      model.Notes,
		Tags:       modelcorevo.FromTagModels(model.Tags),
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

func GetSystems(c context.Context, params apiutil.QueryParams) ([]*vo.SystemVO, error) {
	span := tracing.BuildSpanWithParams(c, "contributions", "db-get-contributions", params)
	filter, sort, projection := apiutil.ConvertQueryParams(params)
	models, err := database.Query[models.System]("systems", filter, sort, projection, params.Start, params.Limit)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Systems: %v", err))
		return nil, err
	}

	modelCount := len(models)
	if modelCount == 0 {
		// short-circuit if there's nothing to do
		return make([]*vo.SystemVO, 0), nil
	}

	var vos []*vo.SystemVO
	for _, model := range models {
		vo, err := GetSystem(c, model.ID)
		if err != nil {
			logging.Logger.Error(fmt.Sprintf("No System found from item in array for ID: %s", model.ID))
			continue
		}
		vos = append(vos, vo)
	}

	return vos, err
}
