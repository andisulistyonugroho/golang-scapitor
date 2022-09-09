package main

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	twitterscraper "github.com/n0madic/twitter-scraper"
)

type (
	Date struct {
		from string
		to   string
	}

	UserTweetsBody struct {
		User      string   `json:"user"`
		Daterange []string `json:"daterange"`
		MaxTweet  int      `json:"maxTweet"`
		Keyword   string   `json:"keyword"`
	}
)

func main() {
	router := gin.Default()
	router.POST("/searchUserTweets", searchUserTweets)
	router.POST("/searchUserTweetsByKey", searchUserTweetsWithKeyword)
	router.POST("/getTweetsByKey", getTweetsByKey)
	router.Run("localhost:3019")
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
