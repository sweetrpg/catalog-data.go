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

// UpdateLicense merges the provided fields into the existing license and writes it, mirroring
// UpdatePublisher's raw-marshal $set approach.
func UpdateLicense(c context.Context, id string, license *vo.LicenseVO) (*vo.LicenseVO, error) {
	_, span := otel.Tracer("license").Start(c, "db-update-license", oteltrace.WithAttributes(attribute.String("id", id)))
	defer span.End()

	existing, err := GetLicense(c, id)
	if err != nil {
		logging.Logger.Error("Error while looking up existing License for update", "id", id, "error", err)
		return nil, err
	}
	if existing == nil {
		logging.Logger.Info(fmt.Sprintf("License not found for update, ID: %s", id))
		return nil, nil
	}

	model := models.License{
		ID:           id,
		Title:        license.Title,
		ShortTitle:   license.ShortTitle,
		Version:      license.Version,
		Deed:         license.Deed,
		LegalCode:    license.LegalCode,
		Website:      license.Website,
		Status:       license.Status,
		Availability: license.Availability,
		Notes:        license.Notes,
		Properties:   modelcoreutil.ToPropertyModels(license.Properties),
		Tags:         modelcoreutil.ToTagModels(license.Tags),
		Auditable: modelcore.Auditable{
			CreatedAt: existing.CreatedAt,
			CreatedBy: existing.CreatedBy,
			UpdatedAt: time.Now(),
			UpdatedBy: license.UpdatedBy,
			DeletedAt: nil,
			DeletedBy: nil,
		},
	}

	data, err := bson.Marshal(model)
	if err != nil {
		logging.Logger.Error("Error while preparing License document for update", "error", err)
		return nil, err
	}
	var update bson.D
	if err := bson.Unmarshal(data, &update); err != nil {
		logging.Logger.Error("Error while unmarshaling License document for update", "error", err)
		return nil, err
	}

	result, err := database.Db.Collection("licenses").UpdateOne(
		c,
		bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$set", Value: update}},
	)
	if err != nil {
		logging.Logger.Error("Error while updating License in database", "id", id, "error", err)
		return nil, err
	}
	if result.MatchedCount == 0 {
		logging.Logger.Info(fmt.Sprintf("License not found for update, ID: %s", id))
		return nil, nil
	}

	return GetLicense(c, id)
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
