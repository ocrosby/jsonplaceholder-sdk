package jsonplaceholder_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	jp "github.com/ocrosby/jsonplaceholder-sdk"
)

// These examples are compiled but not run, because they call the live service.

func ExampleNewClient() {
	client, err := jp.NewClient() // no credentials needed
	if err != nil {
		log.Fatal(err)
	}
	post, err := client.Posts.Get(context.Background(), 1)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(post.Title)
}

func ExamplePostsService_List() {
	client, _ := jp.NewClient()
	page, err := client.Posts.List(context.Background(), jp.PostListParams{
		UserID:      jp.Int(2), // only user 2's posts
		ListOptions: jp.ListOptions{Page: 1, Limit: 5},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("showing %d of %d\n", len(page.Items), page.Total)
}

func ExampleTodosService_All() {
	client, _ := jp.NewClient()
	// One loop over every open todo, however many pages that takes.
	for todo, err := range client.Todos.All(context.Background(), jp.TodoListParams{Completed: jp.Bool(false)}) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(todo.Title)
	}
}

func ExampleIsNotFound() {
	client, _ := jp.NewClient()
	_, err := client.Posts.Get(context.Background(), 99999)
	if jp.IsNotFound(err) {
		fmt.Println("no such post")
	}
}

func ExampleAPIError() {
	client, _ := jp.NewClient()
	_, err := client.Users.Get(context.Background(), 1)
	var apiErr *jp.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusTooManyRequests {
		fmt.Println("slow down; quote this to support:", apiErr.RequestID)
	}
}

func ExampleWithRequestHook() {
	client, _ := jp.NewClient(jp.WithRequestHook(func(_ context.Context, method, url string) {
		log.Printf("%s %s", method, url) // metrics, tracing or an audit log fit here too
	}))
	_, _ = client.Users.List(context.Background(), jp.ListOptions{})
}
