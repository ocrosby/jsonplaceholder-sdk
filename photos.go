package jsonplaceholder

import (
	"context"
	"iter"
	"net/url"
)

// PhotosService accesses the photos resource. Obtain it from Client.Photos.
type PhotosService struct{ c *Client }

// PhotoListParams filters and pages a Photos listing. Nil filters are not sent.
type PhotoListParams struct {
	// AlbumID returns only photos in this album. Always set it unless you
	// really want all 5000 photos.
	AlbumID *int
	ListOptions
}

// Get returns the photo with the given id. It returns an error wrapping
// ErrInvalidArgument when id is less than 1, and an *APIError with status 404
// (see IsNotFound) when no such photo exists.
func (s *PhotosService) Get(ctx context.Context, id int) (*Photo, error) {
	return getOne[Photo](ctx, s.c, "photos", id)
}

// List returns one page of photos. It returns an error wrapping ErrInvalidArgument
// for a non-positive filter id or a negative page or limit.
func (s *PhotosService) List(ctx context.Context, p PhotoListParams) (*Page[Photo], error) {
	q := url.Values{}
	if err := setID(q, "albumId", p.AlbumID); err != nil {
		return nil, err
	}
	return list[Photo](ctx, s.c, "photos", q, p.ListOptions)
}

// All iterates over every photo matching p, fetching pages on demand. Stop early
// with break and no further pages are fetched. The first error is yielded and
// ends the iteration. p.Limit sets the page size (default DefaultPageSize).
func (s *PhotosService) All(ctx context.Context, p PhotoListParams) iter.Seq2[Photo, error] {
	return all(p.ListOptions, func(o ListOptions) (*Page[Photo], error) {
		p.ListOptions = o
		return s.List(ctx, p)
	})
}
