package main

import (
	"context"
	"fmt"

	twitterscraper "github.com/n0madic/twitter-scraper"
)

type (
	Date struct {
		from string
		to   string
	}
)

func main() {
	date := Date{"2022-01-01", "2022-01-31"}
	maxTweet := 50
	tweets := searchUserTweetsWithKeyword("tournament", "light_kengo", date, maxTweet)

	fmt.Println(tweets)
}

func searchUserTweetsWith(user string, daterange Date, maxTweet int) []twitterscraper.TweetResult {
	scraper := twitterscraper.New()
	tweets := []twitterscraper.TweetResult{}
	query := "from:" + user

	if (daterange != Date{}) {
		query = "from:" + user + " since:" + daterange.from + " until:" + daterange.to
	}

	for tweet := range scraper.SearchTweets(context.Background(), query, maxTweet) {
		if tweet.Error != nil {
			panic(tweet.Error)
		}
		tweets = append(tweets, *tweet)
	}

	return tweets
}

func searchUserTweetsWithKeyword(keyword string, user string, daterange Date, maxTweet int) []twitterscraper.TweetResult {
	scraper := twitterscraper.New()
	tweets := []twitterscraper.TweetResult{}
	query := keyword + " from:" + user

	if (daterange != Date{}) {
		query = keyword + " from:" + user + " since:" + daterange.from + " until:" + daterange.to
	}

	for tweet := range scraper.SearchTweets(context.Background(), query, maxTweet) {
		if tweet.Error != nil {
			panic(tweet.Error)
		}
		tweets = append(tweets, *tweet)
	}

	return tweets
}

func getUserProfile(username string) twitterscraper.Profile {
	scraper := twitterscraper.New()
	profile, err := scraper.GetProfile(username)
	if err != nil {
		panic(err)
	}
	return profile
}

func getTweetsByKey(keyword string, daterange Date, maxTweet int) []twitterscraper.TweetResult {
	scraper := twitterscraper.New()
	tweets := []twitterscraper.TweetResult{}
	query := keyword

	if (daterange != Date{}) {
		query = keyword + " since:" + daterange.from + " until:" + daterange.to
	}

	for tweet := range scraper.SearchTweets(context.Background(), query, maxTweet) {
		if tweet.Error != nil {
			panic(tweet.Error)
		}
		tweets = append(tweets, *tweet)
	}

	return tweets
}
