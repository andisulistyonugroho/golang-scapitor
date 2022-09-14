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
	router.GET("/findAccount", findAccount)
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
	var datefrom, dateto time.Time

	if err := ginContext.BindJSON(&userTweetsBody); err != nil {
		return
	}

	user := getAccount(userTweetsBody.User)

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

	tweetParams := funk.Map(tweets, func(x twitterscraper.TweetResult) TweetParams {
		return TweetParams{ID: x.ID, Text: x.Text, TimeParsed: neo4j.DateOf(x.TimeParsed)}
	}).([]TweetParams)

	datefrom, _ = time.Parse("2006-01-02", from)
	dateto, _ = time.Parse("2006-01-02", to)

	// Neo4j
	// Input Account
	queryAccount :=
		`MERGE (a:Account {accountName: $props.accountName, accountId: $props.accountId})
        ON CREATE
        SET a.createdAt = datetime(),
        a.createdBy = $props.createdBy,
        a.from = $props.from,
		a.to = $props.to`

	paramsAccount := map[string]interface{}{
		"accountName": user.Username,
		"accountId":   user.UserID,
		"from":        neo4j.DateOf(datefrom),
		"to":          neo4j.DateOf(dateto),
		"createdBy":   1,
	}

	if err := saveToNeo4j(queryAccount, map[string]interface{}{"props": paramsAccount}); err != nil {
		log.Fatal(err)
	}

	// Input Twit
	interfaceParams := make([]interface{}, 0)

	for _, data := range tweetParams {
		interfaceParams = append(interfaceParams, data)
	}

	params := convertToMapArray(interfaceParams)

	if err := saveToNeo4j(`UNWIND $props AS map
	MERGE (t:Twit {twitId: map.ID, message: map.Text, twitDate: map.TimeParsed})
	ON CREATE
	SET t.createdAt = datetime()`, map[string]interface{}{"props": params}); err != nil {
		log.Fatal(err)
	}

	//Input Relation
	queryRel :=
		`UNWIND $props AS map
        MATCH (a:Account {accountName: $username}), (t:Twit {twitId: map.ID, message: map.Text, twitDate: map.TimeParsed})
        MERGE (a)-[r:POSTING]->(t)
        ON CREATE` +
			"\nSET r.`tweet by user` = true\n" +
			"ON MATCH\nSET r.`tweet by user` = true"

	if err := saveToNeo4j(queryRel, map[string]interface{}{"props": params, "username": user.Username}); err != nil {
		log.Fatal(err)
	}

	ginContext.IndentedJSON(http.StatusCreated, tweets)
}

func searchUserTweetsWithKeyword(ginContext *gin.Context) {
	var userTweetsBody UserTweetsBody
	scraper := twitterscraper.New()
	tweets := []twitterscraper.TweetResult{}

	var from string
	var to string
	var datefrom, dateto time.Time

	if err := ginContext.BindJSON(&userTweetsBody); err != nil {
		return
	}

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
		tweets = append(tweets, *tweet)
	}

	tweetParams := funk.Map(tweets, func(x twitterscraper.TweetResult) TweetParams {
		return TweetParams{ID: x.ID, Text: x.Text, TimeParsed: neo4j.DateOf(x.TimeParsed)}
	}).([]TweetParams)

	datefrom, _ = time.Parse("2006-01-02", from)
	dateto, _ = time.Parse("2006-01-02", to)

	// Neo4j
	// Input Account
	queryAccount :=
		`MERGE (a:Account {accountName: $props.accountName, accountId: $props.accountId})
        ON CREATE
        SET a.createdAt = datetime(),
        a.createdBy = $props.createdBy,
        a.from = $props.from,
		a.to = $props.to`

	paramsAccount := map[string]interface{}{
		"accountName": user.Username,
		"accountId":   user.UserID,
		"from":        neo4j.DateOf(datefrom),
		"to":          neo4j.DateOf(dateto),
		"createdBy":   1,
	}

	if err := saveToNeo4j(queryAccount, map[string]interface{}{"props": paramsAccount}); err != nil {
		log.Fatal(err)
	}

	// Input Twit
	interfaceParams := make([]interface{}, 0)

	for _, data := range tweetParams {
		interfaceParams = append(interfaceParams, data)
	}

	params := convertToMapArray(interfaceParams)

	if err := saveToNeo4j(`UNWIND $props AS map
	MERGE (t:Twit {twitId: map.ID, message: map.Text, twitDate: map.TimeParsed})
	ON CREATE
	SET t.createdAt = datetime()`, map[string]interface{}{"props": params}); err != nil {
		log.Fatal(err)
	}

	//Input Relation
	queryRel :=
		`UNWIND $props AS map
        MATCH (a:Account {accountName: $username}), (t:Twit {twitId: map.ID, message: map.Text, twitDate: map.TimeParsed})
        MERGE (a)-[r:POSTING]->(t)
        ON CREATE` +
			"\nSET r.`tweet by user` = true\n" +
			"r.`" + userTweetsBody.Keyword + "` = true\n" +
			"ON MATCH\nSET r.`tweet by user` = true\n" +
			"r.`" + userTweetsBody.Keyword + "` = true"

	if err := saveToNeo4j(queryRel, map[string]interface{}{"props": params, "username": user.Username}); err != nil {
		log.Fatal(err)
	}

	ginContext.IndentedJSON(http.StatusCreated, tweets)
}

func getTweetsByKey(ginContext *gin.Context) {
	var userTweetsBody UserTweetsBody
	scraper := twitterscraper.New()
	tweets := []twitterscraper.TweetResult{}

	var from string
	var to string
	var datefrom, dateto time.Time

	if err := ginContext.BindJSON(&userTweetsBody); err != nil {
		return
	}

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

	tweetParams := funk.Map(tweets, func(x twitterscraper.TweetResult) TweetParams {
		return TweetParams{ID: x.ID, Text: x.Text, TimeParsed: neo4j.DateOf(x.TimeParsed), Username: x.Username}
	}).([]TweetParams)

	datefrom, _ = time.Parse("2006-01-02", from)
	dateto, _ = time.Parse("2006-01-02", to)

	userLists := funk.Map(tweets, func(x twitterscraper.TweetResult) string {
		return x.Username
	})

	users := funk.Map(userLists, func(username string) TwitterAccount {
		user := getAccount(username)
		return TwitterAccount{Username: user.Username, UserID: user.UserID, From: neo4j.DateOf(datefrom), To: neo4j.DateOf(dateto), CreatedBy: 1}
	}).([]TwitterAccount)

	// Neo4j
	// Input Account
	queryAccount :=
		`UNWIND $props as map
		MERGE (a:Account {accountName: map.Username, accountId: map.UserID})
	    ON CREATE
	    SET a.createdAt = datetime(),
	    a.createdBy = map.CreatedBy,
	    a.from = map.From,
		a.to = map.To`

	interfaceParamsAccount := make([]interface{}, 0)

	for _, data := range users {
		interfaceParamsAccount = append(interfaceParamsAccount, data)
	}

	ParamsAccount := convertToMapArray(interfaceParamsAccount)

	if err := saveToNeo4j(queryAccount, map[string]interface{}{"props": ParamsAccount}); err != nil {
		log.Fatal(err)
	}

	// Input Twit
	interfaceParams := make([]interface{}, 0)

	for _, data := range tweetParams {
		interfaceParams = append(interfaceParams, data)
	}

	queryTwit := `UNWIND $props AS map
	MERGE (t:Twit {twitId: map.ID, message: map.Text, twitDate: map.TimeParsed})
	ON CREATE
	SET t.createdAt = datetime()`
	paramsTwit := convertToMapArray(interfaceParams)

	if err := saveToNeo4j(queryTwit, map[string]interface{}{"props": paramsTwit}); err != nil {
		log.Fatal(err)
	}

	//Input Relation
	queryRel :=
		`UNWIND $props AS map
	    MATCH (a:Account {accountName: $username}), (t:Twit {twitId: map.ID, message: map.Text, twitDate: map.TimeParsed})
	    MERGE (a)-[r:POSTING]->(t)
	    ON CREATE` +
			"\nSET r.`" + userTweetsBody.Keyword + "` = true\n" +
			"ON MATCH\nSET r.`" + userTweetsBody.Keyword + "` = true"

	for _, akun := range ParamsAccount {
		paramTwitRel := make([]map[string]interface{}, 0)
		for _, twit := range paramsTwit {
			if akun["Username"] == twit["Username"] {
				paramTwitRel = append(paramTwitRel, twit)
			}
		}
		if err := saveToNeo4j(queryRel, map[string]interface{}{"props": paramTwitRel, "username": akun["Username"].(string)}); err != nil {
			log.Fatal(err)
		}
	}

	ginContext.IndentedJSON(http.StatusCreated, tweets)
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
