// Package jsonplaceholder is a Go SDK for JSONPlaceholder
// (https://jsonplaceholder.typicode.com), the free fake REST API.
//
// Create a Client once and reuse it. Each of the six resources (users, posts,
// comments, albums, photos, todos) is a field with Get, List and All methods:
//
//	client, err := jsonplaceholder.NewClient()
//	if err != nil {
//		return err
//	}
//
//	post, err := client.Posts.Get(ctx, 1)
//
//	page, err := client.Todos.List(ctx, jsonplaceholder.TodoListParams{
//		UserID:      jsonplaceholder.Int(2),
//		Completed:   jsonplaceholder.Bool(false),
//		ListOptions: jsonplaceholder.ListOptions{Limit: 10},
//	})
//
//	for photo, err := range client.Photos.All(ctx, jsonplaceholder.PhotoListParams{AlbumID: jsonplaceholder.Int(1)}) {
//		...
//	}
//
// The SDK owns the cross-cutting work so callers do not repeat it: transient
// failures are retried with jittered exponential backoff (honoring
// Retry-After), pagination is hidden behind a range-over-func iterator, and
// failures are typed values (APIError, ErrInvalidArgument) rather than
// strings. Every method takes a context.Context first and honors
// cancellation, including during backoff.
//
// Errors carry a go-platform *apperrors.AppError in their chain, so
// apperrors.IsNotFound(err) works on them.
//
// JSONPlaceholder fakes writes and persists nothing, so this SDK is read-only.
package jsonplaceholder
