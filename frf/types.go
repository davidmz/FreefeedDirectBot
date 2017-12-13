package frf

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"
)

type Post struct {
	ID         string
	Body       string
	Author     string   // username
	Addressees []string // usernames
}

type PostResponseStaff struct {
	Users []struct {
		ID   string `json:"id"`
		Name string `json:"username"`
	} `json:"subscribers"`
	Users2 []struct {
		ID   string `json:"id"`
		Name string `json:"username"`
	} `json:"users"`
	Feeds []struct {
		ID     string `json:"id"`
		Type   string `json:"name"`
		UserID string `json:"user"`
	} `json:"subscriptions"`
}

type DirectChannelResponse struct {
	PostResponseStaff
	Timelines *struct {
		ID     string `json:"id"`
		UserID string `json:"user"`
	} `json:"timelines"`
	Posts []struct {
		ID      string   `json:"id"`
		UserID  string   `json:"createdBy"`
		Body    string   `json:"body"`
		FeedIDs []string `json:"postedTo"`
	} `json:"posts"`
}

type OnePostResponse struct {
	PostResponseStaff
	Post struct {
		ID      string   `json:"id"`
		UserID  string   `json:"createdBy"`
		Body    string   `json:"body"`
		FeedIDs []string `json:"postedTo"`
	} `json:"posts"`
}

type NewPostRequest struct {
	Meta struct {
		Feeds []string `json:"feeds"`
	} `json:"meta"`
	Post struct {
		Body string `json:"body"`
	} `json:"post"`
}

type NewCommentRequest struct {
	Comment struct {
		Body   string `json:"body"`
		PostID string `json:"postId"`
	} `json:"comment"`
}

type ErrorResponse struct {
	Err            string `json:"err"`
	HTTPStatus     string `json:"-"`
	HTTPStatusCode int    `json:"-"`
}

func (e *ErrorResponse) Error() string {
	if e.Err != "" {
		return e.Err
	}
	return e.HTTPStatus
}
func (e *ErrorResponse) String() string { return e.Error() }

func ReadErrorResponse(resp *http.Response) *ErrorResponse {
	er := &ErrorResponse{}
	json.NewDecoder(resp.Body).Decode(er)
	er.HTTPStatus = resp.Status
	er.HTTPStatusCode = resp.StatusCode
	return er
}

type PostResponse struct {
	Posts *struct {
		ID string `json:"id"`
	} `json:"posts"`
}

type RTNewComment struct {
	Comment struct {
		ID     string `json:"id"`
		Body   string `json:"body"`
		UserID string `json:"createdBy"`
		PostID string `json:"postId"`
	} `json:"comments"`
	Users []struct {
		ID   string `json:"id"`
		Name string `json:"username"`
	} `json:"users"`
}

type WhoAmIResponse struct {
	User struct {
		Subscribers []struct {
			ID       string `json:"id"`
			UserName string `json:"username"`
		} `json:"subscribers"`
	} `json:"users"`
	Subscriptions []struct {
		Name   string `json:"name"`
		UserID string `json:"user"`
	}
}

/////////////////////

func (f *PostResponseStaff) UserNameByID(userID string) string {
	for _, u := range f.Users {
		if u.ID == userID {
			return u.Name
		}
	}
	for _, u := range f.Users2 {
		if u.ID == userID {
			return u.Name
		}
	}
	log.Println("Can not find username by id", userID)
	return "" // не должно быть
}

func (f *PostResponseStaff) UserNameByFeedID(feedID string) (name, typ string) {
	for _, u := range f.Feeds {
		if u.ID == feedID {
			name = f.UserNameByID(u.UserID)
			typ = u.Type
			break
		}
	}
	return
}

func (f *DirectChannelResponse) AllPosts() (posts []*Post) {
	for _, p := range f.Posts {
		post := new(Post)
		post.ID = p.ID
		post.Body = p.Body
		post.Author = f.UserNameByID(p.UserID)
		for _, fid := range p.FeedIDs {
			n, t := f.UserNameByFeedID(fid)
			if t == "Directs" && n != post.Author {
				post.Addressees = append(post.Addressees, n)
			}
		}
		posts = append(posts, post)
	}
	return
}

func (f *OnePostResponse) GetPost() *Post {
	post := new(Post)
	post.ID = f.Post.ID
	post.Body = f.Post.Body
	post.Author = f.UserNameByID(f.Post.UserID)
	for _, fid := range f.Post.FeedIDs {
		n, t := f.UserNameByFeedID(fid)
		if t == "Directs" && n != post.Author {
			post.Addressees = append(post.Addressees, n)
		}
	}
	return post
}

var whiteSpacesRe = regexp.MustCompile(`\s+`)

func (p *Post) ShortBody() string {
	const maxLen = 40
	words := whiteSpacesRe.Split(p.Body, -1)
	cutIdx := len(words)
	sumLen := 0
	for i, w := range words {
		sumLen += utf8.RuneCountInString(w) + 1
		if sumLen > maxLen {
			cutIdx = i + 1
			break
		}
	}
	s := strings.Join(words[:cutIdx], " ")
	if cutIdx < len(words) {
		s = s + "\u2026"
	}
	return s
}
