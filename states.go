package main

import (
	"encoding/json"
	"strconv"

	"github.com/boltdb/bolt"
	"github.com/davidmz/FreefeedDirectBot/frf"
)

type Action string

const (
	ActNothing     Action = ""
	ActNewToken    Action = "new token"
	ActComposePost Action = "new direct"
	ActAddComment  Action = "add comment"
)

var actionTitles = map[Action]string{
	ActNothing:     "ничего",
	ActNewToken:    "установка токена",
	ActComposePost: "создание директ-сообщения",
	ActAddComment:  "добавление комментария",
}

type State struct {
	stateBase
	Addressees []string
	PostAuthor string
	PostID     string
}

type stateBase struct {
	UserID TgUserID
	Action Action
	User   *frf.User
}

func (s *State) IsAuthorized() bool  { return s.User != nil }
func (s *State) ActionTitle() string { return actionTitles[s.Action] }
func (s *State) Clone(act Action) *State {
	newState := &State{stateBase: s.stateBase}
	newState.Action = act
	return newState
}

func (a *App) LoadState(userID TgUserID) *State {
	state := new(State)
	state.UserID = userID
	a.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(StatesBucket).Get([]byte(strconv.FormatInt(userID, 10)))
		return json.Unmarshal(data, state)
	})
	return state
}

func (a *App) SaveState(state *State) {
	a.db.Update(func(tx *bolt.Tx) error {
		data, _ := json.Marshal(state)
		tx.Bucket(StatesBucket).Put([]byte(strconv.FormatInt(state.UserID, 10)), data)
		return nil
	})
}

func (a *App) ResetState(state *State) *State {
	if state.Action == ActNothing {
		return state
	}
	newState := state.Clone(ActNothing)
	a.SaveState(newState)
	return newState
}
