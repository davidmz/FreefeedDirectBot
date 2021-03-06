package main

import (
	"errors"
	"flag"
	"log"
	"time"

	"github.com/bluele/gcache"
	"github.com/boltdb/bolt"
	"github.com/davidmz/mustbe"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
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
		userAgent  string
	)

	flag.StringVar(&botToken, "token", "", "telegram bot token")
	flag.StringVar(&apiHost, "apihost", "freefeed.net", "backend API host")
	flag.StringVar(&dbFileName, "dbfile", "", "database file name")
	flag.StringVar(&userAgent, "ua", "", "User-Agent for backend requests")
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
		db:        db,
		apiHost:   apiHost,
		userAgent: userAgent,
		outbox:    make(chan tgbotapi.Chattable, 0),
		rts:       make(map[TgUserID]*Realtime),
		cache:     gcache.New(1000).ARC().Build(),
	}

	app.LoadRT()

	for {
		select {
		case update := <-updates:
			go app.HandleMessage(update.Message)
		case msg := <-app.outbox:
			bot.Send(msg)
		}
	}
}
