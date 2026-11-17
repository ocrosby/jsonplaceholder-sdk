package jsonplaceholder

import (
	"context"
	"iter"
	"net/url"
)

// PostsService accesses the posts resource. Obtain it from Client.Posts.
type PostsService struct{ c *Client }

// PostListParams filters and pages a Posts listing. Nil filters are not sent.
type PostListParams struct {
	// UserID returns only posts written by this user.
	UserID *int
	ListOptions
}

// Get returns the post with the given id. It returns an error wrapping
// ErrInvalidArgument when id is less than 1, and an *APIError with status 404
// (see IsNotFound) when no such post exists.
func (s *PostsService) Get(ctx context.Context, id int) (*Post, error) {
	return getOne[Post](ctx, s.c, "posts", id)
}

// List returns one page of posts. It returns an error wrapping ErrInvalidArgument
// for a non-positive filter id or a negative page or limit.
func (s *PostsService) List(ctx context.Context, p PostListParams) (*Page[Post], error) {
	q := url.Values{}
	if err := setID(q, "userId", p.UserID); err != nil {
		return nil, err
	}
	return list[Post](ctx, s.c, "posts", q, p.ListOptions)
}

// All iterates over every post matching p, fetching pages on demand. Stop early
// with break and no further pages are fetched. The first error is yielded and
// ends the iteration. p.Limit sets the page size (default DefaultPageSize).
func (s *PostsService) All(ctx context.Context, p PostListParams) iter.Seq2[Post, error] {
	return all(p.ListOptions, func(o ListOptions) (*Page[Post], error) {
		p.ListOptions = o
		return s.List(ctx, p)
	})
}
