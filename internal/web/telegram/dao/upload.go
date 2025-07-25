// Package dao is a data access object for telegram Upload.
package dao

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	gconfig "github.com/Laisky/go-config/v2"
	"github.com/Laisky/zap"
	"github.com/minio/minio-go/v7"
	"go.mongodb.org/mongo-driver/bson"
	mongoLib "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/Laisky/laisky-blog-graphql/internal/web/telegram/model"
	"github.com/Laisky/laisky-blog-graphql/library/billing/oneapi"
	"github.com/Laisky/laisky-blog-graphql/library/db/arweave"
	"github.com/Laisky/laisky-blog-graphql/library/db/mongo"
)

const (
	colUploadUsers = "upload_users"
	colUploadFiles = "upload_files"
)

var oneapiKeyRegexp = regexp.MustCompile(`^(?:laisky|sk)-[a-zA-Z0-9]{48}$`)

// Upload db
type Upload struct {
	db    mongo.DB
	ar    *arweave.Ardrive
	minio *minio.Client
}

// NewUpload create new DB
func NewUpload(
	db mongo.DB,
	ar *arweave.Ardrive,
	minio *minio.Client,
) *Upload {
	return &Upload{
		db:    db,
		ar:    ar,
		minio: minio,
	}
}

func (d *Upload) GetUsersCol() *mongoLib.Collection {
	return d.db.GetCol(colUploadUsers)
}
func (d *Upload) GetFilesCol() *mongoLib.Collection {
	return d.db.GetCol(colUploadFiles)
}

func (d *Upload) ResetUser(ctx context.Context, telegramUID int64) error {
	_, err := d.GetUsersCol().
		DeleteOne(ctx, bson.M{"telegram_uid": telegramUID})
	if err != nil {
		return errors.Wrap(err, "reset user")
	}

	return nil
}

func (d *Upload) IsUserHasPermToUpload(ctx context.Context, telegramUID int64) (bool, error) {
	cnt, err := d.GetUsersCol().
		CountDocuments(ctx, bson.M{"telegram_uid": telegramUID})
	if err != nil {
		return false, errors.Wrap(err, "count user")
	}

	return cnt > 0, nil
}

func (d *Upload) GetUser(ctx context.Context, telegramUID int64) (user *model.UploadUser, err error) {
	user = new(model.UploadUser)
	err = d.GetUsersCol().
		FindOne(ctx, bson.M{"telegram_uid": telegramUID}).
		Decode(user)
	if err != nil {
		return nil, errors.Wrapf(err, "get user by telegram uid %d", telegramUID)
	}

	return user, nil
}

func (d *Upload) SaveOneapiUser(ctx context.Context, telegramUID int64, oneapiKey string) error {
	oneapiKey = strings.TrimSpace(oneapiKey)
	if !oneapiKeyRegexp.MatchString(oneapiKey) {
		return errors.New("invalid oneapi key")
	}

	_, err := d.GetUsersCol().
		UpdateOne(ctx,
			bson.M{"telegram_uid": telegramUID},
			bson.M{
				"$set": bson.M{
					"updated_at":   time.Now(),
					"oneapi_key":   oneapiKey,
					"billing_type": model.UploadBillingTypeOneapi,
				},
				"$setOnInsert": bson.M{
					"created_at":   time.Now(),
					"telegram_uid": telegramUID,
				},
			},
			options.Update().SetUpsert(true),
		)
	if err != nil {
		return errors.Wrap(err, "save oneapi user")
	}

	return nil
}

// OneapiBilling check user's billing
func (d *Upload) OneapiBilling(ctx context.Context, apikey string, size int64) error {
	price := max(size/1024/1024*int64(oneapi.PriceUploadFileEachMB), int64(oneapi.PriceUploadFileMinimal))

	if err := oneapi.CheckUserExternalBilling(ctx,
		apikey, oneapi.Price(price), "upload file"); err != nil {
		return errors.Wrap(err, "check user billing")
	}

	return nil
}

// UploadFileWithTelegramUID upload file to arweave with telegram uid
func (d *Upload) UploadFileWithTelegramUID(ctx context.Context,
	uid int64, cnt []byte, contentType string) (fileID string, err error) {
	logger := gmw.GetLogger(ctx)

	user, err := d.GetUser(ctx, uid)
	if err != nil {
		return fileID, errors.WithStack(err)
	}

	// upload
	fileID, err = d.UploadFileWithApikey(ctx, user.OneapiKey, cnt, contentType)
	if err != nil {
		return fileID, errors.Wrap(err, "upload file with apikey")
	}

	// save file info
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		_, err = d.db.GetCol(colUploadFiles).
			InsertOne(ctx, bson.M{
				"created_at":   time.Now(),
				"file_id":      fileID,
				"file_size":    len(cnt),
				"telegram_uid": uid,
			})
		if err != nil {
			logger.Error("save uploaded arweave file info", zap.Error(err))
		}
	}()

	return fileID, nil
}

// UploadFileWithApikey upload file to arweave with one-api's apikey
func (d *Upload) UploadFileWithApikey(ctx context.Context, apikey string, cnt []byte, contentType string) (fileID string, err error) {
	// check billing
	if err = d.OneapiBilling(ctx, apikey, int64(len(cnt))); err != nil {
		return fileID, errors.Wrap(err, "check billing")
	}

	return d.UploadFile(ctx, cnt, contentType)
}

// UploadFile upload file to arweave without any authentification
func (d *Upload) UploadFile(ctx context.Context, cnt []byte, contentType string) (fileID string, err error) {
	logger := gmw.GetLogger(ctx)
	fileID, err = d.ar.Upload(ctx, cnt,
		arweave.WithContentType(contentType),
	)
	if err != nil {
		return fileID, errors.Wrap(err, "upload to arweave")
	}

	// also upload to minio as cache
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		objkey := fmt.Sprintf(
			"%s/%s",
			strings.TrimSuffix(gconfig.S.GetString("settings.arweave.s3.prefix"), "/"),
			fileID,
		)
		_, err = d.minio.PutObject(ctx,
			gconfig.S.GetString("settings.arweave.s3.bucket"),
			objkey,
			bytes.NewReader(cnt),
			int64(len(cnt)),
			minio.PutObjectOptions{
				ContentType: contentType,
			},
		)
		if err != nil {
			logger.Error("upload to minio", zap.Error(err))
		}

		logger.Info("upload to minio", zap.String("objkey", objkey))
	}()

	return fileID, nil
}
