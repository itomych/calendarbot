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

	"github.com/rickb777/date/timespan"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"

	"github.com/tucnak/store"
)

//AppConfig is a struct for storing application config
type AppConfig struct {
	Email         string
	CheckInterval time.Duration
}

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

	tok, err := config.Exchange(oauth2.NoContext, code)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web %v", err)
	}
	return tok
}

// tokenCacheFile generates credential file path/filename.
// It returns the generated credential path/filename.
func tokenCacheFile() string {
	tokenCacheDir := filepath.Join(os.Getenv("HOME"), ".credentials")
	os.MkdirAll(tokenCacheDir, 0700)
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
	defer f.Close()
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
	defer f.Close()
	json.NewEncoder(f).Encode(token)
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

var appConfig AppConfig
var calendarService *calendar.Service

func calendarChecker() {
	t := time.Now().Format(time.RFC3339)
	fmt.Printf("Checking calendar %s\n", t)

	events, err := calendarService.Events.List("primary").ShowDeleted(false).
		SingleEvents(true).TimeMin(t).MaxResults(100).OrderBy("startTime").Do()
	if err != nil {
		log.Fatalf("Unable to retrieve next ten of the user's events. %v", err)
	}

	fmt.Println("Upcoming events:")
	if len(events.Items) > 0 {
		counter := 0
		for _, i := range events.Items {

			eventTimeBegin, err := time.Parse(time.RFC3339, i.Start.DateTime)
			if err != nil {
				fmt.Println(err)
				continue
			}

			eventTimeEnd, err := time.Parse(time.RFC3339, i.End.DateTime)
			if err != nil {
				fmt.Println(err)
				continue
			}
			eventDateStart := getDayBeginning(eventTimeBegin)
			eventDateEnd := getDayEnd(eventTimeEnd)
			eventRange := timespan.NewTimeSpan(eventTimeBegin, eventTimeEnd)

			var attendance = findAttendee(appConfig.Email, i.Attendees)
			if attendance.ResponseStatus == "needsAction" || attendance.ResponseStatus == "tentative" {
				counter++
				fmt.Printf("%s (%s) %s\n", i.Summary, eventRange, attendance.ResponseStatus)
				//get the list of all events for the same day
				log.Printf("day: %s %s\n", eventDateStart.Format(time.RFC3339), eventDateEnd.Format(time.RFC3339))
				possibleEvents, err := calendarService.Events.List("primary").ShowDeleted(false).SingleEvents(true).TimeMin(eventDateStart.Format(time.RFC3339)).TimeMax(eventDateEnd.Format(time.RFC3339)).Do()

				if err != nil {
					fmt.Println(err)
					continue
				}

				log.Printf("\t found %d events for analysis", len(possibleEvents.Items))

				isIntersectionFound := false
				for _, possEvent := range possibleEvents.Items {

					var possEventAttendance = findAttendee(appConfig.Email, possEvent.Attendees)
					//find only accepted events
					if possEventAttendance.ResponseStatus != "accepted" {
						continue
					}

					possEventTimeBegin, err := time.Parse(time.RFC3339, possEvent.Start.DateTime)
					if err != nil {
						fmt.Println(err)
						continue
					}

					possEventTimeEnd, err := time.Parse(time.RFC3339, possEvent.End.DateTime)
					if err != nil {
						fmt.Println(err)
						continue
					}

					possEventRange := timespan.NewTimeSpan(possEventTimeBegin, possEventTimeEnd)

					//fmt.Printf("\tpossible event: %s (%s) %s\n", i.Summary, eventRange, attendance.ResponseStatus)

					if areTimespansIntersected(eventRange, possEventRange) {
						log.Printf("\tfound intersection with %s (%s)\n", possEvent.Summary, possEventRange)
						isIntersectionFound = true
						break
					}
				}

				if isIntersectionFound {
					log.Printf("\tdeclining event %s (%s)\n", i.Summary, eventRange)
					attendance.ResponseStatus = "declined"
				} else {
					log.Printf("\taccepting event %s (%s)\n", i.Summary, eventRange)
					attendance.ResponseStatus = "accepted"
				}

				_, err = calendarService.Events.Update("primary", i.Id, i).SendNotifications(true).Do()

				if err != nil {
					fmt.Println(err)
				}
			}
		}

		if counter == 0 {
			fmt.Printf("No upcoming events that needs action found.\n")
		}
	} else {
		fmt.Printf("No upcoming events found.\n")
	}
}

func main() {

	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		os.Exit(1)
	}()

	if err := store.Load("config.yaml", &appConfig); err != nil {
		log.Println("failed to load the config:", err)
		return
	}

	log.Printf("Calendar Checker. Using email %s. Check interval %d seconds", appConfig.Email, appConfig.CheckInterval)

	ctx := context.Background()

	b, err := ioutil.ReadFile("client_credentials.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved credentials
	// at ~/.credentials/calendar-go-quickstart.json
	config, err := google.ConfigFromJSON(b, calendar.CalendarScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(ctx, config)

	calendarService, err = calendar.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve calendar Client %v", err)
	}

	doEvery(appConfig.CheckInterval*time.Second, calendarChecker)
}
