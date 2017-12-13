package main

import (
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/bluele/gcache"
	"github.com/boltdb/bolt"
	"github.com/davidmz/FreefeedDirectBot/frf"
	"gopkg.in/telegram-bot-api.v1"
)

type App struct {
	db      *bolt.DB
	apiHost string
	outbox  chan tgbotapi.Chattable
	rts     map[int]*Realtime
	rtLk    sync.Mutex
	cache   gcache.Cache
}

func (a *App) SendText(chatID int, text string) { a.outbox <- tgbotapi.NewMessage(chatID, text) }

func (a *App) testToken(token string) (*frf.User, error) {
	user := &frf.User{AccessToken: strings.TrimSpace(token)}

	v := new(frf.DirectChannelResponse)
	err := user.SendRequest("GET", "https://"+a.apiHost+"/v2/timelines/filter/directs?offset=0", nil, v)
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

	if err := user.SendRequest("POST", "https://"+a.apiHost+"/v1/posts", postBody, v); err != nil {
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
	tasks := [](*contactTask){
		{Url: "https://" + a.apiHost + "/v1/users/" + user.Name + "/subscribers"},
		{Url: "https://" + a.apiHost + "/v1/users/" + user.Name + "/subscriptions"},
	}
	wg := new(sync.WaitGroup)
	wg.Add(len(tasks))
	for _, task := range tasks {
		go func(task *contactTask) {
			defer wg.Done()

			v := &frf.SubscrResponse{}
			err := user.SendRequest("GET", task.Url, nil, v)
			if err != nil {
				task.Err = err
				return
			}

			task.List = make([]string, len(v.Subscr))
			for i, u := range v.Subscr {
				task.List[i] = u.UserName
			}
		}(task)
	}
	wg.Wait()

	umap := make(map[string]int)
	names := []string{}
	for _, t := range tasks {
		if t.Err != nil {
			return nil, t.Err
		}
		for _, n := range t.List {
			umap[n] = umap[n] + 1
			if umap[n] == len(tasks) {
				names = append(names, n)
			}
		}
	}

	sort.Strings(names)

	return names, nil
}

func (a *App) getAllPosts(user *frf.User) ([]*frf.Post, error) {
	v := &frf.DirectChannelResponse{}
	err := user.SendRequest("GET", "https://"+a.apiHost+"/v2/timelines/filter/directs?offset=0", nil, v)
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
	err := user.SendRequest("GET", "https://"+a.apiHost+"/v2/posts/"+postID, nil, v)
	if err != nil {
		return nil, err
	}
	return v.GetPost(), nil
}
