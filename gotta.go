package main

import (
	"errors"
	"strconv"
	"log"
	"regexp"
	"strings"
	"math/rand"
	"os"
	"io/ioutil"
	"net/http"
	"net/url"
	"encoding/json"
	"database/sql"
	"gopkg.in/telegram-bot-api.v4"
	_ "github.com/mattn/go-sqlite3"
	. "github.com/mattn/go-getopt"
)

// case insensitive substring match
func in(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

func main() {
	// telegram and google keys
	var tgkey, gkey, gcx string

	var c int
	OptErr = 0
	for {
		if c = Getopt("t:g:c:h"); c == EOF {
			break
		}
		switch c {
		case 't':
			tgkey = OptArg
		case 'g':
			gkey = OptArg
		case 'c':
			gcx = OptArg
		case 'h':
			log.Printf("usage: gotta -t tgkey -g googlekey -c cx")
			os.Exit(0)
		}
	}

	if len(tgkey) | len(tgkey) | len(tgkey) == 0 {
		log.Panic("usage: gotta -t tgkey -g googlekey -c cx")
		os.Exit(1)
	}

	// the JSON struct
	var cfg struct {
		Pongs []string `json:"pongs"`
		repliesre []*regexp.Regexp
		Replies [][]string `json:"replies"`
		Offenses []string `json:"offenses"`
		Kicks [][]string `json:"kicks"`
		Sounds struct {
			Dir string `json:"dir"`
			Sounds [][]string `json:"sounds"`
			soundsre []*regexp.Regexp
			soundsid []string
		} `json:"sounds"`
	}
	jsdata, err := ioutil.ReadFile("gotta.json")
	err = json.Unmarshal(jsdata, &cfg)

	// compile pongs regexp on start for faster matching
	cfg.repliesre = make([]*regexp.Regexp, len(cfg.Replies))
	for i, word := range cfg.Replies {
		cfg.repliesre[i] = regexp.MustCompile("(?i)\\b" + word[0] + "\\b")
	}

	// same for sounds
	cfg.Sounds.soundsre = make([]*regexp.Regexp, len(cfg.Sounds.Sounds))
	cfg.Sounds.soundsid = make([]string, len(cfg.Sounds.Sounds))
	for i, word := range cfg.Sounds.Sounds {
		word[1] = cfg.Sounds.Dir + "/" + word[1] + ".opus"
		cfg.Sounds.soundsre[i] = regexp.MustCompile("(?i)\\b" + word[0] + "\\b")
	}

	// create the bot
	bot, err := tgbotapi.NewBotAPI(tgkey)
	if err != nil {
		log.Panic(err)
	}
	log.Printf("Authorized on account @%s as %s", bot.Self.UserName, bot.Self.FirstName)

	// create the map for eaters and positions
	eaters := make(map[string]bool)
	positions := make(map[int]string)

	// compile the regexp for karma query
	karmare := regexp.MustCompile("^karma\\s+(.*)$");

	// compile the regexp for google queries
	ask := regexp.MustCompile("^@" + bot.Self.UserName + " (.*)\\?$")

	// fill the structs to query google for searches and images
	gapi := url.URL {
		Scheme : "https",
		Host : "www.googleapis.com",
		Path : "/customsearch/v1",
	}
	// ask Google for only one search result, in Italian
	query := url.Values {
		"key" : []string { gkey },
		"cx" : []string { gcx },
		"hl" : []string { "it" },
		"num" : []string { "1" },
	}

	// fill the structs for google maps venue search
	gmaps := url.URL {
		Scheme : "https",
		Host : "maps.googleapis.com",
		Path : "/maps/api/place/nearbysearch/json",
	}
	// ask Google Maps for a food venue in 800 meters range as the crow flies
	mquery := url.Values {
		"key" : []string { gkey },
		"type" : []string { "food" },
		"radius" : []string { "800" },
	}

	// open the sqlite db
	db, err := sql.Open("sqlite3", "gotta.db")
	if err != nil {
		log.Panic(err)
	}
	// create the DB if doesn't exist
	if _, err := os.Stat("gotta.db"); os.IsNotExist(err) {
		_, err = db.Exec(`
			CREATE TABLE karma (id INTEGER PRIMARY KEY, karma INTEGER DEFAULT 0);
			CREATE TABLE tgusername (username TEXT PRIMARY KEY, id INTEGER NOT NULL, FOREIGN KEY(id) REFERENCES karma(id) ON DELETE CASCADE);
			CREATE TABLE ircnicks (nick TEXT PRIMARY KEY, id INTEGER NOT NULL, FOREIGN KEY(id) REFERENCES karma(id) ON DELETE CASCADE);`)
		if err != nil {
			log.Panic(err)
		}
	}
	db.Exec("PRAGMA foreign_keys = ON");

	// start getting updates
	upd := tgbotapi.NewUpdate(0)
	upd.Timeout = 60
	updates, err := bot.GetUpdatesChan(upd)
	if err != nil {
		log.Panic("error getting updates")
	}
	msgloop: for update := range updates {
		msg := update.Message
		var cmd string
		var tag string

		// skip empty messages
		if msg == nil {
			continue
		}

		// if we received a location or venue, save the user position in a map
		if msg.Location != nil || msg.Venue != nil {
			loc := msg.Location
			if loc == nil {
				loc = &msg.Venue.Location
			}
			positions[msg.From.ID] = strconv.FormatFloat(loc.Latitude, 'f', -1, 64) + "," + strconv.FormatFloat(loc.Longitude, 'f', -1, 64)
			continue
		}

		// save mention and commands in a variable, but only if there is one of them
		// leading / and @ is stripped from mentions and commands 
		if msg.Entities != nil && len(*msg.Entities) == 1 {
			switch e := (*msg.Entities)[0]; e.Type {
			case "bot_command":
				cmdx := msg.Text[e.Offset + 1 : e.Offset + e.Length]
				// handle the /command@botname syntax too by stripping @botname if the command is for us
				if strings.ContainsRune(cmdx, '@') {
					if strings.HasSuffix(cmdx, "@" + bot.Self.UserName) {
						cmd = cmdx[: strings.IndexRune(cmdx, '@')]
					}
				} else {
					cmd = cmdx
				}
			case "mention":
				tag = msg.Text[e.Offset + 1 : e.Offset + e.Length]
			case "text_mention":
				// unsupported yet
			}
		}

		switch {
		// reply if someone says our name
		case in(msg.Text, bot.Self.FirstName):
			bot.Send(tgbotapi.NewMessage(msg.Chat.ID, cfg.Pongs[rand.Intn(len(cfg.Pongs))]))

		// handle commands
		case len(cmd) > 0:
			switch cmd {
			case "mangio":
				// new eater
				eaters[msg.From.FirstName] = true
				bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "ok"))
			case "salto":
				// an eater less
				delete(eaters, msg.From.FirstName)
				bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "finocchio"))
			case "chimagna":
				// eaters list
				var chimagna string
				keys := make([]string, 0, len(eaters))
				for k := range eaters {
					keys = append(keys, k)
				}
				switch len(keys) {
				case 0:
					chimagna = "niente mangiare, niente bere, per i prossimi 20 giorni"
				case 1:
					chimagna = keys[0] + ", solo come un cane"
				default:
					chimagna = strings.Join(keys, ", ") + "\nper un totale di " + strconv.Itoa(len(keys)) + " pranzonauti"
				}
				bot.Send(tgbotapi.NewMessage(msg.Chat.ID, chimagna))
			case "ndosemagna":
				// check if we know the user position
				if pos, ok := positions[msg.From.ID]; ok {
					mquery.Set("location", pos)
					gmaps.RawQuery = mquery.Encode()
					get := gmaps.String()
					resp, err := http.Get(get)
					if err == nil {
						body, err := ioutil.ReadAll(resp.Body)
						// struct for gmaps reply
						var gresp struct {
							Results []struct {
								Geometry struct {
									Location struct {
										Lat float64 `json:"lat"`
										Lng float64 `json:"lng"`
									} `json:"location"`
								} `json:"geometry"`
								Name string `json:"name"`
								Vicinity string `json:"vicinity"`
								Opening_hours struct {
									Open_now bool `json:"open_now"`
								} `json:"opening_hours"`
							} `json:"results"`
						}
						err = json.Unmarshal(body, &gresp)
						if err == nil && len(gresp.Results) == 0 {
							err = errors.New("zero results")
						}
						if err == nil {
							// get a random venue
							i := rand.Intn(len(gresp.Results))
							_, err = bot.Send(tgbotapi.NewVenue(
								msg.Chat.ID,
								gresp.Results[i].Name,
								gresp.Results[i].Vicinity,
								gresp.Results[i].Geometry.Location.Lat,
								gresp.Results[i].Geometry.Location.Lng,
							))
							// advise if the venue could be closed
							if ! gresp.Results[i].Opening_hours.Open_now {
								bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "attenzione perché potrebbe essere chiuso"))
							}
						}
					}
					if err != nil {
						// no venues, learn cooking
						bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "a casa tua"))
					}
				} else {
					// no position cached
					bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "ndo cazzo stai?"))
				}
			case "karmas":
				// double left join to interpolate ids, ircnames and tg usernames
				rows, err := db.Query("SELECT karma, nick, username FROM karma LEFT JOIN ircnicks ON karma.id = ircnicks.id LEFT JOIN tgusername ON karma.id = tgusername.id ORDER BY karma DESC")
				if err != nil {
					log.Fatal(err)
				}
				var result string
				// build the reply string, prefer tg usernames over irc ones, if both
				for rows.Next() {
					var karma int
					var nick []byte
					var username []byte
					err = rows.Scan(&karma, &nick, &username)
					if err != nil {
						log.Fatal(err)
					}
					if len(username) > 0 {
						result += "@" + string(username)
					} else if len(nick) > 0 {
						result += string(nick)
					}
					result += " " + strconv.Itoa(karma) + "\n"
				}
				bot.Send(tgbotapi.NewMessage(msg.Chat.ID, result))
			case "tette", "culo", "maschione":
				// the flesh is weak
				queries := map[string]string {
					"tette" : "tits",
					"culo" : "ass",
					"maschione" : "men",
				}
				// fill in the search type
				query.Set("searchType", "image")
				query.Set("start", strconv.Itoa(rand.Intn(100) + 1))
				query.Set("q", "hot+" + queries[cmd])
				gapi.RawQuery = query.Encode()
				get := gapi.String()
				resp, err := http.Get(get)
				if err == nil {
					body, err := ioutil.ReadAll(resp.Body)
					// google reply struct
					var gresp struct {
						Items []struct {
							Link string `json:"link"`
						} `json:"items"`
					}
					err = json.Unmarshal(body, &gresp)
					if err == nil && len(gresp.Items) == 0 {
						err = errors.New("zero results")
					}
					if err == nil {
						resp, err = http.Get(gresp.Items[0].Link)
						// if we get a reply with a foto, send it
						if err == nil {
							_, err = bot.Send(tgbotapi.NewPhotoUpload(msg.Chat.ID, tgbotapi.FileReader{
								Name : cmd + ".jpg",
								Reader : resp.Body,
								Size : -1,
							}))
						}
					}
				}
				if err != nil {
					bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "usa la fantasia"))
				}
			}
		// offend people when asked to
		case len(tag) > 0 && regexp.MustCompile("(?i)^(offendi|insulta)\\s+@" + tag + "\\b").MatchString(msg.Text):
			bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "@" + tag + " " + cfg.Offenses[rand.Intn(len(cfg.Offenses))]))
		// google search if mentioned with a trailing '?'
		case len(tag) > 0 && tag == bot.Self.UserName:
			if q := ask.FindStringSubmatch(msg.Text); len(q) > 1 {
				link := "boh"
				query.Del("searchType")
				query.Del("start")
				query.Set("q", q[1])
				gapi.RawQuery = query.Encode()
				get := gapi.String()
				resp, err := http.Get(get)
				// do the query to google and publish the first link
				if err == nil {
					body, err := ioutil.ReadAll(resp.Body)
					var gresp struct {
						Items []struct {
							Link string `json:"link"`
						} `json:"items"`
					}
					err = json.Unmarshal(body, &gresp)
					if err == nil && len(gresp.Items) > 0 {
						link = gresp.Items[0].Link
					}
				}
				bot.Send(tgbotapi.NewMessage(msg.Chat.ID, link))
			}
		// get the id from the tg username
		case len(tag) > 0 && regexp.MustCompile("^@" + tag + "\\s*\\+\\+$").MatchString(msg.Text):
			var id int64
			err := db.QueryRow(`SELECT id FROM tgusername WHERE username = "` + tag + `"`).Scan(&id)
			switch err {
			case sql.ErrNoRows:
				// new user
				res, _ := db.Exec("INSERT INTO karma VALUES(NULL, 0)")
				id, err = res.LastInsertId()
				db.Exec(`INSERT INTO tgusername VALUES("` + tag + `", ` + strconv.FormatInt(id, 10) + ")");
			case nil:
				// user already mapped
			default:
				log.Fatal(err)
			}
			// give a karma point to the wise man
			var k int
			db.Exec("UPDATE KARMA SET karma = karma + 1 WHERE id = " + strconv.FormatInt(id, 10))
			db.QueryRow("SELECT karma FROM karma WHERE id = " + strconv.FormatInt(id, 10)).Scan(&k)
			bot.Send(tgbotapi.NewMessage(msg.Chat.ID, tag + " ha #karma " + strconv.Itoa(k)))
		// regular text search
		default:
			// there is no democracy, kick the regime offenders
			for _, re := range cfg.Kicks {
				if in(msg.Text, re[0]) {
					kicked := tgbotapi.ChatMemberConfig {
						ChatID : msg.Chat.ID,
						UserID : msg.From.ID,
					}
					bot.Send(tgbotapi.NewMessage(msg.Chat.ID, re[1]))
					bot.KickChatMember(kicked)
					// but mercifully allow them to rejoin
					bot.UnbanChatMember(kicked)
					continue msgloop
				}
			}
			// send voice notes on matching patterns
			for i, re := range cfg.Sounds.soundsre {
				if re.MatchString(msg.Text) {
					// if we didn't send this note before, prepare a new upload
					if len(cfg.Sounds.soundsid[i]) == 0 {
						voice, err := bot.Send(tgbotapi.NewVoiceUpload(msg.Chat.ID, cfg.Sounds.Sounds[i][1]))
						if err == nil && voice.Voice != nil {
							cfg.Sounds.soundsid[i] = voice.Voice.FileID
						}
					} else {
						// otherwise reuse the cached ID to save people's bandwidth and space
						bot.Send(tgbotapi.NewVoiceShare(msg.Chat.ID, cfg.Sounds.soundsid[i]))
					}
					continue msgloop
				}
			}
			// replies on matching patterns
			for i, re := range cfg.repliesre {
				if re.MatchString(msg.Text) {
					bot.Send(tgbotapi.NewMessage(msg.Chat.ID, cfg.Replies[i][1]))
					continue msgloop
				}
			}
			// get karma for an user
			if q:= karmare.FindStringSubmatch(msg.Text); len(q) > 1 {
				var k int
				var col, table, user string
				// we have different tables and columns for IRC and telegram
				if q[1][0] == '@' {
					table = "tgusername"
					col = "username"
					user = q[1][1:]
				} else {
					table = "ircnicks"
					col = "nick"
					user = q[1]
				}
				db.QueryRow("SELECT karma FROM karma JOIN " + table + " ON karma.id = " + table + ".id WHERE " + col + ` = "` + user + `"`).Scan(&k)
				bot.Send(tgbotapi.NewMessage(msg.Chat.ID, q[1] + " ha karma " + strconv.Itoa(k)))
				continue msgloop
			}
		}
	}
}
