package main

import (
	"encoding/json"
	"log"
	"net/url"
	"strconv"
	"time"

	"github.com/boltdb/bolt"
	"github.com/davidmz/FreefeedDirectBot/frf"
	"github.com/gorilla/websocket"
)

func (a *App) LoadRT() {
	var states []*State
	a.db.View(func(tx *bolt.Tx) error {
		tx.Bucket(StatesBucket).ForEach(func(k, v []byte) error {
			s := new(State)
			json.Unmarshal(v, s)
			if s.User != nil {
				states = append(states, s.Clone(ActNothing))
			}
			return nil
		})
		return nil
	})

	for _, s := range states {
		a.StartRT(s)
	}
}

func (a *App) StartRT(s *State) {
	a.rtLk.Lock()
	a.rts[s.UserID] = NewRealtime(a, s)
	a.rtLk.Unlock()
}

func (a *App) StopRT(s *State) {
	a.rtLk.Lock()
	if r, ok := a.rts[s.UserID]; ok {
		r.Close()
		delete(a.rts, s.UserID)
	}
	a.rtLk.Unlock()
}

type Realtime struct {
	App     *App
	UserID  int
	User    *frf.User
	closeCh chan struct{}
}

func NewRealtime(a *App, s *State) *Realtime {
	rt := &Realtime{
		App:     a,
		UserID:  s.UserID,
		User:    s.User,
		closeCh: make(chan struct{}, 0),
	}
	go rt.run()
	return rt
}

func (r *Realtime) run() {
	var (
		conn *websocket.Conn
		err  error
	)
	go func() {
		<-r.closeCh
		conn.Close()
	}()
loop:
	for {

		select {
		case <-r.closeCh:
			break loop
		default:
		}

		conn, _, err = websocket.DefaultDialer.Dial(
			"wss://"+r.App.apiHost+"/socket.io/?token="+url.QueryEscape(r.User.AccessToken)+"&EIO=3&transport=websocket",
			nil,
		)
		if err != nil {
			log.Println("Can not connect to websocket:", err)
			time.Sleep(10 * time.Second)
			continue
		}

		conn.WriteMessage(websocket.TextMessage, []byte(`42["subscribe",{"timeline":["`+r.User.DirectFeed+`"]}]`))

		for {
			select {
			case <-r.closeCh:
				break loop
			default:
			}

			_, p, err := conn.ReadMessage()
			if err != nil {
				log.Print("Error: ", err)
				break
			}
			t, p := msgSplit(p)
			if t == 0 {
				// start
				v := &struct {
					PingInterval int `json:"pingInterval"`
				}{}
				if err := json.Unmarshal(p, v); err != nil {
					log.Print("Error: ", err)
					break
				}
				go pingCycle(conn, time.Duration(v.PingInterval)*time.Millisecond)
			} else if t == 42 {
				v := []json.RawMessage{}
				if err := json.Unmarshal(p, &v); err != nil {
					log.Print("Error: ", err)
					break
				}
				// log.Println("Event:", string(v[0]), string(v[1]))
				r.App.HandleRT(r.UserID, string(v[0]), v[1])
			}
		}
	}
}

func (r *Realtime) Close() { close(r.closeCh) }

func msgSplit(p []byte) (int, []byte) {
	cut := len(p)
	for i, b := range p {
		if b < 0x30 || b > 0x39 { // not a digit
			cut = i
			break
		}
	}
	t, _ := strconv.Atoi(string(p[:cut]))
	return t, p[cut:]
}

func pingCycle(conn *websocket.Conn, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := conn.WriteMessage(websocket.TextMessage, []byte("2")); err != nil {
				return
			}
		}
	}
}
