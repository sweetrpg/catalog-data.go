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
	"go.mongodb.org/mongo-driver/bson"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func GetReview(c context.Context, id string) (*vo.ReviewVO, error) {
	_, span := otel.Tracer("review").Start(c, "db-get-review", oteltrace.WithAttributes(attribute.String("id", id)))
	results, err := database.Query[models.Review]("reviews", bson.D{{Key: "_id", Value: id}}, nil, nil, 0, 1)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Review: %v", err))
		return nil, err
	}

	if len(results) == 0 {
		logging.Logger.Info(fmt.Sprintf("Review not found for ID: %s", id))
		return nil, nil
	}

	return reviewModelToVO(c, results[0]), nil
}

func reviewModelToVO(c context.Context, model *models.Review) *vo.ReviewVO {
	volumeVO, err := GetVolume(c, model.VolumeId)
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("No Volume found from Review for ID %s: %s", model.VolumeId, err.Error()))
	}

	return &vo.ReviewVO{
		ID:       model.ID,
		Title:    model.Title,
		Body:     model.Body,
		Language: model.Language,
		Tags:     modelcoreutil.FromTagModels(model.Tags),
		Volume:   volumeVO,
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

func QueryReviews(c context.Context, params apiutil.QueryParams) ([]*vo.ReviewVO, error) {
	span := tracing.BuildSpanWithParams(c, "contributions", "db-get-contributions", params)
	filter, sort, projection := apiutil.ConvertQueryParams(params)
	models, err := database.Query[models.Review]("reviews", filter, sort, projection, params.Start, params.Limit)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Reviews: %v", err))
		return nil, err
	}

	vos := make([]*vo.ReviewVO, 0, len(models))
	for _, model := range models {
		vos = append(vos, reviewModelToVO(c, model))
	}

	return vos, nil
}
