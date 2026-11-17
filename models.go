package jsonplaceholder

// Post is a blog post written by a User.
type Post struct {
	UserID int    `json:"userId"`
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

// Comment is a comment left on a Post.
type Comment struct {
	PostID int    `json:"postId"`
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Body   string `json:"body"`
}

// Album is a photo album owned by a User.
type Album struct {
	UserID int    `json:"userId"`
	ID     int    `json:"id"`
	Title  string `json:"title"`
}

// Photo is a photo in an Album.
type Photo struct {
	AlbumID      int    `json:"albumId"`
	ID           int    `json:"id"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	ThumbnailURL string `json:"thumbnailUrl"`
}

// Todo is a to-do item belonging to a User.
type Todo struct {
	UserID    int    `json:"userId"`
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
}

// Geo is a latitude/longitude pair. The service sends both as strings.
type Geo struct {
	Lat string `json:"lat"`
	Lng string `json:"lng"`
}

// Address is a User's postal address.
type Address struct {
	Street  string `json:"street"`
	Suite   string `json:"suite"`
	City    string `json:"city"`
	Zipcode string `json:"zipcode"`
	Geo     Geo    `json:"geo"`
}

// Company is the company a User works for.
type Company struct {
	Name        string `json:"name"`
	CatchPhrase string `json:"catchPhrase"`
	BS          string `json:"bs"`
}

// User is a JSONPlaceholder user.
type User struct {
	ID       int     `json:"id"`
	Name     string  `json:"name"`
	Username string  `json:"username"`
	Email    string  `json:"email"`
	Address  Address `json:"address"`
	Phone    string  `json:"phone"`
	Website  string  `json:"website"`
	Company  Company `json:"company"`
}

// Int returns a pointer to v, for optional filters such as PostListParams.UserID.
func Int(v int) *int { return &v }

// Bool returns a pointer to v, for optional filters such as TodoListParams.Completed.
func Bool(v bool) *bool { return &v }
