package main

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/davidmz/FreefeedDirectBot/frf"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

var reCmdRE = regexp.MustCompile(`/re_([a-f0-9]{4,})`)

func (a *App) HandleMessage(msg *tgbotapi.Message) {
	state := a.LoadState(TgUserID(msg.From.ID))
	a.ResetState(state) // по умолчанию сбрасываем состояние

	replyToShortCode := ""
	if msg.ReplyToMessage != nil {
		if m := reCmdRE.FindAllStringSubmatch(msg.ReplyToMessage.Text, -1); m != nil {
			replyToShortCode = m[len(m)-1][1]
		}
	}

	switch cmd := msg.Command(); {

	case cmd == "cancel":
		if state.Action != ActNothing {
			a.SendText(state.UserID, "OK, операция «"+state.ActionTitle()+"» отменена.")
		} else {
			a.SendText(state.UserID, "Сейчас нечего отменять. Используйте /help чтобы увидеть список команд.")
		}

	case cmd == "help":
		a.SendText(state.UserID, HelpMessage)

	case cmd == "start":
		if !state.IsAuthorized() {
			for _, m := range HelloMessages {
				a.SendText(state.UserID, m)
			}
			a.SaveState(state.Clone(ActNewToken))
		} else {
			a.SendText(state.UserID,
				"Мы с вами уже знакомы, "+state.User.Name+". "+
					"Если вы хотите чтобы я вас забыл, используйте команду /logout",
			)
		}

		// возврат из команды /start
	case cmd == "" && state.Action == ActNewToken && msg.Text != "":
		a.SendText(state.UserID, "Спасибо, проверяю ваш токен…")
		u, err := a.testToken(msg.Text)
		if er, ok := err.(*frf.ErrorResponse); ok && er.HTTPStatusCode == http.StatusUnauthorized {
			a.SaveState(state)
			a.SendText(state.UserID, "Похоже, вы указали неправильный токен. Попробуйте ещё раз?")
		} else if err != nil {
			a.SaveState(state)
			a.SendText(state.UserID, "Что-то пошло не так: "+err.Error()+"\nПопробуйте ещё раз?")
		} else {
			state.User = u
			a.ResetState(state) // сохраняем с новым пользователем
			a.SendText(state.UserID,
				"Рад знакомству, "+state.User.Name+"!\n"+
					"Теперь, когда появятся новые директы или комментарии к ним, я вам об этом сообщу. "+
					"Если хотите узнать больше о моих возможностях, используйте коменду /help")
			a.StartRT(state)
		}

	case cmd == "logout" && state.IsAuthorized():
		a.StopRT(state)
		st := state.Clone(ActNothing)
		st.User = nil
		a.SaveState(st)
		a.SendText(state.UserID, "Всё, я вас забыл и стёр все данные о вас. "+
			"Если захотите вернуться, используйте команду /start")

	case cmd == "contacts" && state.IsAuthorized():
		contacts, err := a.getContacts(state.User)
		if err != nil {
			a.SendText(state.UserID, "Что-то пошло не так: "+err.Error())
		} else if len(contacts) == 0 {
			a.SendText(state.UserID, "Похоже, у вас нет взаимных друзей. Вы никому не можете написать директ.")
		} else {
			lines := []string{}
			lines = append(lines, "Ваши взаимные друзья:")
			for _, c := range contacts {
				lines = append(lines, "    /to_"+c)
			}
			lines = append(lines, "Вы можете отправить директ нескольким получателям, кликнув последовательно по их именам.")
			a.SendText(state.UserID, strings.Join(lines, "\n"))
		}

	case strings.HasPrefix(cmd, "to_") && state.IsAuthorized():
		name := strings.TrimPrefix(cmd, "to_")
		if state.Action != ActComposePost {
			state = state.Clone(ActComposePost)
		}

		p := sort.SearchStrings(state.Addressees, name)
		if p == len(state.Addressees) || state.Addressees[p] != name {
			// insert into position p
			state.Addressees = append(state.Addressees, "")
			copy(state.Addressees[p+1:], state.Addressees[p:])
			state.Addressees[p] = name
		}
		a.SaveState(state)
		a.SendText(state.UserID, "OK, ваше сообщение для "+humanList(state.Addressees, state.User.Name, "вас")+" (/cancel — отмена)	:")

		// возврат из команды /to*
	case cmd == "" && state.Action == ActComposePost && msg.Text != "":
		if msg.Text == "" {
			a.SaveState(state)
			a.SendText(state.UserID, "Извините, сообщение может быть только текстовым. Попробуйте ещё раз (/cancel — отмена)?")
			break
		}
		postID, err := a.sendDirect(state.User, state.Addressees, msg.Text)
		if err != nil {
			a.SendText(state.UserID, "Не удалось отправить сообщение. "+err.Error())
		} else {
			shortCode := postID[:4]
			m := tgbotapi.NewMessage(state.UserID,
				"Сообщение отправлено!\n"+
					strings.Repeat("\u2500", 10)+"\n"+
					"Ответить: /re_"+shortCode+" или ответить (Reply) на это сообщение\n"+
					"Открыть: https://"+a.apiHost+"/"+state.User.Name+"/"+postID+"\n",
			)
			m.DisableWebPagePreview = true
			a.outbox <- m
		}

	case strings.HasPrefix(cmd, "re_") && state.IsAuthorized():
		shortCode := strings.TrimPrefix(cmd, "re_")
		post, err := a.getPost(state.User, shortCode)
		if err == ErrNotFound {
			a.SendText(state.UserID, "Сообщение не найдено.")
		} else if err != nil {
			a.SendText(state.UserID, "Что-то пошло не так: "+err.Error())
		} else {
			state = state.Clone(ActAddComment)
			state.PostID = post.ID
			state.PostAuthor = post.Author
			a.SaveState(state)
			a.SendText(state.UserID, "OK, ваш комментарий к сообщению "+post.Author+" «"+post.ShortBody()+"» (/cancel — отмена):")
		}

	case cmd == "" && state.Action == ActAddComment:
		if msg.Text == "" {
			a.SaveState(state)
			a.SendText(state.UserID, "Извините, комментарий может быть только текстовым. Попробуйте ещё раз (/cancel — отмена)?")
			break
		}
		req := new(frf.NewCommentRequest)
		req.Comment.Body = msg.Text
		req.Comment.PostID = state.PostID
		err := a.SendRequest(state.User, "POST", "/v1/comments", req, nil)
		if err != nil {
			a.SendText(state.UserID, "Что-то пошло не так: "+err.Error())
		} else {
			a.SendText(state.UserID, "Комментарий отправлен!\n"+
				strings.Repeat("\u2500", 10)+"\n"+
				"Ответить: /re_"+state.PostID[:4]+" или ответить (Reply) на это сообщение\n"+
				"Открыть: https://"+a.apiHost+"/"+state.PostAuthor+"/"+state.PostID+"\n",
			)
		}

	case cmd == "" && replyToShortCode != "" && state.IsAuthorized():
		post, err := a.getPost(state.User, replyToShortCode)
		if err == ErrNotFound {
			a.SendText(state.UserID, "Сообщение не найдено.")
		} else if err != nil {
			a.SendText(state.UserID, "Что-то пошло не так: "+err.Error())
		} else {
			if msg.Text == "" {
				a.SendText(state.UserID, "Извините, комментарий может быть только текстовым. Попробуйте ещё раз?")
				break
			}
			req := new(frf.NewCommentRequest)
			req.Comment.Body = msg.Text
			req.Comment.PostID = post.ID
			err := a.SendRequest(state.User, "POST", "/v1/comments", req, nil)
			if err != nil {
				a.SendText(state.UserID, "Что-то пошло не так: "+err.Error())
			} else {
				a.SendText(state.UserID, "Комментарий отправлен!\n"+
					strings.Repeat("\u2500", 10)+"\n"+
					"Ответить: /re_"+post.ID[:4]+" или ответить (Reply) на это сообщение\n"+
					"Открыть: https://"+a.apiHost+"/"+post.Author+"/"+post.ID+"\n",
				)
			}
		}

	case cmd == "list" && state.IsAuthorized():
		cnt, _ := strconv.Atoi(strings.TrimSpace(msg.CommandArguments()))
		if cnt == 0 {
			cnt = 5
		}
		posts, err := a.getAllPosts(state.User)
		if err != nil {
			a.SendText(state.UserID, "Что-то пошло не так: "+err.Error())
		} else if len(posts) == 0 {
			a.SendText(state.UserID, "Похоже, у вас нет директ-сообщений.")
		} else {
			if len(posts) > cnt {
				posts = posts[:cnt]
			}
			a.SendText(state.UserID, fmt.Sprintf("Ваши директ-сообщения (%d):", len(posts)))
			for i := range posts {
				p := posts[len(posts)-i-1]
				a.SendText(state.UserID,
					fmt.Sprintf("%d/%d", i+1, len(posts))+
						" ✉ "+humanName(p.Author, state.User.Name, "вы")+" \u2192 "+humanList(p.Addressees, state.User.Name, "вам")+":\n"+
						strings.Repeat("\u2500", 10)+"\n"+
						p.Body+"\n"+
						strings.Repeat("\u2500", 10)+"\n"+
						"Ответить: /re_"+p.ID[:4]+" или ответить (Reply) на это сообщение\n"+
						"Открыть: https://"+a.apiHost+"/"+p.Author+"/"+p.ID+"\n",
				)
			}
		}

	default:
		if !state.IsAuthorized() {
			a.SendText(state.UserID, "К сожалению, я мало что могу сделать, не зная ваш токен. "+
				"Чтобы задать токен используйте команду /start")
		} else {
			a.SendText(state.UserID, "Простите, не понимаю. Используйте /help чтобы увидеть список команд.")
		}
	}
}

func humanList(names []string, yourName string, yourTitle string) (out string) {
	for i, n := range names {
		if n == yourName {
			names[i] = yourTitle
		}
	}
	switch len(names) {
	case 0:
	case 1:
		out = names[0]
	default:
		out = strings.Join(names[:len(names)-1], ", ")
		out += " и " + names[len(names)-1]
	}
	return
}

func humanName(name string, yourName string, yourTitle string) string {
	if name == yourName {
		return yourTitle
	}
	return name
}
