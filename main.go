package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/fatih/structs"
	"github.com/gin-gonic/gin"
	twitterscraper "github.com/n0madic/twitter-scraper"
	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
	"github.com/neo4j/neo4j-go-driver/v4/neo4j/dbtype"
	"github.com/spf13/viper"
	"github.com/thoas/go-funk"
)

type (
	UserTweetsBody struct {
		User      string   `json:"user"`
		Daterange []string `json:"daterange"`
		MaxTweet  int      `json:"maxTweet"`
		Keyword   string   `json:"keyword"`
	}

	TweetParams struct {
		ID         string
		Text       string
		TimeParsed dbtype.Date
		Username   string
	}

	TwitterAccount struct {
		Username  string
		UserID    string
		From      dbtype.Date
		To        dbtype.Date
		CreatedBy int
	}

	AccountBody struct {
		Username string `json:"username"`
	}

	LoopbackParam []struct {
		Account       string      `json:"account"`
		AccountID     interface{} `json:"accountId"`
		Keyword       string      `json:"keyword"`
		From          time.Time   `json:"from"`
		To            time.Time   `json:"to"`
		CreatedAt     time.Time   `json:"createdAt"`
		CreatedBy     int         `json:"createdBy"`
		StatusRunning int         `json:"statusRunning"`
		ID            int         `json:"id"`
	}
)

func viperEnvVar(key string) string {

	// SetConfigFile explicitly defines the path, name and extension of the config file.
	// Viper will use this and not check any of the config paths.
	// .env - It will search for the .env file in the current directory
	viper.SetConfigFile(".env")

	// Find and read the config file
	err := viper.ReadInConfig()

	if err != nil {
		log.Fatalf("Error while reading config file %s", err)
	}

	// viper.Get() returns an empty interface{}
	// to get the underlying type of the key,
	// we have to do the type assertion, we know the underlying value is string
	// if we type assert to other type it will throw an error
	value, ok := viper.Get(key).(string)

	// If the type is a string then ok will be true
	// ok will make sure the program not break
	if !ok {
		log.Fatalf("Invalid type assertion")
	}

	return value
}

var neo4jUri = viperEnvVar("uri")
var neo4jUsername = viperEnvVar("username")
var neo4jPassword = viperEnvVar("password")
var baseUrl = viperEnvVar("base_url")

func main() {
	params := UserTweetsBody{
		MaxTweet: 5,
	}
	doScrap(params)
	for _ = range time.Tick(time.Minute * 10) {
		doScrap(params)
	}

}

func doScrap(params UserTweetsBody) {
	var url string

	twitScrap := getNeoParam()
	fmt.Println(twitScrap[0])
	if !twitScrap[0].From.IsZero() {
		paramFrom, _ := time.Parse(time.RFC3339, twitScrap[0].From.Format(time.RFC3339))
		paramTo, _ := time.Parse(time.RFC3339, twitScrap[0].To.Format(time.RFC3339))
		params.Daterange = []string{paramFrom.Format("2006-01-02"), paramTo.Format("2006-01-02")}
	}
	params.Keyword = twitScrap[0].Keyword
	var tweets []twitterscraper.TweetResult
	var user twitterscraper.Profile
	var userList []TwitterAccount
	if twitScrap[0].Account != "" {
		fmt.Println("account ada")
		params.User = twitScrap[0].Account
		if twitScrap[0].Keyword != "" {
			fmt.Println("keyword ada")
			url = baseUrl + "/TwitScraps/insertTweetByUserAndSpecificKeyword"
			tweets, user = searchUserTweetsWithKeyword(params)
			jsonData := map[string]interface{}{
				"user":      user,
				"listTweet": tweets,
				"keyword":   params.Keyword,
				"idScrap":   twitScrap[0].ID,
			}
			sendToLoopback(url, jsonData)
		} else {
			fmt.Println("keyword kosong")
			url = baseUrl + "/TwitScraps/insertTweetByUser"
			tweets, user = searchUserTweets(params)
			jsonData := map[string]interface{}{
				"user":      user,
				"listTweet": tweets,
				"idScrap":   twitScrap[0].ID,
			}
			sendToLoopback(url, jsonData)
		}
	} else {
		fmt.Println("akun tidak ada")
		url = baseUrl + "/TwitScraps/insertTweetGlobally"
		tweets, userList = getTweetsByKey(params)
		jsonData := map[string]interface{}{
			"user":      userList,
			"listTweet": tweets,
			"keyword":   params.Keyword,
			"idScrap":   twitScrap[0].ID,
		}
		sendToLoopback(url, jsonData)
	}
	// scrapData := map[string]interface{}{
	// 	"statusRunning": 1,
	// }
	// updateScrapStatus(baseUrl+"/TwitScraps/"+strconv.Itoa(twitScrap[0].ID), scrapData)
}

func getNeoParam() LoopbackParam {
	resp, err := http.Get(baseUrl + `/TwitScraps?filter[where][statusRunning]=0&filter[order]=id%20desc`)

	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	var result LoopbackParam
	err = json.Unmarshal(body, &result)
	if err != nil {
		panic(err)
	}

	return result

}

func sendToLoopback(url string, jsonData map[string]interface{}) {
	jsonVal, _ := json.Marshal(jsonData)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonVal))

	if err != nil {
		log.Fatal(err)
	}

	var res map[string]interface{}

	json.NewDecoder(resp.Body).Decode(&res)
}

func updateScrapStatus(url string, jsonData map[string]interface{}) {
	client := &http.Client{}
	jsonVal, _ := json.Marshal(jsonData)
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewBuffer(jsonVal))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	log.Println(string(body))
}

func saveToNeo4j(query string, props map[string]interface{}) error {
	var (
		driver  neo4j.Driver
		session neo4j.Session
		tx      neo4j.Transaction
		result  neo4j.Result
		err     error
	)

	useConsoleLogger := func() func(config *neo4j.Config) {
		return func(config *neo4j.Config) {
			config.Log = neo4j.ConsoleLogger(neo4j.ERROR)
		}
	}

	// Construct a new driver
	if driver, err = neo4j.NewDriver(neo4jUri, neo4j.BasicAuth(neo4jUsername, neo4jPassword, ""), useConsoleLogger()); err != nil {
		return err
	}
	defer driver.Close()

	// Acquire a session
	if session, err = driver.Session(neo4j.AccessModeWrite); err != nil {
		return err
	}
	defer session.Close()

	if tx, err = session.BeginTransaction(); err != nil {
		return err
	}
	defer tx.Close()

	if result, err = tx.Run(query, props); err != nil {
		return err
	}

	for result.Next() {
		fmt.Println(result.Record().GetByIndex(0))
	}
	if err = result.Err(); err != nil {
		return err
	}

	return tx.Commit()
}

func convertToMapArray(rawParams []interface{}) []map[string]interface{} {
	var params = make([]map[string]interface{}, len(rawParams))

	for index, item := range rawParams {
		params[index] = structs.Map(item)
	}

	return params
}

func convertToMap(rawParams interface{}) map[string]interface{} {
	return structs.Map(rawParams)
}

func searchUserTweets(userTweetsBody UserTweetsBody) ([]twitterscraper.TweetResult, twitterscraper.Profile) {
	scraper := twitterscraper.New()
	tweets := []twitterscraper.TweetResult{}

	var from string
	var to string
	// var datefrom, dateto time.Time

	user := getAccount(userTweetsBody.User)

	var query = "from:" + userTweetsBody.User

	if userTweetsBody.Daterange != nil {
		date1, _ := time.Parse("2006-01-02", userTweetsBody.Daterange[0])
		date2, _ := time.Parse("2006-01-02", userTweetsBody.Daterange[1])
		if compareDate := date1.Before(date2); compareDate == bool(true) {
			from = userTweetsBody.Daterange[0]
			to = userTweetsBody.Daterange[1]
		} else {
			from = userTweetsBody.Daterange[1]
			to = userTweetsBody.Daterange[0]
		}
		query = "from:" + userTweetsBody.User + " since:" + from + " until:" + to
	}

	for tweet := range scraper.SearchTweets(context.Background(), query, userTweetsBody.MaxTweet) {
		if tweet.Error != nil {
			panic(tweet.Error)
		}
		tweets = append(tweets, *tweet)
	}
	println(tweets)

	return tweets, user
}

func searchUserTweetsWithKeyword(userTweetsBody UserTweetsBody) ([]twitterscraper.TweetResult, twitterscraper.Profile) {
	scraper := twitterscraper.New()
	tweets := []twitterscraper.TweetResult{}

	var from string
	var to string
	// var datefrom, dateto time.Time

	user := getAccount(userTweetsBody.User)

	query := userTweetsBody.Keyword + " from:" + userTweetsBody.User

	if userTweetsBody.Daterange != nil {
		date1, _ := time.Parse("2006-01-02", userTweetsBody.Daterange[0])
		date2, _ := time.Parse("2006-01-02", userTweetsBody.Daterange[1])
		if compareDate := date1.Before(date2); compareDate == bool(true) {
			from = userTweetsBody.Daterange[0]
			to = userTweetsBody.Daterange[1]
		} else {
			from = userTweetsBody.Daterange[1]
			to = userTweetsBody.Daterange[0]
		}
		query = userTweetsBody.Keyword + " from:" + userTweetsBody.User + " since:" + from + " until:" + to
	}

	for tweet := range scraper.SearchTweets(context.Background(), query, userTweetsBody.MaxTweet) {
		if tweet.Error != nil {
			panic(tweet.Error)
		}
		fmt.Println(*tweet)
		tweets = append(tweets, *tweet)
	}

	return tweets, user
}

func getTweetsByKey(userTweetsBody UserTweetsBody) ([]twitterscraper.TweetResult, []TwitterAccount) {
	scraper := twitterscraper.New()
	tweets := []twitterscraper.TweetResult{}

	var from string
	var to string

	query := userTweetsBody.Keyword

	if userTweetsBody.Daterange != nil {
		date1, _ := time.Parse("2006-01-02", userTweetsBody.Daterange[0])
		date2, _ := time.Parse("2006-01-02", userTweetsBody.Daterange[1])
		if compareDate := date1.Before(date2); compareDate == bool(true) {
			from = userTweetsBody.Daterange[0]
			to = userTweetsBody.Daterange[1]
		} else {
			from = userTweetsBody.Daterange[1]
			to = userTweetsBody.Daterange[0]
		}
		query = userTweetsBody.Keyword + " since:" + from + " until:" + to
	}

	for tweet := range scraper.SearchTweets(context.Background(), query, userTweetsBody.MaxTweet) {
		if tweet.Error != nil {
			panic(tweet.Error)
		}
		tweets = append(tweets, *tweet)
	}

	userLists := funk.Map(tweets, func(x twitterscraper.TweetResult) string {
		return x.Username
	})

	users := funk.Map(userLists, func(username string) TwitterAccount {
		user := getAccount(username)
		return TwitterAccount{Username: user.Username, UserID: user.UserID, CreatedBy: 1}
	}).([]TwitterAccount)

	fmt.Println(tweets)

	return tweets, users
}

func getAccount(username string) twitterscraper.Profile {
	scraper := twitterscraper.New()
	profile, err := scraper.GetProfile(username)
	if err != nil {
		panic(err)
	}
	return profile
}

func findAccount(ginContext *gin.Context) {
	var accountBody AccountBody

	if err := ginContext.BindJSON(&accountBody); err != nil {
		return
	}

	scraper := twitterscraper.New()
	profile, err := scraper.GetProfile(accountBody.Username)
	if err != nil {
		panic(err)
	}

	ginContext.IndentedJSON(http.StatusCreated, profile)
}
