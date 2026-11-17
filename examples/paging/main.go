// Command paging walks every photo in one album with the All iterator.
package main

import (
	"context"
	"fmt"
	"os"

	jp "github.com/ocrosby/jsonplaceholder-sdk"
)

func main() {
	requests := 0
	client, err := jp.NewClient(jp.WithRequestHook(func(_ context.Context, method, url string) {
		requests++
		fmt.Fprintf(os.Stderr, "-> %s %s\n", method, url)
	}))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	// 50 photos in album 3, fetched 20 at a time: three requests, one loop.
	count := 0
	for photo, err := range client.Photos.All(context.Background(), jp.PhotoListParams{
		AlbumID:     jp.Int(3),
		ListOptions: jp.ListOptions{Limit: 20},
	}) {
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		count++
		if count <= 3 {
			fmt.Println(photo.ID, photo.Title)
		}
	}
	fmt.Printf("%d photos in %d requests\n", count, requests)
}
