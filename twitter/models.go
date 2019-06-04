package twitter

import (
	"fmt"
	"time"

	"github.com/Laisky/go-utils"
	"github.com/Laisky/laisky-blog-graphql/models"
	"github.com/Laisky/zap"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type TwitterDB struct {
	*models.DB
	tweets *mgo.Collection
}

type Tweet struct {
	ID        bson.ObjectId `bson:"_id" json:"mongo_id"`
	TID       int64         `bson:"id" json:"tweet_id"`
	CreatedAt time.Time     `bson:"created_at" json:"created_at"`
	Text      string        `bson:"text" json:"text"`
	Topics    []string      `bson:"topics" json:"topics"`
	User      *User         `bson:"user" json:"user"`
}

type User struct {
	ID         int32  `bson:"id" json:"id"`
	ScreenName string `bson:"screenname" json:"screenname"`
	Name       string `bson:"name" json:"name"`
	Dscription string `bson:"dscription" json:"dscription"`
}

const (
	DB_NAME        = "twitter"
	TWEET_COL_NAME = "tweets"
)

func NewTwitterDB(addr string) (db *TwitterDB, err error) {
	db = &TwitterDB{
		DB: &models.DB{},
	}
	if err = db.Dial(addr, DB_NAME); err != nil {
		return nil, err
	}

	db.tweets = db.GetCol(TWEET_COL_NAME)
	return db, nil
}

func (t *TwitterDB) LoadTweets(page, size int, topic, regexp string) (results []*Tweet, err error) {
	utils.Logger.Debug("LoadTweets",
		zap.Int("page", page), zap.Int("size", size),
		zap.String("topic", topic),
		zap.String("regexp", regexp),
	)

	if size > 100 || size < 0 {
		return nil, fmt.Errorf("size shoule in [0~100]")
	}

	results = []*Tweet{}
	var query = bson.M{}
	if topic != "" {
		query["topics"] = topic
	}

	if regexp != "" {
		query["text"] = bson.M{"$regex": bson.RegEx{regexp, "im"}}
	}

	if err = t.tweets.Find(query).Sort("-_id").Skip(page * size).Limit(size).All(&results); err != nil {
		return nil, err
	}

	return results, nil
}