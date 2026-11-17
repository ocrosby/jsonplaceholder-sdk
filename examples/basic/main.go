// Command basic fetches one record from each of a few resources.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	jp "github.com/ocrosby/jsonplaceholder-sdk"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	client, err := jp.NewClient()
	if err != nil {
		return err
	}

	user, err := client.Users.Get(ctx, 1)
	if err != nil {
		return err
	}
	fmt.Printf("%s works at %s\n", user.Name, user.Company.Name)

	open, err := client.Todos.List(ctx, jp.TodoListParams{
		UserID:      jp.Int(user.ID),
		Completed:   jp.Bool(false),
		ListOptions: jp.ListOptions{Limit: 3},
	})
	if err != nil {
		return err
	}
	fmt.Printf("%d of %d matching todos:\n", len(open.Items), open.Total)
	for _, t := range open.Items {
		fmt.Println(" -", t.Title)
	}

	// A missing record is a typed error you can branch on.
	if _, err := client.Posts.Get(ctx, 99999); jp.IsNotFound(err) {
		var apiErr *jp.APIError
		if errors.As(err, &apiErr) {
			log.Printf("post 99999: %d %s (request %s)", apiErr.StatusCode, apiErr.Message, apiErr.RequestID)
		}
	}
	return nil
}
