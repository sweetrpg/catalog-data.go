package data

import (
	"context"
	"fmt"

	"github.com/sweetrpg/api-core.go/tracing"
	apiutil "github.com/sweetrpg/api-core.go/util"
	"github.com/sweetrpg/catalog-objects.go/models"
	"github.com/sweetrpg/catalog-objects.go/vo"
	"github.com/sweetrpg/common.go/logging"
	modelcorevo "github.com/sweetrpg/model-core.go/vo"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// Get a single contribution.
//
//	  @Summary Get a contribution
//		 @Description Retrieve a contribution from the data store.
//		 @Param c
//		 @Param id
func GetContribution(c context.Context, id string) (*vo.ContributionVO, error) {
	_, span := otel.Tracer("contribution").Start(c, "db-get-contribution", oteltrace.WithAttributes(attribute.String("id", id)))
	results, err := database.Query[models.Contribution]("contributions", bson.D{{Key: "_id", Value: id}}, nil, nil, 0, 1)
	span.End()
	if err != nil {
		logging.Logger.Error("Error while querying database for Contribution", "error", err)
		return nil, err
	}

	if len(results) == 0 {
		logging.Logger.Info("Contribution not found for ID", "id", id)
		return nil, nil
	}

	return contributionModelToVO(c, results[0]), nil
}

func contributionModelToVO(c context.Context, model *models.Contribution) *vo.ContributionVO {
	personVO, err := GetPerson(c, model.PersonId)
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("No Person found from Contribution for ID %s: %s", model.PersonId, err.Error()))
	}
	volumeVO, err := GetVolume(c, model.VolumeId)
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("No Volume found from Contribution for ID %s: %s", model.VolumeId, err.Error()))
	}

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
	}
}

// Get many contributions.
//
//	@Summary Query the datastore for contributions.
//	@Description Given a set of parameters, query the datastore for contributions that match.
//	@Param c A Context object
//	@Param params A QueryParams object that contains the parameters for the query
func QueryContributions(c context.Context, params apiutil.QueryParams) ([]*vo.ContributionVO, error) {
	span := tracing.BuildSpanWithParams(c, "contributions", "db-get-contributions", params)
	filter, sort, projection := apiutil.ConvertQueryParams(params)
	models, err := database.Query[models.Contribution]("contributions", filter, sort, projection, params.Start, params.Limit)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Contributions: %v", err))
		return nil, err
	}

	vos := make([]*vo.ContributionVO, 0, len(models))
	for _, model := range models {
		vos = append(vos, contributionModelToVO(c, model))
	}

	return vos, nil
}
