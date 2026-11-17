package jsonplaceholder

import (
	"context"
	"iter"
	"net/url"
)

// CommentsService accesses the comments resource. Obtain it from Client.Comments.
type CommentsService struct{ c *Client }

// CommentListParams filters and pages a Comments listing. Nil filters are not sent.
type CommentListParams struct {
	// PostID returns only comments on this post.
	PostID *int
	ListOptions
}

// Get returns the comment with the given id. It returns an error wrapping
// ErrInvalidArgument when id is less than 1, and an *APIError with status 404
// (see IsNotFound) when no such comment exists.
func (s *CommentsService) Get(ctx context.Context, id int) (*Comment, error) {
	return getOne[Comment](ctx, s.c, "comments", id)
}

// List returns one page of comments. It returns an error wrapping ErrInvalidArgument
// for a non-positive filter id or a negative page or limit.
func (s *CommentsService) List(ctx context.Context, p CommentListParams) (*Page[Comment], error) {
	q := url.Values{}
	if err := setID(q, "postId", p.PostID); err != nil {
		return nil, err
	}
	return list[Comment](ctx, s.c, "comments", q, p.ListOptions)
}

// All iterates over every comment matching p, fetching pages on demand. Stop early
// with break and no further pages are fetched. The first error is yielded and
// ends the iteration. p.Limit sets the page size (default DefaultPageSize).
func (s *CommentsService) All(ctx context.Context, p CommentListParams) iter.Seq2[Comment, error] {
	return all(p.ListOptions, func(o ListOptions) (*Page[Comment], error) {
		p.ListOptions = o
		return s.List(ctx, p)
	})
}
