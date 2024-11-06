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

func GetPublisher(c context.Context, id string) (*vo.PublisherVO, error) {
	_, span := otel.Tracer("publisher").Start(c, "db-get-publisher", oteltrace.WithAttributes(attribute.String("id", id)))
	model, err := database.Get[models.Publisher]("publishers", id)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Publisher: %v", err))
		return nil, err
	}

	if model == nil {
		logging.Logger.Info(fmt.Sprintf("Publisher not found for ID: %s", id))
		return nil, nil
	}

	return &vo.PublisherVO{
		ID:         model.ID,
		Name:       model.Name,
		Address:    model.Address,
		Website:    model.Website,
		Notes:      model.Notes,
		Properties: modelcorevo.FromPropertyModels(model.Properties),
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

func GetPublishers(c context.Context, params apiutil.QueryParams) ([]*vo.PublisherVO, error) {
	span := tracing.BuildSpanWithParams(c, "contributions", "db-get-contributions", params)
	filter, sort, projection := apiutil.ConvertQueryParams(params)
	models, err := database.Query[models.Publisher]("publishers", filter, sort, projection, params.Start, params.Limit)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Publishers: %v", err))
		return nil, err
	}

	modelCount := len(models)
	if modelCount == 0 {
		// short-circuit if there's nothing to do
		return make([]*vo.PublisherVO, 0), nil
	}

	var vos []*vo.PublisherVO
	for _, model := range models {
		vo, err := GetPublisher(c, model.ID)
		if err != nil {
			logging.Logger.Error(fmt.Sprintf("No Publisher found from item in array for ID: %s", model.ID))
			continue
		}
		vos = append(vos, vo)
	}

	return vos, err
}
