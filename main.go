package main

import (
	"fmt"
	"unicode"
	"log"
	"os"
	"bufio"
	"net/http"
	"flag"

	"github.com/coreos/pkg/flagutil"
	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
	"github.com/fatih/color"
)

type AppState int

const (
	AppState_Main AppState = iota
	AppState_Tweet
	AppState_UserStream
	AppState_DM
	AppState_Events
)

var (
	cyan        = color.New(color.FgCyan)
	green       = color.New(color.FgGreen)
	red         = color.New(color.FgRed)
	blue        = color.New(color.FgBlue)
	config      *oauth1.Config
	token       *oauth1.Token
	client      *twitter.Client
	state       AppState
	running     bool
	newTweets   []*twitter.Tweet
	newDMs      []*twitter.DirectMessage
	newEvents   []*twitter.Event
	bufInput    *bufio.Reader
)

func equalsInsens(k rune, c rune) bool {
	return k == c || k == unicode.ToUpper(c)
}

func displayMenuOption(title string) {
	fmt.Print("> ")
	blue.Print(string(title[0]))
	fmt.Println(title[1:])
}

func displayMenu() {
	cyan.Println("\n*** fucking bird ***")
	displayMenuOption("tweet")

	var streamStr = "user stream (your timeline)"
	var ts = len(newTweets)
	if true {
		streamStr += fmt.Sprintf(" (%v new tweets)", ts)
	}
	displayMenuOption(streamStr)
	displayMenuOption("direct messages")
	displayMenuOption("events")
	displayMenuOption("quit")
}

func displayStartStreamMessage() {
	fmt.Print("Now streaming (")
	blue.Print("q")
	fmt.Println(" to quit)...\n")
}

func flushNewTweets() {
	for _, t := range newTweets {
		demuxTweet(t)
	}
	// make tweets eligible for gc
	newTweets = nil
}

func demuxTweet(tweet *twitter.Tweet) {
	if state == AppState_UserStream {
		cyan.Printf("\n%s (@%s):\n", tweet.User.Name, tweet.User.ScreenName)
		green.Println(tweet.CreatedAt)
		fmt.Printf("\n%s\n\n", tweet.Text)
		printIndicator("", "> ")
		bufInput.Reset(os.Stdin)
	} else {
		newTweets = append(newTweets, tweet)
	}
}

func demuxDM(dm *twitter.DirectMessage) {
	if state == AppState_DM {
		cyan.Printf("\n%s [%s]: %s\n", dm.Sender.Name, dm.CreatedAt, dm.Text)
		printIndicator("", "> ")
		bufInput.Reset(os.Stdin)
	} else {
		newDMs = append(newDMs, dm)
	}
}

func demuxEvent(event *twitter.Event) {
	if state == AppState_Events {
		printIndicator("", "> ")
		bufInput.Reset(os.Stdin)
	} else {
		newEvents = append(newEvents, event)
	}
}

func readTweetText() string {
	printIndicator("Enter tweet", "> ")
	str, err := bufInput.ReadString('\n')
	if err != nil {
		log.Fatal(err)
	}
	return str
}

func sendTweet(text string, params *twitter.StatusUpdateParams) (*twitter.Tweet, *http.Response, error) {
	return client.Statuses.Update(text, params)
}

func sendReplyTweet(text string, id int64) (*twitter.Tweet, *http.Response, error) {
	params := &twitter.StatusUpdateParams{InReplyToStatusID: id}
	return sendTweet(text, params)
}

func printIndicator(text string, indicator string) {
	blue.Print(text)
	fmt.Print(indicator)
}

func readInput(text string, indicator string) rune {
	printIndicator(text, indicator)
	str, err := bufInput.ReadString('\n')
	if err != nil {
		log.Fatal(err)
	}
	str = str[:len(str) - 1]
	if '\r' == str[len(str) - 1] {
		str = str[:len(str) - 1]
	}
	return rune(str[len(str) - 1])
}

func readKeyTyped() rune {
	//c := C.getch()
	return rune(0)
}

func handleInput(key rune) {
	switch state {
	case AppState_Main:
		if equalsInsens(key, 't') {
			state = AppState_Tweet
		} else if equalsInsens(key, 'u') {
			state = AppState_UserStream
			displayStartStreamMessage()
			flushNewTweets()
		} else if equalsInsens(key, 'd') {
			state = AppState_DM
		} else if equalsInsens(key, 'e') {
			state = AppState_Events
		} else if equalsInsens(key, 'q') {
			running = false
		}

	case AppState_UserStream:
		if equalsInsens(key, 'q') {
			state = AppState_Main
		}
	}
}

func main() {
	flags := flag.NewFlagSet("user-auth", flag.ExitOnError)
	consumerKey := flags.String("consumer-key", "", "Twitter Consumer Key")
	consumerSecret := flags.String("consumer-secret", "", "Twitter Consumer Secret")
	accessToken := flags.String("access-token", "", "Twitter Access Token")
	accessSecret := flags.String("access-secret", "", "Twitter Access Secret")
	flags.Parse(os.Args[1:])
	flagutil.SetFlagsFromEnv(flags, "TWITTER")

	if *consumerKey == "" || *consumerSecret == "" || *accessToken == "" || *accessSecret == "" {
		log.Fatal("Consumer key/secret and Access token/secret required")
	}

	bufInput = bufio.NewReader(os.Stdin)
	config = oauth1.NewConfig(*consumerKey, *consumerSecret)
	token = oauth1.NewToken(*accessToken, *accessSecret)
	
	// OAuth1 http.Client will automatically authorize Requests
	httpClient := config.Client(oauth1.NoContext, token)
	// Twitter Client
	client = twitter.NewClient(httpClient)

	// Convenience Demux demultiplexed stream messages
	demux := twitter.NewSwitchDemux()
	demux.Tweet = demuxTweet
	demux.DM = demuxDM
	demux.Event = demuxEvent
	userParams := &twitter.StreamUserParams{
	 	StallWarnings: twitter.Bool(true),
	 	With:          "followings",
		Language:      []string{"pt"},
	}
	stream, err := client.Streams.User(userParams)
	if err != nil {
		log.Fatal(err)
	}
	// Receive messages until stopped or stream quits
	go demux.HandleChan(stream.Messages)

	running = true
	for running {
		switch state {
		case AppState_Main:
			displayMenu()
			input := readInput("What now", "> ")
			handleInput(input)
		case AppState_UserStream:
			input := readInput("", "> ")
			handleInput(input)
		case AppState_Tweet:
			input := readTweetText()
			sendTweet(input, nil)
			state = AppState_UserStream
		}
		
	}

	stream.Stop()
}
