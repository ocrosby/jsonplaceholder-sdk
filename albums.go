package jsonplaceholder

import (
	"context"
	"iter"
	"net/url"
)

// AlbumsService accesses the albums resource. Obtain it from Client.Albums.
type AlbumsService struct{ c *Client }

// AlbumListParams filters and pages a Albums listing. Nil filters are not sent.
type AlbumListParams struct {
	// UserID returns only albums owned by this user.
	UserID *int
	ListOptions
}

// Get returns the album with the given id. It returns an error wrapping
// ErrInvalidArgument when id is less than 1, and an *APIError with status 404
// (see IsNotFound) when no such album exists.
func (s *AlbumsService) Get(ctx context.Context, id int) (*Album, error) {
	return getOne[Album](ctx, s.c, "albums", id)
}

// List returns one page of albums. It returns an error wrapping ErrInvalidArgument
// for a non-positive filter id or a negative page or limit.
func (s *AlbumsService) List(ctx context.Context, p AlbumListParams) (*Page[Album], error) {
	q := url.Values{}
	if err := setID(q, "userId", p.UserID); err != nil {
		return nil, err
	}
	return list[Album](ctx, s.c, "albums", q, p.ListOptions)
}

// All iterates over every album matching p, fetching pages on demand. Stop early
// with break and no further pages are fetched. The first error is yielded and
// ends the iteration. p.Limit sets the page size (default DefaultPageSize).
func (s *AlbumsService) All(ctx context.Context, p AlbumListParams) iter.Seq2[Album, error] {
	return all(p.ListOptions, func(o ListOptions) (*Page[Album], error) {
		p.ListOptions = o
		return s.List(ctx, p)
	})
}
