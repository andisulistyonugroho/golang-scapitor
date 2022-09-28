package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	twitterscraper "github.com/n0madic/twitter-scraper"
)

// A struct to map searching ticket
type TwitScraps struct {
	Account       string      `json:"account"`
	AccountID     interface{} `json:"accountId"`
	Keyword       string      `json:"keyword"`
	Since         string      `json:"since"`
	Until         string      `json:"until"`
	CreatedAt     string      `json:"createdAt"`
	CreatedBy     int         `json:"createdBy"`
	StatusRunning bool        `json:"statusRunning"`
	ID            int         `json:"id"`
}

type RequestDataGrouping struct {
	Id        int                          `json:"id"`
	DataGroup []twitterscraper.TweetResult `json:"dataGroup"`
}

// use godot package to load/read the .env file and
// return the value of the key
func goDotEnvVariable(key string) string {

	// load .env file
	err := godotenv.Load(".env")

	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	return os.Getenv(key)
}

func main() {
	// find out if we have a scraping to do
	scrapingTicket := getScrapingTicket()

	if scrapingTicket.ID > 0 {
		flagScrapingTicketRunState(scrapingTicket.ID, true)
		// do scraping based on the ticket
		tweets := []twitterscraper.TweetResult{}
		tweets = searchingTweetByTicket(scrapingTicket)
		// send to auraDB via API
		fmt.Println("ADA ", len(tweets), " RECORD")
		requestGroup := generateRequestGroup(tweets, scrapingTicket.ID)
		tellAPItoSaveInGraphDB(requestGroup)
		flagScrapingTicketRunState(scrapingTicket.ID, false)
	}
}

func getScrapingTicket() TwitScraps {
	baseUrl := goDotEnvVariable("baseUrl")
	params := url.Values{}
	today := time.Now()

	params.Add("filter", `{"where":{`+
		`"statusRunning":0,`+
		`"or":[`+
		`{"until":{"eq":null}},`+
		`{"until":{"gte":"`+today.Format("2006-01-02")+`"}}`+
		`]`+
		`}}`)

	resp, err := http.Get(baseUrl + "/TwitScraps/findOne?" + params.Encode())
	resp.Header.Set("Authorization", goDotEnvVariable("scraperbotToken"))
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	var result TwitScraps
	err = json.Unmarshal(body, &result)
	if err != nil {
		panic(err)
	}
	return result
}

func flagScrapingTicketRunState(id int, statusRunning bool) {
	jsonData := map[string]interface{}{
		"statusRunning": statusRunning,
	}
	url := "/TwitScraps/" + strconv.Itoa(id)
	baseUrl := goDotEnvVariable("baseUrl")
	payload, _ := json.Marshal(jsonData)
	client := &http.Client{}
	req, err := http.NewRequest(http.MethodPatch, baseUrl+url, bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", goDotEnvVariable("scraperbotToken"))

	resp, err := client.Do(req)

	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()
}

func searchingTweetByTicket(ticket TwitScraps) []twitterscraper.TweetResult {
	scraper := twitterscraper.New()
	scraper.SetSearchMode(twitterscraper.SearchLatest)
	scraper.WithDelay(15)

	tweets := []twitterscraper.TweetResult{}
	searchParam := ticket
	today := time.Now().UTC()
	since := today.AddDate(0, -1, 0)

	queryString := "-filter:retweets"

	if searchParam.Keyword != "" {
		queryString += " " + searchParam.Keyword
	}
	if searchParam.Account != "" {
		queryString += " (from:" + searchParam.Account + ")"
	}
	if searchParam.Since != "" {
		since, _ = time.Parse("2006-01-02", string(searchParam.Since[0:10]))
	}
	if searchParam.Until != "" {
		fmt.Println("TO ADA")
		today, _ = time.Parse("2006-01-02", string(searchParam.Until[0:10]))

	}

	for since.Before(today) || since.Equal(today) {
		until := since.AddDate(0, 0, 1)
		limitedQueryString := queryString + " since:" + since.Format("2006-01-02") + " until:" + until.Format("2006-01-02")

		// trim extra space begin and end of string
		limitedQueryString = strings.Trim(limitedQueryString, " ")
		fmt.Println("BBB:", limitedQueryString)
		fmt.Println("===")

		for tweet := range scraper.SearchTweets(context.Background(), limitedQueryString, 1000) {
			if tweet.Error != nil {
				panic(tweet.Error)
			}
			fmt.Println(*tweet)
			tweets = append(tweets, *tweet)
		}

		since = since.AddDate(0, 0, 1)
	}

	return tweets
}

func generateRequestGroup(tweets []twitterscraper.TweetResult, id int) []RequestDataGrouping {
	// split up all record to 20 rows per request
	splitter := 20
	rdg := []RequestDataGrouping{}
	tweets20 := []twitterscraper.TweetResult{}

	for i := 0; i < len(tweets); i++ {
		tweets20 = append(tweets20, tweets[i])
		if i > 0 && i%splitter == 0 {
			rdg = append(rdg, RequestDataGrouping{id, tweets20})
			tweets20 = []twitterscraper.TweetResult{}
		} else if i == len(tweets)-1 {
			rdg = append(rdg, RequestDataGrouping{id, tweets20})
		}
	}
	fmt.Println("ADA ", len(rdg), "GROUP")
	// c, _ := json.MarshalIndent(rdg, "", "\t")
	// fmt.Println(string(c))
	return rdg
}

func tellAPItoSaveInGraphDB(requestGroup []RequestDataGrouping) {
	url := "/TwitScraps/insert2GraphDB"

	for i := 0; i < len(requestGroup); i++ {
		jsonData := map[string]interface{}{
			"tweets": requestGroup[i].DataGroup,
			"id":     requestGroup[i].Id,
		}
		sendToLoopback(url, jsonData)
	}
}

func sendToLoopback(url string, jsonData map[string]interface{}) {
	baseUrl := goDotEnvVariable("baseUrl")
	fmt.Println("SENDING TO API", baseUrl+url)

	jsonVal, _ := json.Marshal(jsonData)

	resp, err := http.Post(baseUrl+url, "application/json", bytes.NewBuffer(jsonVal))
	resp.Header.Set("Authorization", goDotEnvVariable("scraperbotToken"))

	if err != nil {
		log.Fatal(err)
	}

	var res map[string]interface{}

	json.NewDecoder(resp.Body).Decode(&res)
}
