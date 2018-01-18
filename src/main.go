package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hashicorp/logutils"
	"github.com/jessevdk/go-flags"
	"github.com/rickb777/date/timespan"
	"github.com/tucnak/store"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
)

// Options is a struct for storing app config
type Options struct {
	Email         string `short:"e" env:"GOOGLE_EMAIL" description:"Email that bot will use to access Google Calendar"`
	CheckInterval int64  `short:"c" default:"60" env:"CHECK_INTERVAL" description:"Interval of checks in seconds"`
}

var opts Options

var revision = "unknown"

// getClient uses a Context and Config to retrieve a Token
// then generate a Client. It returns the generated Client.
func getClient(ctx context.Context, config *oauth2.Config) *http.Client {
	cacheFile := tokenCacheFile()
	tok, err := tokenFromFile(cacheFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(cacheFile, tok)
	}
	return config.Client(ctx, tok)
}

// getTokenFromWeb uses Config to request a Token.
// It returns the retrieved Token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var code string
	if _, err := fmt.Scan(&code); err != nil {
		log.Fatalf("Unable to read authorization code %v", err)
	}

	tok, err := config.Exchange(context.Background(), code)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web %v", err)
	}
	return tok
}

// tokenCacheFile generates credential file path/filename.
// It returns the generated credential path/filename.
func tokenCacheFile() string {
	tokenCacheDir := filepath.Join(os.Getenv("HOME"), ".credentials")
	err := os.MkdirAll(tokenCacheDir, 0700)
	if err != nil {
		log.Fatal(err)
	}

	return filepath.Join(tokenCacheDir,
		url.QueryEscape("itomych-calendar-bot.json"))
}

// tokenFromFile retrieves a Token from a given file path.
// It returns the retrieved Token and any read error encountered.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	t := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(t)
	defer func(f *os.File) {
		err = f.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(f)
	return t, err
}

// saveToken uses a file path to create a file and store the
// token in it.
func saveToken(file string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", file)
	f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}

	defer func(f *os.File) {
		err = f.Close()
		if err != nil {
			log.Fatalf("Unable to cache oauth token: %v", err)
		}
	}(f)

	err = json.NewEncoder(f).Encode(token)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
}

func findAttendee(email string, attendees []*calendar.EventAttendee) *calendar.EventAttendee {
	for _, val := range attendees {
		//log.Println("%T %s", val, val)
		//if &val. {
		//    return k
		//}
		if val.Email == email {
			return val
		}
	}
	return nil
}

func getDayBeginning(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}

func getDayEnd(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 23, 59, 59, 0, t.Location())
}

func areTimespansIntersected(t1, t2 timespan.TimeSpan) bool {
	return t1.Contains(t2.Start().Add(1*time.Minute)) || t1.Contains(t2.End().Add(-1*time.Minute))
}

func init() {
	// You must init store with some truly unique path first!
	store.Init("itomych/calendar-bot")
}

func doEvery(d time.Duration, f func()) {
	f()
	for range time.Tick(d) {
		f()
	}
}

var calendarService *calendar.Service

func analyzeIntersections(eventRange timespan.TimeSpan, possibleEvents *calendar.Events) bool {
	for _, possEvent := range possibleEvents.Items {

		var possEventAttendance = findAttendee(opts.Email, possEvent.Attendees)
		//find only accepted events
		if possEventAttendance.ResponseStatus != "accepted" {
			continue
		}

		possEventTimeBegin, beginParseError := time.Parse(time.RFC3339, possEvent.Start.DateTime)
		if beginParseError != nil {
			fmt.Println(beginParseError)
			continue
		}

		possEventTimeEnd, endParseError := time.Parse(time.RFC3339, possEvent.End.DateTime)
		if endParseError != nil {
			fmt.Println(endParseError)
			continue
		}

		possEventRange := timespan.NewTimeSpan(possEventTimeBegin, possEventTimeEnd)

		//fmt.Printf("\tpossible event: %s (%s) %s\n", i.Summary, eventRange, attendance.ResponseStatus)

		if areTimespansIntersected(eventRange, possEventRange) {
			log.Printf("[INFO] \tfound intersection with %s (%s)\n", possEvent.Summary, possEventRange)
			return true
		}
	}

	return false
}

func findPossibleEvents(start time.Time, end time.Time) (*calendar.Events, error) {
	possibleEvents, err := calendarService.Events.List("primary").ShowDeleted(false).SingleEvents(true).TimeMin(start.Format(time.RFC3339)).TimeMax(end.Format(time.RFC3339)).Do()
	return possibleEvents, err
}

func checkEvent(event *calendar.Event) bool {
	eventTimeBegin, err := time.Parse(time.RFC3339, event.Start.DateTime)
	if err != nil {
		fmt.Println(err)
		return true
	}

	eventTimeEnd, err := time.Parse(time.RFC3339, event.End.DateTime)
	if err != nil {
		fmt.Println(err)
		return true
	}
	eventDateStart := getDayBeginning(eventTimeBegin)
	eventDateEnd := getDayEnd(eventTimeEnd)
	eventRange := timespan.NewTimeSpan(eventTimeBegin, eventTimeEnd)

	var attendance = findAttendee(opts.Email, event.Attendees)
	if attendance.ResponseStatus == "needsAction" || attendance.ResponseStatus == "tentative" {

		fmt.Printf("%s (%s) %s\n", event.Summary, eventRange, attendance.ResponseStatus)
		//get the list of all events for the same day
		//log.Printf("day: %s %s\n", eventDateStart.Format(time.RFC3339), eventDateEnd.Format(time.RFC3339))
		possibleEvents, err := findPossibleEvents(eventDateStart, eventDateEnd)

		if err != nil {
			fmt.Println(err)
			return false
		}

		log.Printf("[INFO] \t found %d events for analysis", len(possibleEvents.Items))

		isIntersectionFound := analyzeIntersections(eventRange, possibleEvents)

		if isIntersectionFound {
			log.Printf("[INFO] \tdeclining event %s (%s)\n", event.Summary, eventRange)
			attendance.ResponseStatus = "declined"
		} else {
			log.Printf("[INFO] \taccepting event %s (%s)\n", event.Summary, eventRange)
			attendance.ResponseStatus = "accepted"
		}

		_, err = calendarService.Events.Update("primary", event.Id, event).SendNotifications(true).Do()

		if err != nil {
			fmt.Println(err)
		}

		return true
	}

	return false
}

func calendarChecker() {
	t := time.Now().Format(time.RFC3339)
	fmt.Printf("[INFO] Checking calendar %s\n", t)

	events, err := calendarService.Events.List("primary").ShowDeleted(false).
		SingleEvents(true).TimeMin(t).MaxResults(100).OrderBy("startTime").Do()
	if err != nil {
		log.Fatalf("[ERROR] Unable to retrieve next ten of the user's events. %v", err)
	}

	fmt.Println("[INFO] Upcoming events:")
	if len(events.Items) > 0 {
		counter := 0
		for _, i := range events.Items {
			var result = checkEvent(i)
			if result {
				counter++
			}
		}

		if counter == 0 {
			fmt.Printf("[INFO] No upcoming events that needs action found.\n")
		}
	} else {
		fmt.Printf("[INFO] No upcoming events found.\n")
	}
}

func setupLog(dbg bool) {
	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"DEBUG", "INFO", "WARN", "ERROR"},
		MinLevel: logutils.LogLevel("INFO"),
		Writer:   os.Stdout,
	}

	log.SetFlags(log.Ldate | log.Ltime)

	if dbg {
		log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
		filter.MinLevel = logutils.LogLevel("DEBUG")
	}
	log.SetOutput(filter)
}

func main() {
	fmt.Printf("calendarbot %s\n", revision)

	var parser = flags.NewParser(&opts, flags.Default)
	if _, err := parser.Parse(); err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		} else {
			log.Fatal(flagsErr.Message)
			os.Exit(1)
		}
	}
	setupLog(true)
	log.Printf("[INFO] options: %+v", opts)

	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		os.Exit(1)
	}()

	//if err := store.Load("config.yaml", &appConfig); err != nil {
	//	log.Println("[ERROR] failed to load the config:", err)
	//	return
	//}

	//log.Printf("[INFO] Calendar Checker. Using email %s. Check interval %d seconds", appConfig.Email, appConfig.CheckInterval)

	ctx := context.Background()

	b, err := ioutil.ReadFile("client_credentials.json")
	if err != nil {
		log.Fatalf("[ERROR] Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved credentials
	// at ~/.credentials/calendar-go-quickstart.json
	config, err := google.ConfigFromJSON(b, calendar.CalendarScope)
	if err != nil {
		log.Fatalf("[ERROR] Unable to parse client secret file to config: %v", err)
	}
	client := getClient(ctx, config)

	calendarService, err = calendar.New(client)
	if err != nil {
		log.Fatalf("[ERROR] Unable to retrieve calendar Client %v", err)
	}

	doEvery(time.Duration(opts.CheckInterval)*time.Second, calendarChecker)
}
