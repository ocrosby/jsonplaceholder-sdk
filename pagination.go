package jsonplaceholder

import (
	"context"
	"iter"
	"net/http"
	"net/url"
	"strconv"
)

// Paging constants. MaxPages is the provable upper bound on an All iterator.
const (
	// DefaultPageSize is the page size an All iterator uses when
	// ListOptions.Limit is zero.
	DefaultPageSize = 100
	// MaxPages bounds how many pages one All iterator will fetch.
	MaxPages = 1000
)

// ListOptions selects one page of a collection. The zero value sends no paging
// parameters, and the service then returns every record, which is 5000 for
// photos. Set Limit when you do not want that.
type ListOptions struct {
	// Page is the 1-based page number. Zero means the first page.
	Page int
	// Limit is the number of items per page. Zero means no limit.
	Limit int
}

// validate rejects negative values.
func (o ListOptions) validate() error {
	if o.Page < 0 {
		return invalid("page must not be negative, got %d", o.Page)
	}
	if o.Limit < 0 {
		return invalid("limit must not be negative, got %d", o.Limit)
	}
	return nil
}

// apply adds the paging parameters to q.
func (o ListOptions) apply(q url.Values) {
	if o.Page > 0 {
		q.Set("_page", strconv.Itoa(o.Page))
	}
	if o.Limit > 0 {
		q.Set("_limit", strconv.Itoa(o.Limit))
	}
}

// Page is one page of results.
type Page[T any] struct {
	// Items are the records on this page. It is never nil.
	Items []T
	// Page and Limit echo the options the page was requested with.
	Page  int
	Limit int
	// Total is the size of the whole collection as reported by the service
	// for this query, or -1 when the service did not report it.
	Total int
}

// list fetches one page of T from /<collection>. q already holds any filters.
func list[T any](ctx context.Context, c *Client, collection string, q url.Values, o ListOptions) (*Page[T], error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	o.apply(q)
	op := operation{resource: collection, name: "list", route: "GET /" + collection}
	var items []T
	hdr, err := c.get(ctx, op, "/"+collection, q, &items)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []T{}
	}
	return &Page[T]{Items: items, Page: o.Page, Limit: o.Limit, Total: totalCount(hdr)}, nil
}

// totalCount reads the X-Total-Count header, returning -1 when it is absent or invalid.
func totalCount(h http.Header) int {
	n, err := strconv.Atoi(h.Get("X-Total-Count"))
	if err != nil || n < 0 {
		return -1
	}
	return n
}

// all turns a page fetcher into an iterator over every item. It fetches the
// next page only when the caller asks for more, stops at the first short page
// (or at Total when known), and yields the first error and then stops.
// The loop is bounded by MaxPages.
func all[T any](o ListOptions, fetch func(ListOptions) (*Page[T], error)) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T
		size := o.Limit
		if size == 0 {
			size = DefaultPageSize
		}
		page := max(o.Page, 1)
		for range MaxPages {
			p, err := fetch(ListOptions{Page: page, Limit: size})
			if err != nil {
				yield(zero, err)
				return
			}
			for _, item := range p.Items {
				if !yield(item, nil) {
					return
				}
			}
			if lastPage(p, page, size) {
				return
			}
			page++
		}
		yield(zero, ErrTooManyPages)
	}
}

// lastPage reports whether p, fetched as page number page of the given size,
// is the end of the collection: a short page, or the reported total reached.
func lastPage[T any](p *Page[T], page, size int) bool {
	return len(p.Items) < size || (p.Total >= 0 && page*size >= p.Total)
}
