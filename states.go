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
	UserID int
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

func (a *App) LoadState(userID int) *State {
	state := new(State)
	state.UserID = userID
	a.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(StatesBucket).Get([]byte(strconv.Itoa(userID)))
		return json.Unmarshal(data, state)
	})
	return state
}

func (a *App) SaveState(state *State) {
	a.db.Update(func(tx *bolt.Tx) error {
		data, _ := json.Marshal(state)
		tx.Bucket(StatesBucket).Put([]byte(strconv.Itoa(state.UserID)), data)
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
