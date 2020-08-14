package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/bluele/gcache"
	"github.com/boltdb/bolt"
	"github.com/davidmz/FreefeedDirectBot/frf"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

type App struct {
	db        *bolt.DB
	apiHost   string
	userAgent string
	outbox    chan tgbotapi.Chattable
	rts       map[TgUserID]*Realtime
	rtLk      sync.Mutex
	cache     gcache.Cache
}

func (a *App) SendText(chatID TgUserID, text string) { a.outbox <- tgbotapi.NewMessage(chatID, text) }

func (a *App) testToken(token string) (*frf.User, error) {
	user := &frf.User{AccessToken: strings.TrimSpace(token)}

	v := new(frf.DirectChannelResponse)
	err := a.SendRequest(user, "GET", "/v2/timelines/filter/directs?offset=0", nil, v)
	if err != nil {
		return nil, err
	}

	user.DirectFeed = v.Timelines.ID
	for _, u := range v.Users {
		if u.ID == v.Timelines.UserID {
			user.Name = u.Name
			break
		}
	}

	if user.Name == "" {
		for _, u := range v.Users2 {
			if u.ID == v.Timelines.UserID {
				user.Name = u.Name
				break
			}
		}
	}

	return user, nil
}

func (a *App) sendDirect(user *frf.User, addressees []string, text string) (string, error) {
	postBody := &frf.NewPostRequest{}
	postBody.Meta.Feeds = addressees
	postBody.Post.Body = text
	v := &frf.PostResponse{}

	if err := a.SendRequest(user, "POST", "/v1/posts", postBody, v); err != nil {
		return "", err
	}
	return v.Posts.ID, nil
}

var toRe = regexp.MustCompile(`^\s*([a-zA-Z0-9]{3,25})\s+(.+?)\s*$`)

type contactTask struct {
	Url  string
	Err  error
	List []string
}

func (a *App) getContacts(user *frf.User) ([]string, error) {
	v := &frf.WhoAmIResponse{}
	err := a.SendRequest(user, "GET", "/v2/users/whoami", nil, v)
	if err != nil {
		return nil, err
	}
	names := []string{}
	userIDs := map[string]string{} // ID -> username
	for _, s := range v.User.Subscribers {
		userIDs[s.ID] = s.UserName
	}
	for _, s := range v.Subscriptions {
		if s.Name == "Posts" && userIDs[s.UserID] != "" {
			names = append(names, userIDs[s.UserID])
			delete(userIDs, s.UserID) // возможные дубликаты
		}
	}
	sort.Strings(names)

	return names, nil
}

func (a *App) getAllPosts(user *frf.User) ([]*frf.Post, error) {
	v := &frf.DirectChannelResponse{}
	err := a.SendRequest(user, "GET", "/v2/timelines/filter/directs?offset=0", nil, v)
	if err != nil {
		return nil, err
	}
	return v.AllPosts(), nil
}

func (a *App) getPost(user *frf.User, shortCode string) (*frf.Post, error) {
	posts, err := a.getAllPosts(user)
	if err != nil {
		return nil, err
	}

	for _, p := range posts {
		if strings.HasPrefix(p.ID, shortCode) {
			return p, nil
		}
	}

	return nil, ErrNotFound
}

func (a *App) getPostByID(user *frf.User, postID string) (*frf.Post, error) {
	v := &frf.OnePostResponse{}
	err := a.SendRequest(user, "GET", "/v2/posts/"+postID, nil, v)
	if err != nil {
		return nil, err
	}
	return v.GetPost(), nil
}

type requestSigner interface {
	Sign(*http.Request) *http.Request
}

func (a *App) SendRequest(u requestSigner, method string, uri string, reqObj interface{}, respObj interface{}) error {
	url := "https://" + a.apiHost + uri
	var req *http.Request
	if reqObj == nil {
		req, _ = http.NewRequest(method, url, nil)
	} else {
		r, w := io.Pipe()
		go func() { json.NewEncoder(w).Encode(reqObj); w.Close() }()
		req, _ = http.NewRequest(method, url, r)
		req.Header.Add("Content-Type", "application/json; charset=utf-8")
	}
	if a.userAgent != "" {
		req.Header.Set("User-Agent", a.userAgent)
	}
	u.Sign(req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := frf.ReadErrorResponse(resp)
		log.Println("Error:", err, "while send", method, "request to", url)
		return err
	}

	if respObj != nil {
		return json.NewDecoder(resp.Body).Decode(respObj)
	}

	return nil
}
