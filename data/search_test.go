package data

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/sweetrpg/catalog-objects.go/vo"
	"github.com/sweetrpg/common.go/logging"
	dbconstants "github.com/sweetrpg/mongodb.go/constants"
	"github.com/sweetrpg/mongodb.go/database"
)

func TestSearchParamsPushQueryAndCapToMongo(t *testing.T) {
	params := searchParams("cele")

	assert.Equal(t, 100, params.Limit)
	if assert.Len(t, params.Filter, 1) {
		assert.Equal(t, searchTermField, params.Filter[0].Field)
		assert.Equal(t, []string{"cele"}, params.Filter[0].Value)
	}
}

type SearchTestSuite struct {
	suite.Suite
}

func (suite *SearchTestSuite) SetupTest() {
	_ = os.Setenv(dbconstants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()
	ctx := suite.T().Context()
	for _, ensure := range []func(context.Context) error{
		EnsurePublisherVersioningIndexes, EnsureStudioVersioningIndexes,
		EnsurePersonVersioningIndexes, EnsureLicenseVersioningIndexes,
	} {
		assert.NoError(suite.T(), ensure(ctx))
	}
}

// searchCase wires one entity's add/search pair so the same assertions run over all four.
type searchCase struct {
	name   string
	add    func(ctx context.Context, label string) (*string, error)
	search func(ctx context.Context, query string) ([]string, error)
}

func searchCases() []searchCase {
	return []searchCase{
		{"persons",
			func(ctx context.Context, l string) (*string, error) { return AddPerson(ctx, &vo.PersonVO{Name: l}) },
			func(ctx context.Context, q string) ([]string, error) {
				r, err := SearchPersons(ctx, q)
				return personIDs(r), err
			}},
		{"publishers",
			func(ctx context.Context, l string) (*string, error) {
				return AddPublisher(ctx, &vo.PublisherVO{Name: l})
			},
			func(ctx context.Context, q string) ([]string, error) {
				r, err := SearchPublishers(ctx, q)
				return publisherIDs(r), err
			}},
		{"studios",
			func(ctx context.Context, l string) (*string, error) { return AddStudio(ctx, &vo.StudioVO{Name: l}) },
			func(ctx context.Context, q string) ([]string, error) {
				r, err := SearchStudios(ctx, q)
				return studioIDs(r), err
			}},
		{"licenses",
			func(ctx context.Context, l string) (*string, error) { return AddLicense(ctx, &vo.LicenseVO{Title: l}) },
			func(ctx context.Context, q string) ([]string, error) {
				r, err := SearchLicenses(ctx, q)
				return licenseIDs(r), err
			}},
	}
}

func personIDs(vos []*vo.PersonVO) (ids []string) {
	for _, v := range vos {
		ids = append(ids, v.ID)
	}
	return
}

func publisherIDs(vos []*vo.PublisherVO) (ids []string) {
	for _, v := range vos {
		ids = append(ids, v.ID)
	}
	return
}

func studioIDs(vos []*vo.StudioVO) (ids []string) {
	for _, v := range vos {
		ids = append(ids, v.ID)
	}
	return
}

func licenseIDs(vos []*vo.LicenseVO) (ids []string) {
	for _, v := range vos {
		ids = append(ids, v.ID)
	}
	return
}

// Substring, case-insensitive match, and regex metacharacters in the term match literally.
func (suite *SearchTestSuite) TestSearchMatchesSubstringCaseInsensitivelyAndLiterally() {
	ctx := suite.T().Context()
	for _, tc := range searchCases() {
		hit, err := tc.add(ctx, "Xsrch(a+)Marker "+tc.name)
		assert.NoError(suite.T(), err, tc.name)
		_, err = tc.add(ctx, "Xsrchaaaamarker "+tc.name)
		assert.NoError(suite.T(), err, tc.name)

		ids, err := tc.search(ctx, "xsrch(a+)marker")
		assert.NoError(suite.T(), err, tc.name)
		assert.Equal(suite.T(), []string{*hit}, ids, "%s: only the literal match, case-insensitively", tc.name)
	}
}

// A term matching more records than the cap returns exactly the cap - the old scan-and-filter
// returned every match it scanned.
func (suite *SearchTestSuite) TestSearchCapsResults() {
	ctx := suite.T().Context()
	for i := range searchResultLimit + 5 {
		_, err := AddPerson(ctx, &vo.PersonVO{Name: fmt.Sprintf("Xcap%03d Person", i)})
		assert.NoError(suite.T(), err)
	}

	ids, err := searchCases()[0].search(ctx, "xcap")
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), ids, searchResultLimit)
}

func TestSearchTestSuite(t *testing.T) {
	suite.Run(t, new(SearchTestSuite))
}
