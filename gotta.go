/*
 * Bottarga is a rough, rude, shameless Telegram bot, but it has some lacks too
 * Copyright (C) 2016  Matteo Croce <matteo@openwrt.org>
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	_ "github.com/mattn/go-sqlite3"
	"gopkg.in/telegram-bot-api.v4"
	"html"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// case insensitive substring match
func in(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

func parseArgs(db *sql.DB) map[string]string {
	// setup the flags
	tgkey := flag.String("tgkey", "", "Telegram API Key")
	googlekey := flag.String("googlekey", "", "Google API Key")
	googlecx := flag.String("googlecx", "", "Google CX")
	ttskey := flag.String("ttskey", "", "VoiceRSS API Key")

	// parse
	flag.Parse()

	// fill the db
	if len(*tgkey) > 0 {
		db.Exec(`INSERT OR REPLACE INTO config VALUES("tgkey", "` + *tgkey + `")`)
	}
	if len(*googlekey) > 0 {
		db.Exec(`INSERT OR REPLACE INTO config VALUES("googlekey", "` + *googlekey + `")`)
	}
	if len(*googlecx) > 0 {
		db.Exec(`INSERT OR REPLACE INTO config VALUES("googlecx", "` + *googlecx + `")`)
	}
	if len(*ttskey) > 0 {
		db.Exec(`INSERT OR REPLACE INTO config VALUES("ttskey", "` + *ttskey + `")`)
	}

	config := map[string]string{}

	if rows, err := db.Query("SELECT * FROM config"); err == nil {
		for rows.Next() {
			var k, v string
			rows.Scan(&k, &v)
			config[k] = v
		}
		rows.Close()
	} else {
		log.Fatal("Can't read config")
	}

	if _, ok := config["tgkey"]; !ok {
		log.Fatal("Missing Telegram API Key")
	}
	if _, ok := config["googlekey"]; !ok {
		log.Fatal("Missing Google API Key")
	}
	if _, ok := config["googlecx"]; !ok {
		log.Fatal("Missing Google CX")
	}
	if _, ok := config["ttskey"]; !ok {
		log.Fatal("Missing VoiceRSS API Key")
	}

	return config
}

func setupDB(path string) *sql.DB {
	// open the sqlite db
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		log.Panic(err)
	}
	// create the DB if doesn't exist
	if _, err := os.Stat(path); os.IsNotExist(err) {
		_, err = db.Exec(`CREATE TABLE config (key TEXT PRIMARY KEY, value TEXT);
			CREATE TABLE karma (username TEXT NOT NULL, gid INTEGER NOT NULL, karma INTEGER DEFAULT 0);`)
		if err != nil {
			log.Panic(err)
		}
	}

	return db
}

var aggettivi, santi []string

func fillSlice(path string) []string {
	slice := []string{}
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		slice = append(slice, scanner.Text())
	}
	return slice
}

func loadBestemmie(aggettiviFile, santiFile string) {
	aggettivi = fillSlice(aggettiviFile)
	santi = fillSlice(santiFile)
}

func bestemmia() string {
	sub := [4]string{
		"", "Dio", "Cristo", "Madonna",
	}
	suff := [4]string{
		"", "ato", "ato", "ata",
	}
	i := rand.Intn(len(sub))
	if i == 0 {
		return "Mannaggia a " + santi[rand.Intn(len(santi))]
	} else {
		return sub[i] + " " + aggettivi[rand.Intn(len(aggettivi))] + suff[i]
	}
}

func speak(key, text string) (io.ReadCloser, *exec.Cmd) {
	params := url.Values{
		"key":   {key},
		"src":   {text},
		"hl":    {"it-it"},
		"speed": {"10"},
		//"c": {"OGG"},
		"f": {"22khz_16bit_mono"},
	}
	url := "http://api.voicerss.org/?" + params.Encode()
	if response, err := http.Get(url); err == nil {
		if response.StatusCode == 200 {
			mp3 := exec.Command("mpg123", "-w-", "-")
			opus := exec.Command("opusenc", "-", "-")

			mp3.Stdin = response.Body
			opus.Stdin, _ = mp3.StdoutPipe()
			stdout, _ := opus.StdoutPipe()

			opus.Start()
			mp3.Run()
			return stdout, opus
		}
		response.Body.Close()
	}
	return nil, nil
}

// unescape HTML, and expand %xx characters
func unescape(in string) string {
	b := []byte(html.UnescapeString(in))

	l := len(b)
	for i := 0; i < len(b); i++ {
		// look for a %xx token
		e := bytes.IndexByte(b[i:], '%')
		if e < 0 {
			break
		}
		h := make([]byte, 2)
		_, err := hex.Decode(h, b[e+1:e+3])
		if err != nil {
			log.Println(err)
			return in
		}

		// replace and memove
		b[e] = h[0]
		copy(b[e+1:], b[e+3:])
		i += 2
		l -= 2
	}

	for i, c := range b {
		switch c {
		case '4', '@':
			b[i] = 'a'
		case '3':
			b[i] = 'e'
		case '1':
			b[i] = 'i'
		case '0':
			b[i] = 'o'
		}
	}

	return string(b[:l])
}

type jsoncfg struct {
	Pongs     []string `json:"pongs"`
	repliesre []*regexp.Regexp
	Replies   [][]string `json:"replies"`
	Offenses  []string   `json:"offenses"`
	Kicks     [][]string `json:"kicks"`
	Sounds    struct {
		Dir      string     `json:"dir"`
		Sounds   [][]string `json:"sounds"`
		soundsre []*regexp.Regexp
		soundsid []string
	} `json:"sounds"`
	Shocks   []string `json:"shocks"`
	shocksid []string
}

func kick(bot *tgbotapi.BotAPI, chat int64, user int, message string, ban bool) {
	kicked := tgbotapi.ChatMemberConfig{
		ChatID: chat,
		UserID: user,
	}
	if len(message) > 0 {
		bot.Send(tgbotapi.NewMessage(chat, message))
	}
	bot.KickChatMember(kicked)
	if !ban {
		bot.UnbanChatMember(kicked)
	}
}

func main() {
	// seed rng
	rand.Seed(int64(time.Now().Nanosecond()))

	// profanities
	loadBestemmie("aggettivi.txt", "tuttisanti.txt")

	db := setupDB("gotta.db")

	// fill the DB with new args and loadsaved ones
	config := parseArgs(db)

	// the JSON struct
	var cfg jsoncfg

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

	// id caching for shock images
	cfg.shocksid = make([]string, len(cfg.Shocks))

	// create the bot
	bot, err := tgbotapi.NewBotAPI(config["tgkey"])
	if err != nil {
		log.Panic(err)
	}
	log.Printf("Authorized on account @%s as %s", bot.Self.UserName, bot.Self.FirstName)

	// create the map for eaters and positions
	eaters := map[int64]map[string]bool{}
	positions := map[int]string{}

	// compile the regexp for karma query
	karmare := regexp.MustCompile("^karma\\s+(.*)$")

	// compile the regexp for google queries
	ask := regexp.MustCompile("^@" + bot.Self.UserName + " (.*)\\?$")

	// fill the structs to query google for searches and images
	gapi := url.URL{
		Scheme: "https",
		Host:   "www.googleapis.com",
		Path:   "/customsearch/v1",
	}
	// ask Google for only one search result, in Italian
	query := url.Values{
		"key": []string{config["googlekey"]},
		"cx":  []string{config["googlecx"]},
		"hl":  []string{"it"},
		"num": []string{"1"},
	}

	// fill the structs for google maps venue search
	gmaps := url.URL{
		Scheme: "https",
		Host:   "maps.googleapis.com",
		Path:   "/maps/api/place/nearbysearch/json",
	}
	// ask Google Maps for a food venue in 800 meters range as the crow flies
	mquery := url.Values{
		"key":    []string{config["googlekey"]},
		"type":   []string{"food"},
		"radius": []string{"800"},
	}

	// start getting updates
	upd := tgbotapi.NewUpdate(0)
	upd.Timeout = 60
	updates, err := bot.GetUpdatesChan(upd)
	if err != nil {
		log.Panic("error getting updates")
	}

	lastkarmas := map[int64]string{}

	// to reset eaters daily
	day := time.Now().Day()
msgloop:
	for update := range updates {
		msg := update.Message
		var cmd string
		var tag string

		// skip empty messages
		if msg == nil {
			continue
		}

		// clear the eaters list on midnight
		if newday := time.Now().Day(); newday != day {
			day = newday
			eaters = map[int64]map[string]bool{}
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
				cmdx := msg.Text[e.Offset+1 : e.Offset+e.Length]
				// handle the /command@botname syntax too by stripping @botname if the command is for us
				if strings.ContainsRune(cmdx, '@') {
					if strings.HasSuffix(cmdx, "@"+bot.Self.UserName) {
						cmd = cmdx[:strings.IndexRune(cmdx, '@')]
					}
				} else {
					cmd = cmdx
				}
			case "mention":
				tag = msg.Text[e.Offset+1 : e.Offset+e.Length]
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
				if eaters[msg.Chat.ID] == nil {
					eaters[msg.Chat.ID] = map[string]bool{}
				}
				eaters[msg.Chat.ID][msg.From.FirstName] = true
				bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "ok"))
			case "salto":
				// an eater less
				delete(eaters[msg.Chat.ID], msg.From.FirstName)
				bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "finocchio"))
			case "chimagna":
				// eaters list
				var chimagna string
				keys := make([]string, 0, len(eaters[msg.Chat.ID]))
				for k := range eaters[msg.Chat.ID] {
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
						resp.Body.Close()
						// struct for gmaps reply
						var gresp struct {
							Results []struct {
								Geometry struct {
									Location struct {
										Lat float64 `json:"lat"`
										Lng float64 `json:"lng"`
									} `json:"location"`
								} `json:"geometry"`
								Name          string `json:"name"`
								Vicinity      string `json:"vicinity"`
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
							if !gresp.Results[i].Opening_hours.Open_now {
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
				// join to interpolate ids and tg usernames
				rows, err := db.Query("SELECT username, karma FROM karma WHERE gid = " + strconv.FormatInt(msg.Chat.ID, 10) + " ORDER BY karma DESC")
				if err != nil {
					log.Fatal(err)
				}
				var result string
				// build the reply string
				for rows.Next() {
					var username []byte
					var karma int
					if err = rows.Scan(&username, &karma); err != nil {
						log.Fatal(err)
					}
					result += "@" + string(username) + " " + strconv.Itoa(karma) + "\n"
				}
				rows.Close()
				bot.Send(tgbotapi.NewMessage(msg.Chat.ID, result))
			case "tette", "culo", "maschione", "travone":
				// the flesh is weak
				queries := map[string]string{
					"tette":     "tits",
					"culo":      "ass",
					"maschione": "men",
					"travone":   "transgender",
				}
				// fill in the search type
				query.Set("searchType", "image")
				query.Set("start", strconv.Itoa(rand.Intn(100)+1))
				query.Set("q", "hot+"+queries[cmd])
				gapi.RawQuery = query.Encode()
				get := gapi.String()
				resp, err := http.Get(get)
				if err == nil {
					body, err := ioutil.ReadAll(resp.Body)
					resp.Body.Close()
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
								Name:   cmd + ".jpg",
								Reader: resp.Body,
								Size:   -1,
							}))
							resp.Body.Close()
						}
					}
				}
				if err != nil {
					bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "usa la fantasia"))
				}
			case "bestemmia":
				b := bestemmia()
				if len(b) > 0 {
					if s, o := speak(config["ttskey"], b); s != nil {
						bot.Send(tgbotapi.NewVoiceUpload(msg.Chat.ID, tgbotapi.FileReader{
							Name:   b + ".opus",
							Reader: s,
							Size:   -1,
						}))
						s.Close()
						o.Wait()
					} else {
						bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "ti ho dato le ali, adesso Mosconi scorre forte in te"))
					}
				} else {
					bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "ti ho dato le ali, adesso Mosconi scorre forte in te"))
				}
			case "shock":
				// kick 1/3 of the shockers, just to be fair
				if rand.Intn(3) == 0 {
					kick(bot, msg.Chat.ID, msg.From.ID, "E mò basta, hai fatto schifo pure a me!", false)
				} else {
					if i := rand.Intn(len(cfg.Shocks)); len(cfg.shocksid[i]) == 0 {
						photo, err := bot.Send(tgbotapi.NewPhotoUpload(msg.Chat.ID, cfg.Shocks[i]))
						if err == nil && photo.Photo != nil && len(*photo.Photo) != 0 {
							cfg.shocksid[i] = (*photo.Photo)[0].FileID
						}
					} else {
						// otherwise reuse the cached ID to save people's bandwidth and space
						bot.Send(tgbotapi.NewPhotoShare(msg.Chat.ID, cfg.shocksid[i]))
					}
				}
			}
		// offend people when asked to
		case len(tag) > 0 && regexp.MustCompile("(?i)^(offendi|insulta)\\s+@"+tag+"\\b").MatchString(msg.Text):
			bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "@"+tag+" "+cfg.Offenses[rand.Intn(len(cfg.Offenses))]))
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
					resp.Body.Close()
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
		case len(tag) > 0 && msg.Chat.IsGroup() && regexp.MustCompile("^@"+tag+"\\s*\\+\\+$").MatchString(msg.Text):
			if lastkarmas[msg.Chat.ID] != tag {
				karma := 1
				gid := strconv.FormatInt(msg.Chat.ID, 10)
				res, err := db.Exec(`UPDATE KARMA SET karma=karma+1 WHERE username="` + tag + `" AND gid=` + gid)
				if err != nil {
					log.Panic(err)
				}
				if rows, _ := res.RowsAffected(); rows == 0 {
					// new user
					db.Exec(`INSERT INTO karma VALUES("` + tag + `", ` + gid + ", 1)")
				}
				if db.QueryRow(`SELECT karma FROM karma WHERE username="`+tag+`" AND gid=`+gid).Scan(&karma) == nil {
					bot.Send(tgbotapi.NewMessage(msg.Chat.ID, tag+" ha #karma "+strconv.Itoa(karma)))
				}
				lastkarmas[msg.Chat.ID] = tag
			} else {
				bot.Send(tgbotapi.NewMessage(msg.Chat.ID, tag+" ha già dato"))
			}
		// regular text search
		default:
			// there is no democracy, kick the regime offenders
			for _, re := range cfg.Kicks {
				normal := in(msg.Text, re[0])
				if normal || in(unescape(msg.Text), re[0]) {
					var kickmsg string
					if normal {
						kickmsg = re[1]
					} else {
						kickmsg = "Ricordati soltanto una cosa " + msg.From.FirstName + ", e te lo dico una volta sola: non mi fregare… non provare mai a fregarmi…"
					}
					// but mercifully allow them to rejoin, unless he tried to cheat us
					kick(bot, msg.Chat.ID, msg.From.ID, kickmsg, !normal)
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
			if q := karmare.FindStringSubmatch(msg.Text); msg.Chat.IsGroup() && len(q) > 1 {
				var k int
				db.QueryRow(`SELECT karma FROM karma WHERE username="` + q[1][1:] + `" AND gid=` + strconv.FormatInt(msg.Chat.ID, 10)).Scan(&k)
				bot.Send(tgbotapi.NewMessage(msg.Chat.ID, q[1]+" ha karma "+strconv.Itoa(k)))
				continue msgloop
			}
		}
	}
}
