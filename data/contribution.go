package data

import (
	"context"
	"fmt"

	apicore "github.com/sweetrpg/api-core/constants"
	"github.com/sweetrpg/api-core/tracing"
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

// Get contribution.
//
//	  @Summary Get a contribution
//		 @Description Retrieve a contribution from the data store.
//		 @Param c
//		 @Param id
func GetContribution(c context.Context, id string) (*vo.ContributionVO, error) {
	_, span := otel.Tracer("contribution").Start(c, "db-get-contribution", oteltrace.WithAttributes(attribute.String("id", id)))
	model, err := database.Get[models.Contribution]("contributions", id)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Contribution: %v", err))
		return nil, err
	}

	if model == nil {
		logging.Logger.Info(fmt.Sprintf("Contribution not found for ID: %s", id))
		return nil, nil
	}

	personVO, _ := GetPerson(c, model.PersonId)
	volumeVO, _ := GetVolume(c, model.VolumeId)

	return &vo.ContributionVO{
		ID:     model.ID,
		Person: personVO,
		Volume: volumeVO,
		Roles:  model.Roles,
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

func GetContributions(c context.Context, filter bson.D, options options.Options) ([]*vo.ContributionVO, error) {
	span := tracing.BuildSpanWithOptions(c, "contributions", "db-get-contributions", options)
	models, err := database.Query[models.Contribution]("contributions", filter, "_id", options.Page[apicore.PageStartOption], options.Page[apicore.PageLimitOption])
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Contributions: %v", err))
		return nil, err
	}

	modelCount := len(models)
	if modelCount == 0 {
		// short-circuit if there's nothing to do
		return make([]*vo.ContributionVO, 0), nil
	}

	var vos []*vo.ContributionVO
	for _, model := range models {
		vo, err := GetContribution(c, model.ID)
		if err != nil {
			logging.Logger.Error(fmt.Sprintf("No Contribution found from item in array for ID: %s", model.ID))
			continue
		}
		vos = append(vos, vo)
	}

	return vos, err
}
