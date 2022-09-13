package main

import (
	"context"
	"fmt"
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

func main() {
	router := gin.Default()
	router.POST("/searchUserTweets", searchUserTweets)
	router.POST("/searchUserTweetsByKey", searchUserTweetsWithKeyword)
	router.POST("/getTweetsByKey", getTweetsByKey)
	router.Run("localhost:3019")
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

func searchUserTweets(ginContext *gin.Context) {
	var userTweetsBody UserTweetsBody
	scraper := twitterscraper.New()
	tweets := []twitterscraper.TweetResult{}

	var from string
	var to string

	if err := ginContext.BindJSON(&userTweetsBody); err != nil {
		return
	}

	query := "from:" + userTweetsBody.User

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

	rawParams := funk.Map(tweets, func(x twitterscraper.TweetResult) TweetParams {
		return TweetParams{ID: x.ID, Text: x.Text, TimeParsed: neo4j.DateOf(x.TimeParsed)}
	}).([]TweetParams)

	fmt.Printf("%+v\n", rawParams)

	// Neo4j

	params := convertToMapArray(rawParams)

	fmt.Printf("%+v\n", params)

	if err := saveToNeo4j(`UNWIND $props AS map
	MERGE (t:Twit {twitId: map.ID, message: map.Text, twitDate: map.TimeParsed})
	ON CREATE
	SET t.createdAt = $now`, map[string]interface{}{"props": params, "now": neo4j.DateOf(time.Now())}); err != nil {
		log.Fatal(err)
	}

	ginContext.IndentedJSON(http.StatusCreated, tweets)
}

func searchUserTweetsWithKeyword(ginContext *gin.Context) {
	var userTweetsBody UserTweetsBody
	scraper := twitterscraper.New()
	tweets := []twitterscraper.TweetResult{}
	query := userTweetsBody.Keyword + " from:" + userTweetsBody.User

	var from string
	var to string

	if err := ginContext.BindJSON(&userTweetsBody); err != nil {
		return
	}

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
		tweets = append(tweets, *tweet)
	}

	ginContext.IndentedJSON(http.StatusCreated, tweets)
}

func getTweetsByKey(ginContext *gin.Context) {
	var userTweetsBody UserTweetsBody
	scraper := twitterscraper.New()
	tweets := []twitterscraper.TweetResult{}
	query := userTweetsBody.Keyword

	var from string
	var to string

	if err := ginContext.BindJSON(&userTweetsBody); err != nil {
		return
	}

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

	ginContext.IndentedJSON(http.StatusCreated, tweets)
}

func getUserProfile(username string) twitterscraper.Profile {
	scraper := twitterscraper.New()
	profile, err := scraper.GetProfile(username)
	if err != nil {
		panic(err)
	}
	return profile
}
