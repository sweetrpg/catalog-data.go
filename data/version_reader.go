package data

import (
	"errors"

	"github.com/sweetrpg/catalog-objects.go/models"
	"github.com/sweetrpg/catalog-objects.go/vo"
	"go.mongodb.org/mongo-driver/bson"
)

// ErrVersionNotFound is returned for a version that doesn't exist, and for one the caller may
// not act on, so the two can't be told apart.
var ErrVersionNotFound = errors.New("version not found")

// VersionReader identifies who is reading version records. The zero value is a signed-out
// reader, so a caller that forgets to fill it in gets the most restricted view.
type VersionReader struct {
	// UserID is the verified caller's canonical user ID, or "" when signed out.
	UserID string
	// Reviewer is true only for a verified caller with a review role.
	Reviewer bool
}

// scope narrows a version query to what the reader may see: published or formerly published
// versions, plus the reader's own.
func (r VersionReader) scope(filter bson.D) bson.D {
	if r.Reviewer {
		return filter
	}
	published := bson.D{{Key: "state", Value: bson.D{{Key: "$in", Value: bson.A{
		string(models.VersionStateLive), string(models.VersionStateArchived),
	}}}}}
	if r.UserID == "" {
		return append(filter, published...)
	}
	return append(filter, bson.E{Key: "$or", Value: bson.A{
		published, bson.D{{Key: "submitted_by", Value: r.UserID}},
	}})
}

// owns reports whether the reader submitted a version. A signed-out reader owns nothing, even a
// record with no stored submitter.
func (r VersionReader) owns(submittedBy string) bool {
	return r.UserID != "" && r.UserID == submittedBy
}

func (r VersionReader) redactLifecycle(l *vo.VersionLifecycleVO) {
	if r.Reviewer {
		return
	}
	l.ReviewedBy = nil
	if !r.owns(l.SubmittedBy) {
		l.SubmittedBy, l.ReviewNote = "", nil
	}
}

func (r VersionReader) redactVolume(v *vo.VolumeVersionVO) {
	if r.Reviewer {
		return
	}
	v.ReviewedBy = nil
	if !r.owns(v.SubmittedBy) {
		v.SubmittedBy, v.ReviewNote = "", nil
		v.StagedCoverAssetId, v.StagedSampleAssetIds = nil, nil
	}
}
