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

func GetPublisher(c context.Context, id string) (*vo.PublisherVO, error) {
	_, span := otel.Tracer("publisher").Start(c, "db-get-publisher", oteltrace.WithAttributes(attribute.String("id", id)))
	results, err := database.Query[models.Publisher]("publishers", bson.D{{Key: "_id", Value: id}}, nil, nil, 0, 1)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Publisher: %v", err))
		return nil, err
	}

	if len(results) == 0 {
		logging.Logger.Info(fmt.Sprintf("Publisher not found for ID: %s", id))
		return nil, nil
	}

	return publisherModelToVO(results[0]), nil
}

func publisherModelToVO(model *models.Publisher) *vo.PublisherVO {
	return &vo.PublisherVO{
		ID:         model.ID,
		Name:       model.Name,
		Address:    model.Address,
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

// UpdatePublisher merges the provided fields into the existing publisher and writes it, mirroring
// UpdateVolume's raw-marshal $set approach (there's no generic update path since the "_id" field
// is a plain string, not a primitive.ObjectID, so database.Update[T] doesn't apply here).
func UpdatePublisher(c context.Context, id string, publisher *vo.PublisherVO) (*vo.PublisherVO, error) {
	_, span := otel.Tracer("publisher").Start(c, "db-update-publisher", oteltrace.WithAttributes(attribute.String("id", id)))
	defer span.End()

	existing, err := GetPublisher(c, id)
	if err != nil {
		logging.Logger.Error("Error while looking up existing Publisher for update", "id", id, "error", err)
		return nil, err
	}
	if existing == nil {
		logging.Logger.Info(fmt.Sprintf("Publisher not found for update, ID: %s", id))
		return nil, nil
	}

	model := models.Publisher{
		ID:         id,
		Name:       publisher.Name,
		Address:    publisher.Address,
		Website:    publisher.Website,
		Notes:      publisher.Notes,
		Properties: modelcoreutil.ToPropertyModels(publisher.Properties),
		Tags:       modelcoreutil.ToTagModels(publisher.Tags),
		Auditable: modelcore.Auditable{
			CreatedAt: existing.CreatedAt,
			CreatedBy: existing.CreatedBy,
			UpdatedAt: time.Now(),
			UpdatedBy: publisher.UpdatedBy,
			DeletedAt: nil,
			DeletedBy: nil,
		},
	}

	data, err := bson.Marshal(model)
	if err != nil {
		logging.Logger.Error("Error while preparing Publisher document for update", "error", err)
		return nil, err
	}
	var update bson.D
	if err := bson.Unmarshal(data, &update); err != nil {
		logging.Logger.Error("Error while unmarshaling Publisher document for update", "error", err)
		return nil, err
	}

	result, err := database.Db.Collection("publishers").UpdateOne(
		c,
		bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$set", Value: update}},
	)
	if err != nil {
		logging.Logger.Error("Error while updating Publisher in database", "id", id, "error", err)
		return nil, err
	}
	if result.MatchedCount == 0 {
		logging.Logger.Info(fmt.Sprintf("Publisher not found for update, ID: %s", id))
		return nil, nil
	}

	return GetPublisher(c, id)
}

func QueryPublishers(c context.Context, params apiutil.QueryParams) ([]*vo.PublisherVO, error) {
	span := tracing.BuildSpanWithParams(c, "contributions", "db-get-contributions", params)
	filter, sort, projection := apiutil.ConvertQueryParams(params)
	models, err := database.Query[models.Publisher]("publishers", filter, sort, projection, params.Start, params.Limit)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Publishers: %v", err))
		return nil, err
	}

	vos := make([]*vo.PublisherVO, 0, len(models))
	for _, model := range models {
		vos = append(vos, publisherModelToVO(model))
	}

	return vos, nil
}
