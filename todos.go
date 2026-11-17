package jsonplaceholder

import (
	"context"
	"iter"
	"net/url"
	"strconv"
)

// TodosService accesses the todos resource. Obtain it from Client.Todos.
type TodosService struct{ c *Client }

// TodoListParams filters and pages a Todos listing. Nil filters are not sent.
type TodoListParams struct {
	// UserID returns only todos belonging to this user.
	UserID *int
	// Completed returns only finished (true) or only open (false) todos.
	Completed *bool
	ListOptions
}

// Get returns the todo with the given id. It returns an error wrapping
// ErrInvalidArgument when id is less than 1, and an *APIError with status 404
// (see IsNotFound) when no such todo exists.
func (s *TodosService) Get(ctx context.Context, id int) (*Todo, error) {
	return getOne[Todo](ctx, s.c, "todos", id)
}

// List returns one page of todos. It returns an error wrapping ErrInvalidArgument
// for a non-positive filter id or a negative page or limit.
func (s *TodosService) List(ctx context.Context, p TodoListParams) (*Page[Todo], error) {
	q := url.Values{}
	if err := setID(q, "userId", p.UserID); err != nil {
		return nil, err
	}
	if p.Completed != nil {
		q.Set("completed", strconv.FormatBool(*p.Completed))
	}
	return list[Todo](ctx, s.c, "todos", q, p.ListOptions)
}

// All iterates over every todo matching p, fetching pages on demand. Stop early
// with break and no further pages are fetched. The first error is yielded and
// ends the iteration. p.Limit sets the page size (default DefaultPageSize).
func (s *TodosService) All(ctx context.Context, p TodoListParams) iter.Seq2[Todo, error] {
	return all(p.ListOptions, func(o ListOptions) (*Page[Todo], error) {
		p.ListOptions = o
		return s.List(ctx, p)
	})
}
