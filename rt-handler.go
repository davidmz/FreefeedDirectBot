package main

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/davidmz/FreefeedDirectBot/frf"
)

func (a *App) HandleRT(userID int, event string, jmsg json.RawMessage) {
	if event == `"comment:new"` {
		v := new(frf.RTNewComment)
		if err := json.Unmarshal(jmsg, v); err != nil {
			log.Println("Can not decode:", string(jmsg[:20]))
			return
		}

		if _, err := a.cache.Get("comm:" + v.Comment.ID); err == nil {
			// дубль
			return
		}
		a.cache.Set("comm:"+v.Comment.ID, struct{}{})

		state := a.LoadState(userID)
		if !state.IsAuthorized() {
			// нет авторизованного юзера
			log.Println("Cannot find state", userID)
			return
		}

		// автор комментария
		authorName := ""
		for _, u := range v.Users {
			if v.Comment.UserID == u.ID {
				authorName = u.Name
				break
			}
		}

		if authorName == state.User.Name {
			// комментарий от нас
			return
		}

		post, err := a.getPostByID(state.User, v.Comment.PostID)
		if err != nil {
			log.Println("Can not find post:", v.Comment.PostID, err)
			return
		}

		a.SendText(userID,
			"💬 " +authorName+" ответил на пост «"+post.ShortBody()+"»:\n"+
			strings.Repeat("\u2500", 10)+"\n"+
			v.Comment.Body+"\n"+
			strings.Repeat("\u2500", 10)+"\n"+
			"Ответить: /re_"+post.ID[:4]+"\n"+
			"Открыть: https://"+a.apiHost+"/"+post.Author+"/"+post.ID+"\n",
		)
	
	} else if event == `"post:new"` {
		v := new(frf.OnePostResponse)
		if err := json.Unmarshal(jmsg, v); err != nil {
			log.Println("Can not decode:", string(jmsg[:20]))
			return
		}

		state := a.LoadState(userID)
		if !state.IsAuthorized() {
			// нет авторизованного юзера
			log.Println("Cannot find state", userID)
			return
		}

		post:=v.GetPost()
		if post.Author == state.User.Name {
			// комментарий от нас
			return
		}

		a.SendText(userID,
			"📨 " +post.Author+" написал "+humanList(post.Addressees, state.User.Name, "вам")+":\n"+
			strings.Repeat("\u2500", 10)+"\n"+
			post.Body+"\n"+
			strings.Repeat("\u2500", 10)+"\n"+
			"Ответить: /re_"+post.ID[:4]+"\n"+
			"Открыть: https://"+a.apiHost+"/"+post.Author+"/"+post.ID+"\n",
		)
	}
}

