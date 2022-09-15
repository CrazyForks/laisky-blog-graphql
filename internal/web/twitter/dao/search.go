package dao

import (
	"github.com/Laisky/laisky-blog-graphql/internal/web/twitter/model"

	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type Search interface {
	SearchByText(text string) (tweetIDs []string, err error)
}

type sqlSearch struct {
	db *gorm.DB
}

func NewSQLSearch(db *gorm.DB) Search {
	return &sqlSearch{
		db: db,
	}
}

func (s *sqlSearch) SearchByText(text string) (tweetIDs []string, err error) {
	var tweets []model.SearchTweet
	err = s.db.Model(model.SearchTweet{}).
		// Where("text LIKE ?", "%"+text+"%").
		Where("match(text, ?)", text).
		Order("created_at DESC").
		Find(&tweets).Error
	if err != nil {
		return nil, errors.Wrapf(err, "search text `%s", text)
	}

	for i := range tweets {
		tweetIDs = append(tweetIDs, tweets[i].TweetID)
	}

	return tweetIDs, nil
}
