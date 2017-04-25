/*
 * @rotiscianibot is a telegram bot for the ADI Central Committee chat,
 * derived from bottarga, a rough, rude, shameless Telegram bot.
 * Also this bot has its share of defects, of course.
 * Copyright (C) 2017  Matteo Croce <matteo@openwrt.org>
 * Copyright (C) 2017  Andrea Claudi <email@andreaclaudi.it>
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
	//"errors"
	"flag"
	"fmt"
	_ "github.com/lib/pq"
	"gopkg.in/telegram-bot-api.v4"
	"html"
	//"io"
	"io/ioutil"
	"log"
	"math/rand"
	//"net/http"
	"net/smtp"
	//"net/url"
	"os"
	//"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var cfg *jsoncfg

const (
	DB_USER = ""
	DB_PASSWORD = ""
	DB_NAME	= ""
	CONF = "alessio.json"
)

type jsoncfg struct {
	Pongs        []string `json:"pongs"`
	repliesre    []*regexp.Regexp
	Replies      [][]string `json:"replies"`
	Appreciation []string   `json:"appreciation"`
	Sounds       struct {
		Dir      string     `json:"dir"`
		Sounds   [][]string `json:"sounds"`
		soundsre []*regexp.Regexp
		soundsid [][]string
	} `json:"sounds"`
}

/* Mail structs and functions */
type mail struct {
	sender  string
	to      []string
	cc      []string
	bcc     []string
	subject string
	body    string
}

type smtpServer struct {
	host string
	port string
}

func (s *smtpServer) serverName() string {
	return s.host + ":" + s.port
}

func (m *mail) BuildMessage() string {
	header := ""
	header += fmt.Sprintf("From: %s\r\n", m.sender)
	if len(m.to) > 0 {
		header += fmt.Sprintf("To: %s\r\n", strings.Join(m.to, ";"))
	}
	if len(m.cc) > 0 {
		header += fmt.Sprintf("Cc: %s\r\n", strings.Join(m.cc, ";"))
	}

	header += fmt.Sprintf("Subject: %s\r\n", m.subject)
	header += "\r\n" + m.body

	return header
}

func sendMail(config map[string]string, subject, body string) {
	m := mail{}
	m.sender = "sender@example.com"
	m.to = []string{"receiver@example.com"}
	// m.cc = []string{""}		UNSUPPORTED
	// m.bcc = []string{""}		UNSUPPORTED
	m.subject = subject
	m.body = body

	server := smtpServer{host: config["mailserver"], port: config["mailport"]}
	auth := smtp.PlainAuth("", config["mailuser"], config["mailpass"], server.host)

	err := smtp.SendMail(server.serverName(), auth, m.sender, m.to, []byte(m.BuildMessage()))
	if err != nil {
		log.Println(err)
	} else {
		log.Println("Mail sent successfully")
	}
}

// case insensitive substring match
func in(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

func parseArgs(db *sql.DB) map[string]string {
	// setup the flags
	tgkey := flag.String("tgkey", "", "Telegram API Key")
	ongroup := flag.String("ongroup", "", "Add specific features on this Telegram group")
	mailserver := flag.String("mailserver", "", "SMTP server address")
	mailport := flag.String("mailport", "", "SMTP server port")
	mailuser := flag.String("mailuser", "", "SMTP server user for authentication")
	mailpass := flag.String("mailpass", "", "SMTP server password for authentication")
	//googlekey := flag.String("googlekey", "", "Google API Key")
	//googlecx := flag.String("googlecx", "", "Google CX")
	//ttskey := flag.String("ttskey", "", "VoiceRSS API Key")

	// parse
	flag.Parse()

	// fill the db
	if len(*tgkey) > 0 {
		result, err := db.Exec("INSERT INTO config VALUES('tgkey', $1) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;", *tgkey)
		_ = result
		if err != nil {
			log.Fatal(err)
		}
	}
	if len(*ongroup) > 0 {
		result, err := db.Exec("INSERT INTO config VALUES('ongroup', $1) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;", *ongroup)
		_ = result
		if err != nil {
			log.Fatal(err)
		}
	}
	if len(*mailserver) > 0 {
		result, err := db.Exec("INSERT INTO config VALUES('mailserver', $1) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;", *mailserver)
		_ = result
		if err != nil {
			log.Fatal(err)
		}
	}
	if len(*mailport) > 0 {
		result, err := db.Exec("INSERT INTO config VALUES('mailpass', $1) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;", *mailport)
		_ = result
		if err != nil {
			log.Fatal(err)
		}
	}
	if len(*mailuser) > 0 {
		result, err := db.Exec("INSERT INTO config VALUES('mailuser', $1) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;", *mailuser)
		_ = result
		if err != nil {
			log.Fatal(err)
		}
	}
	if len(*mailpass) > 0 {
		result, err := db.Exec("INSERT INTO config VALUES('mailpass', $1) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;", *mailpass)
		_ = result
		if err != nil {
			log.Fatal(err)
		}
	}
	//if len(*googlekey) > 0 {
	//	db.Exec(`INSERT OR REPLACE INTO config VALUES("googlekey", "` + *googlekey + `")`)
	//}
	//if len(*googlecx) > 0 {
	//	db.Exec(`INSERT OR REPLACE INTO config VALUES("googlecx", "` + *googlecx + `")`)
	//}
	//if len(*ttskey) > 0 {
	//	db.Exec(`INSERT OR REPLACE INTO config VALUES("ttskey", "` + *ttskey + `")`)
	//}

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
	if _, ok := config["ongroup"]; !ok {
		log.Println("No restriction on commands")
	}
	if _, ok := config["mailserver"]; !ok {
		log.Println("No mail server address")
	}
	if _, ok := config["mailport"]; !ok {
		log.Println("No mail server port")
	}
	if _, ok := config["mailuser"]; !ok {
		log.Println("No mail server user")
	}
	if _, ok := config["mailpass"]; !ok {
		log.Println("No mail server pass")
	}
	//if _, ok := config["googlekey"]; !ok {
	//	log.Fatal("Missing Google API Key")
	//}
	//if _, ok := config["googlecx"]; !ok {
	//	log.Fatal("Missing Google CX")
	//}
	//if _, ok := config["ttskey"]; !ok {
	//	log.Fatal("Missing VoiceRSS API Key")
	//}

	return config
}

func setupDB() *sql.DB {
	// open the sqlite db
	dbinfo := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", DB_USER, DB_PASSWORD, DB_NAME)
	db, err := sql.Open("postgres", dbinfo)
	if err != nil {
		log.Panic(err)
	}
	// create the DB if doesn't exist
	//if _, err := os.Stat(path); os.IsNotExist(err) {
	//	_, err = db.Exec(`CREATE TABLE config (key TEXT PRIMARY KEY, value TEXT);
	//		CREATE TABLE karma (username TEXT NOT NULL, gid INTEGER NOT NULL, karma INTEGER DEFAULT 0);`)
	//	if err != nil {
	//		log.Panic(err)
	//	}
	//}

	return db
}

//var aggettivi, santi []string

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

//func loadBestemmie(aggettiviFile, santiFile string) {
//	aggettivi = fillSlice(aggettiviFile)
//	santi = fillSlice(santiFile)
//}
//
//func bestemmia() string {
//	sub := [4]string{
//		"", "Dio", "Cristo", "Madonna",
//	}
//	suff := [4]string{
//		"", "ato", "ato", "ata",
//	}
//	i := rand.Intn(len(sub))
//	if i == 0 {
//		return "Mannaggia a " + santi[rand.Intn(len(santi))]
//	} else {
//		return sub[i] + " " + aggettivi[rand.Intn(len(aggettivi))] + suff[i]
//	}
//}
//
//func speak(key, text string) (io.ReadCloser, *exec.Cmd) {
//	params := url.Values{
//		"key":   {key},
//		"src":   {text},
//		"hl":    {"it-it"},
//		"speed": {"10"},
//		//"c": {"OGG"},
//		"f": {"22khz_16bit_mono"},
//	}
//	url := "http://api.voicerss.org/?" + params.Encode()
//	if response, err := http.Get(url); err == nil {
//		if response.StatusCode == 200 {
//			mp3 := exec.Command("mpg123", "-w-", "-")
//			opus := exec.Command("opusenc", "-", "-")
//
//			mp3.Stdin = response.Body
//			opus.Stdin, _ = mp3.StdoutPipe()
//			stdout, _ := opus.StdoutPipe()
//
//			opus.Start()
//			mp3.Run()
//			return stdout, opus
//		}
//		response.Body.Close()
//	}
//	return nil, nil
//}

// unescape HTML, and expand %xx characters
func unescape(in string) string {
	b := []byte(html.UnescapeString(in))
	l := len(b)
	for i := 0; i < len(b)-2; i++ {
		// look for a %xx token
		e := bytes.IndexByte(b[i:len(b)-2], '%')
		if e < 0 {
			break
		}
		h := make([]byte, 2)
		_, err := hex.Decode(h, b[e+1:e+3])
		if err != nil {
			continue
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

func loadConfig(path string) *jsoncfg {
	var cfg jsoncfg

	log.Println("Loading conf from file ", path)
	if jsdata, err := ioutil.ReadFile(CONF); err == nil {
		if err = json.Unmarshal(jsdata, &cfg); err != nil {
			log.Panic(err)
		}
	} else {
		log.Panic(err)
	}

	// compile pongs regexp on start for faster matching
	cfg.repliesre = make([]*regexp.Regexp, len(cfg.Replies))
	for i, word := range cfg.Replies {
		cfg.repliesre[i] = regexp.MustCompile("(?i)\\b" + word[0] + "\\b")
	}

	// same for sounds
	cfg.Sounds.soundsre = make([]*regexp.Regexp, len(cfg.Sounds.Sounds))
	cfg.Sounds.soundsid = make([][]string, len(cfg.Sounds.Sounds))
	for i, word := range cfg.Sounds.Sounds {
		cfg.Sounds.soundsre[i] = regexp.MustCompile("(?i)\\b" + word[0] + "\\b")
		cfg.Sounds.soundsid[i] = make([]string, len(word))
		for j, file := range word[1:] {
			word[j+1] = cfg.Sounds.Dir + "/" + file + ".opus"
		}
	}

	return &cfg
}

func usr1() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGUSR1)

	for {
		cfg = loadConfig(CONF)
		<-sig
	}
}

func main() {
	// seed rng
	rand.Seed(int64(time.Now().Nanosecond()))

	db := setupDB()

	// fill the DB with new args and load saved ones
	config := parseArgs(db)

	// load configuration
	cfg = loadConfig(CONF)

	// start SIGUSR1 loop
	go usr1()

	// init mailbody var
	mailbody := ""

	// create the bot
	bot, err := tgbotapi.NewBotAPI(config["tgkey"])
	if err != nil {
		log.Panic(err)
	}
	log.Printf("Authorized on account @%s as %s\n", bot.Self.UserName, bot.Self.FirstName)

	// verbosity initial settings
	conciseness := 0

	// compile regexp for verbosity
	v1 := regexp.MustCompile("basta(.*)$")
	v2 := regexp.MustCompile("hai rotto(.*)$")
	v3 := regexp.MustCompile("hai scassato(.*)$")

	// compile the regexp for google queries
	//ask := regexp.MustCompile("^@" + bot.Self.UserName + " (.*)\\?$")

	// fill the structs to query google for searches and images
	//gapi := url.URL{
	//	Scheme: "https",
	//	Host:   "www.googleapis.com",
	//	Path:   "/customsearch/v1",
	//}
	// ask Google for only one search result, in Italian
	//query := url.Values{
	//	"key": []string{config["googlekey"]},
	//	"cx":  []string{config["googlecx"]},
	//	"hl":  []string{"it"},
	//	"num": []string{"1"},
	//}

	// fill the structs for google maps venue search
	//gmaps := url.URL{
	//	Scheme: "https",
	//	Host:   "maps.googleapis.com",
	//	Path:   "/maps/api/place/nearbysearch/json",
	//}
	//// ask Google Maps for a food venue in 800 meters range as the crow flies
	//mquery := url.Values{
	//	"key":    []string{config["googlekey"]},
	//	"type":   []string{"food"},
	//	"radius": []string{"800"},
	//}

	// start getting updates
	upd := tgbotapi.NewUpdate(0)
	upd.Timeout = 60
	updates, err := bot.GetUpdatesChan(upd)
	if err != nil {
		log.Panic("error getting updates")
	}

	// to reset eaters daily
	// day := time.Now().Day()
msgloop:
	for update := range updates {
		msg := update.Message
		var tag string
		var args []string

		// skip empty messages
		if msg == nil {
			continue
		}

		// handle commands, do nothing if command does not exists
		cmd := msg.Command()
		if len(cmd) > 0 {
			switch cmd {
			case "reload":
				cfg = loadConfig(CONF)
			// @rotiscianibot keeps track of good and funny actions
			case "karma":
				// get karma for an user
				args = strings.Fields(msg.CommandArguments())
				if len(args) > 0 {
					tag = args[0]
					if len(tag) > 0 && regexp.MustCompile("^@").MatchString(tag) {
						var k int
						db.QueryRow("SELECT karma FROM karma WHERE username=$1 AND gid=$2", tag[1:], strconv.FormatInt(msg.Chat.ID, 10)).Scan(&k)
						bot.Send(tgbotapi.NewMessage(msg.Chat.ID, tag+" ha karma "+strconv.Itoa(k)))
						continue msgloop
					}
				}

				// join to interpolate ids and tg usernames
				rows, err := db.Query("SELECT username, karma FROM karma WHERE gid=$1 ORDER BY karma DESC", strconv.FormatInt(msg.Chat.ID, 10))
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
			// @rotiscianibot is a polite bot that makes compliments to people
			case "complimenti":
				args = strings.Fields(msg.CommandArguments())
				if len(args) > 0 {
					tag = args[0]
					if len(tag) > 0 && regexp.MustCompile("^@").MatchString(tag) {
						bot.Send(tgbotapi.NewMessage(msg.Chat.ID, tag+", "+cfg.Appreciation[rand.Intn(len(cfg.Appreciation))]))
					}
				}
			// @rotiscianibot can also send emails!
			case "mail":
				sendMail(config, "Hi!", mailbody)
			}
		}
		// clear the eaters list on midnight
		//if newday := time.Now().Day(); newday != day {
		//	day = newday
		//	eaters = map[int64]map[string]bool{}
		//}

		// if we received a location or venue, save the user position in a map
		//if msg.Location != nil || msg.Venue != nil {
		//	loc := msg.Location
		//	if loc == nil {
		//		loc = &msg.Venue.Location
		//	}
		//	positions[msg.From.ID] = strconv.FormatFloat(loc.Latitude, 'f', -1, 64) + "," + strconv.FormatFloat(loc.Longitude, 'f', -1, 64)
		//	continue
		//}

		// save mention and commands in a variable, but only if there is one of them
		// leading / and @ is stripped from mentions and commands
		if msg.Entities != nil && len(*msg.Entities) <= 2 {
			switch e := (*msg.Entities)[0]; e.Type {
			case "mention":
				tag = msg.Text[e.Offset+1 : e.Offset+e.Length]
			case "text_mention":
				// unsupported yet
			}
		}

		switch {
		case in(msg.Text, bot.Self.FirstName):
			// be quiet if someone ask to
			if v1.FindStringSubmatch(msg.Text) != nil || v2.FindStringSubmatch(msg.Text) != nil || v3.FindStringSubmatch(msg.Text) != nil {
				if conciseness < 8 {
					conciseness += 1
				}
				bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "Cercherò di essere meno logorroico, prometto."))
				continue msgloop
			// and be louder if someone call us!
			} else if conciseness > 0 {
				conciseness -= 1
			}
			// reply if someone says our name
			bot.Send(tgbotapi.NewMessage(msg.Chat.ID, cfg.Pongs[rand.Intn(len(cfg.Pongs))]))

		// google search if mentioned with a trailing '?'
		//case len(tag) > 0 && tag == bot.Self.UserName:
		//	if q := ask.FindStringSubmatch(msg.Text); len(q) > 1 {
		//		link := "boh"
		//		query.Del("searchType")
		//		query.Del("start")
		//		query.Set("q", q[1])
		//		gapi.RawQuery = query.Encode()
		//		get := gapi.String()
		//		resp, err := http.Get(get)
		//		// do the query to google and publish the first link
		//		if err == nil {
		//			body, err := ioutil.ReadAll(resp.Body)
		//			resp.Body.Close()
		//			var gresp struct {
		//				Items []struct {
		//					Link string `json:"link"`
		//				} `json:"items"`
		//			}
		//			err = json.Unmarshal(body, &gresp)
		//			if err == nil && len(gresp.Items) > 0 {
		//				link = gresp.Items[0].Link
		//			}
		//		}
		//		bot.Send(tgbotapi.NewMessage(msg.Chat.ID, link))
		//	}
		// get the id from the tg username
		case len(tag) > 0 && /* msg.Chat.IsGroup() && */ regexp.MustCompile("^@"+tag+"\\s*\\+\\+$").MatchString(msg.Text):
			karma := 1
			gid := strconv.FormatInt(msg.Chat.ID, 10)
			res, err := db.Exec("UPDATE KARMA SET karma=karma+1 WHERE username=$1 AND gid=$2", tag, gid)
			if err != nil {
				log.Panic(err)
			}
			if rows, _ := res.RowsAffected(); rows == 0 {
				// new user
				result, err := db.Exec("INSERT INTO karma VALUES($1, $2, 1)", tag, gid)
				_ = result
				if err != nil {
					log.Fatal(err)
				}
			}
			if db.QueryRow("SELECT karma FROM karma WHERE username=$1 AND gid=$2", tag, gid).Scan(&karma) == nil {
				bot.Send(tgbotapi.NewMessage(msg.Chat.ID, tag+" ha #karma "+strconv.Itoa(karma)))
			}
		// regular text search
		default:
			// should we answer?
			if (rand.Intn(100) + 1) < (100/2 ^ conciseness + 25) {
				// send voice notes on matching patterns
				for i, re := range cfg.Sounds.soundsre {
					if re.MatchString(msg.Text) {
						j := rand.Intn(len(cfg.Sounds.Sounds[i])-1) + 1
						// if we didn't send this note before, prepare a new upload
						if len(cfg.Sounds.soundsid[i][j]) == 0 {
							voice, err := bot.Send(tgbotapi.NewVoiceUpload(msg.Chat.ID, cfg.Sounds.Sounds[i][j]))
							if err == nil && voice.Voice != nil {
								cfg.Sounds.soundsid[i][j] = voice.Voice.FileID
							}
						} else {
							// otherwise reuse the cached ID to save people's bandwidth and space
							bot.Send(tgbotapi.NewVoiceShare(msg.Chat.ID, cfg.Sounds.soundsid[i][j]))
						}
						continue msgloop
					}
				}
				// replies on matching patterns
				for i, re := range cfg.repliesre {
					if re.MatchString(msg.Text) {
						reply := cfg.Replies[i][rand.Intn(len(cfg.Replies[i])-1)+1]
						bot.Send(tgbotapi.NewMessage(msg.Chat.ID, reply))
						continue msgloop
					}
				}
			}
		}
		// save last message for later use
		mailbody = msg.From.FirstName + " " + msg.From.LastName + ": " + msg.Text
		log.Println(mailbody)
	}
}
