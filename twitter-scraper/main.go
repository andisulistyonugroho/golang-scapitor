package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
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
	From          string      `json:"from"`
	To            string      `json:"to"`
	CreatedAt     string      `json:"createdAt"`
	CreatedBy     int         `json:"createdBy"`
	StatusRunning bool        `json:"statusRunning"`
	ID            int         `json:"id"`
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
	// do scraping based on the ticket
	tweets := []twitterscraper.TweetResult{}
	tweets = searchingTweetByTicket(scrapingTicket)
	fmt.Println(tweets)
	jsonData := map[string]interface{}{
		"listTweet": tweets,
		"idScrap":   scrapingTicket.ID,
	}
	url := "/TwitScraps/insertTweetByUserAndSpecificKeyword"
	sendToLoopback(url, jsonData)
}

func getScrapingTicket() TwitScraps {
	baseUrl := goDotEnvVariable("baseUrl")
	resp, err := http.Get(baseUrl + `/TwitScraps/findOne?filter[where][statusRunning]=0&filter[order]=id%20desc`)
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

func searchingTweetByTicket(ticket TwitScraps) []twitterscraper.TweetResult {
	scraper := twitterscraper.New()
	scraper.SetSearchMode(twitterscraper.SearchLatest)
	scraper.WithDelay(2)

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
	if searchParam.From != "" {
		since, _ = time.Parse("2006-01-02", string(searchParam.From[0:10]))
	}
	if searchParam.To != "" {
		fmt.Println("TO ADA")
		today, _ = time.Parse("2006-01-02", string(searchParam.To[0:10]))

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

	// for tweet := range scraper.SearchTweets(context.Background(), queryString, 1000) {
	// 	if tweet.Error != nil {
	// 		panic(tweet.Error)
	// 	}
	// 	fmt.Println(*tweet)
	// 	tweets = append(tweets, *tweet)
	// }
	fmt.Println("hasile", tweets)

	return tweets
}

func sendToLoopback(url string, jsonData map[string]interface{}) {
	baseUrl := goDotEnvVariable("baseUrl")
	jsonVal, _ := json.Marshal(jsonData)
	resp, err := http.Post(baseUrl+url, "application/json", bytes.NewBuffer(jsonVal))

	if err != nil {
		log.Fatal(err)
	}

	var res map[string]interface{}

	json.NewDecoder(resp.Body).Decode(&res)
}
