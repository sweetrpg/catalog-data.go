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

func GetLicense(c context.Context, id string) (*vo.LicenseVO, error) {
	_, span := otel.Tracer("license").Start(c, "db-get-license", oteltrace.WithAttributes(attribute.String("id", id)))
	results, err := database.Query[models.License]("licenses", bson.D{{Key: "_id", Value: id}}, nil, nil, 0, 1)
	span.End()
	if err != nil {
		logging.Logger.Error(fmt.Sprintf("Error while querying database for License: %v", err))
		return nil, err
	}

	if len(results) == 0 {
		logging.Logger.Info(fmt.Sprintf("License not found for ID: %s", id))
		return nil, nil
	}

	return licenseModelToVO(results[0]), nil
}

func licenseModelToVO(model *models.License) *vo.LicenseVO {
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
	}
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

	vos := make([]*vo.LicenseVO, 0, len(models))
	for _, model := range models {
		vos = append(vos, licenseModelToVO(model))
	}

	return vos, nil
}
