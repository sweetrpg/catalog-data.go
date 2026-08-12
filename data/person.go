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

func GetPerson(c context.Context, id string) (*vo.PersonVO, error) {
	_, span := otel.Tracer("person").Start(c, "db-get-person", oteltrace.WithAttributes(attribute.String("id", id)))
	results, err := database.Query[models.Person]("persons", bson.D{{Key: "_id", Value: id}}, nil, nil, 0, 1)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Person: %v", err))
		return nil, err
	}

	if len(results) == 0 {
		logging.Logger.Info(fmt.Sprintf("Person not found for ID: %s", id))
		return nil, nil
	}

	return personModelToVO(results[0]), nil
}

func personModelToVO(model *models.Person) *vo.PersonVO {
	return &vo.PersonVO{
		ID:         model.ID,
		Name:       model.Name,
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

// UpdatePerson merges the provided fields into the existing person and writes it, mirroring
// UpdatePublisher's raw-marshal $set approach.
func UpdatePerson(c context.Context, id string, person *vo.PersonVO) (*vo.PersonVO, error) {
	_, span := otel.Tracer("person").Start(c, "db-update-person", oteltrace.WithAttributes(attribute.String("id", id)))
	defer span.End()

	existing, err := GetPerson(c, id)
	if err != nil {
		logging.Logger.Error("Error while looking up existing Person for update", "id", id, "error", err)
		return nil, err
	}
	if existing == nil {
		logging.Logger.Info(fmt.Sprintf("Person not found for update, ID: %s", id))
		return nil, nil
	}

	model := models.Person{
		ID:         id,
		Name:       person.Name,
		Notes:      person.Notes,
		Properties: modelcoreutil.ToPropertyModels(person.Properties),
		Tags:       modelcoreutil.ToTagModels(person.Tags),
		Auditable: modelcore.Auditable{
			CreatedAt: existing.CreatedAt,
			CreatedBy: existing.CreatedBy,
			UpdatedAt: time.Now(),
			UpdatedBy: person.UpdatedBy,
			DeletedAt: nil,
			DeletedBy: nil,
		},
	}

	data, err := bson.Marshal(model)
	if err != nil {
		logging.Logger.Error("Error while preparing Person document for update", "error", err)
		return nil, err
	}
	var update bson.D
	if err := bson.Unmarshal(data, &update); err != nil {
		logging.Logger.Error("Error while unmarshaling Person document for update", "error", err)
		return nil, err
	}

	result, err := database.Db.Collection("persons").UpdateOne(
		c,
		bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$set", Value: update}},
	)
	if err != nil {
		logging.Logger.Error("Error while updating Person in database", "id", id, "error", err)
		return nil, err
	}
	if result.MatchedCount == 0 {
		logging.Logger.Info(fmt.Sprintf("Person not found for update, ID: %s", id))
		return nil, nil
	}

	return GetPerson(c, id)
}

func QueryPersons(c context.Context, params apiutil.QueryParams) ([]*vo.PersonVO, error) {
	span := tracing.BuildSpanWithParams(c, "contributions", "db-get-contributions", params)
	filter, sort, projection := apiutil.ConvertQueryParams(params)
	models, err := database.Query[models.Person]("persons", filter, sort, projection, params.Start, params.Limit)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Persons: %v", err))
		return nil, err
	}

	vos := make([]*vo.PersonVO, 0, len(models))
	for _, model := range models {
		vos = append(vos, personModelToVO(model))
	}

	return vos, nil
}
