package jsonplaceholder

import (
	"context"
	"iter"
	"net/url"
)

// UsersService accesses the users resource. Obtain it from Client.Users.
type UsersService struct{ c *Client }

// Get returns the user with the given id. It returns an error wrapping
// ErrInvalidArgument when id is less than 1, and an *APIError with status 404
// (see IsNotFound) when no such user exists.
func (s *UsersService) Get(ctx context.Context, id int) (*User, error) {
	return getOne[User](ctx, s.c, "users", id)
}

// List returns one page of users. There are only 10, so most callers can pass
// the zero ListOptions to get them all.
func (s *UsersService) List(ctx context.Context, o ListOptions) (*Page[User], error) {
	return list[User](ctx, s.c, "users", url.Values{}, o)
}

// All iterates over every user, fetching pages on demand. o.Limit sets the
// page size (default DefaultPageSize).
func (s *UsersService) All(ctx context.Context, o ListOptions) iter.Seq2[User, error] {
	return all(o, func(po ListOptions) (*Page[User], error) { return s.List(ctx, po) })
}
