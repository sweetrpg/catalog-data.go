package data

import (
	"context"
	"fmt"

	apicore "github.com/sweetrpg/api-core/constants"
	"github.com/sweetrpg/catalog-api/util"
	"github.com/sweetrpg/catalog-objects/models"
	"github.com/sweetrpg/catalog-objects/vo"
	"github.com/sweetrpg/common/logging"
	"github.com/sweetrpg/db/database"
	modelcorevo "github.com/sweetrpg/model-core/vo"
	options "go.jtlabs.io/query"
	"go.mongodb.org/mongo-driver/bson"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func GetPerson(c context.Context, id string) (*vo.PersonVO, error) {
	_, span := otel.Tracer("person").Start(c, "db-get-person", oteltrace.WithAttributes(attribute.String("id", id)))
	model, err := database.Get[models.Person]("persons", id)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Person: %v", err))
		return nil, err
	}

	if model == nil {
		logging.Logger.Info(fmt.Sprintf("Person not found for ID: %s", id))
		return nil, nil
	}

	return &vo.PersonVO{
		ID:         model.ID,
		Name:       model.Name,
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

func GetPersons(c context.Context, filter bson.D, options options.Options) ([]*vo.PersonVO, error) {
	span := util.BuildSpanWithOptions(c, "persons", "db-get-persons", options)
	models, err := database.Query[models.Person]("persons", filter, "_id", options.Page[apicore.PageStartOption], options.Page[apicore.PageLimitOption])
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Persons: %v", err))
		return nil, err
	}

	modelCount := len(models)
	if modelCount == 0 {
		// short-circuit if there's nothing to do
		return make([]*vo.PersonVO, 0), nil
	}

	var vos []*vo.PersonVO
	for _, model := range models {
		vo, err := GetPerson(c, model.ID)
		if err != nil {
			logging.Logger.Error(fmt.Sprintf("No Person found from item in array for ID: %s", model.ID))
			continue
		}
		vos = append(vos, vo)
	}

	return vos, err
}
