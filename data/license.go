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

func GetLicense(c context.Context, id string) (*vo.LicenseVO, error) {
	_, span := otel.Tracer("license").Start(c, "db-get-license", oteltrace.WithAttributes(attribute.String("id", id)))
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		logging.Logger.Error("Error while converting object ID for Contribution", "error", err)
		return nil, err
	}
	model, err := database.Get[models.License]("licenses", objectId)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for License: %v", err))
		return nil, err
	}

	if model == nil {
		logging.Logger.Info(fmt.Sprintf("License not found for ID: %s", id))
		return nil, nil
	}

	return &vo.LicenseVO{
		ID:           model.ID,
		Title:        model.Title,
		ShortTitle:   model.ShortTitle,
		Version:      model.Version,
		Deed:         model.Deed,
		LegalCode:    model.LegalCode,
		Website:      model.Website,
		Status:       model.Status,
		Availability: model.Availability,
		Notes:        model.Notes,
		Properties:   modelcoreutil.FromPropertyModels(model.Properties),
		Tags:         modelcoreutil.FromTagModels(model.Tags),
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

func QueryLicenses(c context.Context, params apiutil.QueryParams) ([]*vo.LicenseVO, error) {
	span := tracing.BuildSpanWithParams(c, "contributions", "db-get-contributions", params)
	filter, sort, projection := apiutil.ConvertQueryParams(params)
	models, err := database.Query[models.License]("licenses", filter, sort, projection, params.Start, params.Limit)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for Licenses: %v", err))
		return nil, err
	}

	modelCount := len(models)
	if modelCount == 0 {
		// short-circuit if there's nothing to do
		return make([]*vo.LicenseVO, 0), nil
	}

	var vos []*vo.LicenseVO
	for _, model := range models {
		vo, err := GetLicense(c, model.ID)
		if err != nil {
			logging.Logger.Error(fmt.Sprintf("No License found from item in array for ID: %s", model.ID))
			continue
		}
		vos = append(vos, vo)
	}

	return vos, err
}
