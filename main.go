package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/bluele/gcache"
	"github.com/boltdb/bolt"
	"github.com/davidmz/FreefeedDirectBot/frf"
	"github.com/davidmz/mustbe"
	"github.com/gorilla/websocket"
	"gopkg.in/telegram-bot-api.v1"
)

var (
	StatesBucket = []byte("States")

	ErrNotFound = errors.New("Not Found")
)

func main() {
	defer mustbe.Catched(func(err error) { log.Fatalln("Fatal error:", err) })

	var (
		botToken   string
		apiHost    string
		dbFileName string
	)

	flag.StringVar(&botToken, "token", "", "telegram bot token")
	flag.StringVar(&apiHost, "apihost", "freefeed.net", "backend API host")
	flag.StringVar(&dbFileName, "dbfile", "", "database file name")
	flag.Parse()

	if botToken == "" || dbFileName == "" {
		flag.Usage()
		return
	}

	db := mustbe.OKVal(bolt.Open(dbFileName, 0600, &bolt.Options{Timeout: 1 * time.Second})).(*bolt.DB)
	defer db.Close()

	mustbe.OK(db.Update(func(tx *bolt.Tx) error {
		mustbe.OKVal(tx.CreateBucketIfNotExists(StatesBucket))
		return nil
	}))

	bot := mustbe.OKVal(tgbotapi.NewBotAPI(botToken)).(*tgbotapi.BotAPI)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		log.Fatalln("Can not get update chan:", err)
		return
	}

	log.Println("Starting bot", bot.Self.UserName)

	app := &App{
		db:      db,
		apiHost: apiHost,
		outbox:  make(chan tgbotapi.Chattable, 0),
		rts:     make(map[int]*Realtime),
		cache:   gcache.New(1000).ARC().Build(),
	}

	app.LoadRT()

	for {
		select {
		case update := <-updates:
			go app.HandleMessage(&update.Message)
		case msg := <-app.outbox:
			bot.Send(msg)
		}
	}

	// for update := range updates {
	//msg := tgbotapi.NewMessage(update.Message.Chat.ID, update.Message.Text)
	//msg.ReplyToMessageID = update.Message.MessageID
	//bot.Send(msg)
	// }
}

func main2() {
	var (
		token   string
		apiHost string
	)

	flag.StringVar(&token, "token", "", "Freefeed access token")
	flag.StringVar(&apiHost, "apihost", "freefeed.net", "backend API host")
	flag.Parse()

	if token == "" {
		flag.Usage()
		return
	}

	// Полуаем ID директ-канала
	var (
		directChannelID string
		userID          string
		userName        string
	)
	{
		req, _ := http.NewRequest("GET", "https://"+apiHost+"/v1/timelines/filter/directs?offset=0", nil)
		req.Header.Add("X-Authentication-Token", token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Fatal("Can not obtain directs channel ID: ", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			if resp.StatusCode == http.StatusUnauthorized {
				log.Fatal("Invalid token: ", resp.Status)
			} else {
				log.Fatal("Invalid HTTP response: ", resp.Status)
			}
		}

		v := &frf.DirectChannelResponse{}
		err = json.NewDecoder(resp.Body).Decode(v)
		resp.Body.Close()
		if err != nil {
			log.Fatal("Can not parse JSON: ", err)
		}
		if v.Timelines == nil {
			log.Fatal("Invalid JSON response")
		}

		directChannelID = v.Timelines.ID
		userID = v.Timelines.UserID
		for _, u := range v.Users {
			if u.ID == userID {
				userName = u.Name
				break
			}
		}
	}

	fmt.Println("Channel ID ", directChannelID)
	fmt.Println("User ID    ", userID)
	fmt.Println("User name  ", userName)

	conn, _, err := websocket.DefaultDialer.Dial(
		"wss://"+apiHost+"/socket.io/?token="+url.QueryEscape(token)+"&EIO=3&transport=websocket",
		nil,
	)

	if err != nil {
		log.Fatal("Can not connect to websocket:", err)
	}

	log.Print("Connected!")

	conn.WriteMessage(websocket.TextMessage, []byte(`42["subscribe",{"timeline":["`+directChannelID+`"]}]`))

	for {
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
				log.Fatal(err)
			}
			log.Println("Ping interval:", v.PingInterval)
			go pingCycle(conn, time.Duration(v.PingInterval)*time.Millisecond)
		} else if t == 42 {
			v := []json.RawMessage{}
			if err := json.Unmarshal(p, &v); err != nil {
				log.Fatal(err)
			}
			log.Println("Event:", string(v[0]), string(v[1]))
		}
	}
}
