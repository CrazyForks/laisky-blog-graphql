package web

import (
	"context"
	"fmt"
	"strings"
	"time"

	"laisky-blog-graphql/internal/global"
	blogSvc "laisky-blog-graphql/internal/web/blog/service"
	"laisky-blog-graphql/internal/web/general"
	"laisky-blog-graphql/library"
	"laisky-blog-graphql/library/jwt"
	"laisky-blog-graphql/library/log"

	ginMw "github.com/Laisky/gin-middlewares"
	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	jwtLib "github.com/form3tech-oss/jwt-go"
	"github.com/pkg/errors"
)

const (
	generalTokenName       = "general"
	maxTokenExpireDuration = 3600 * 24 * 7 * time.Second // 7d
)

func (r *Resolver) Lock() LockResolver {
	return &locksResolver{r}
}

// ===========================
// query
// ===========================

type locksResolver struct{ *Resolver }

// =================
// query resolver
// =================

func (r *queryResolver) Lock(ctx context.Context, name string) (*general.Lock, error) {
	return general.Service.LoadLockByName(ctx, name)
}

func (r *queryResolver) LockPermissions(ctx context.Context, username string) (users []*global.GeneralUser, err error) {
	log.Logger.Debug("LockPermissions", zap.String("username", username))
	users = []*global.GeneralUser{}
	var (
		prefixes []string
	)
	if username != "" {
		if prefixes = utils.Settings.GetStringSlice(
			"settings.general.locks.user_prefix_map." + username); prefixes != nil {
			users = append(users, &global.GeneralUser{
				LockPrefixes: prefixes,
			})
			return users, nil
		}
		return nil, errors.Errorf("user `%v` not exists", username)
	}

	for username = range utils.Settings.GetStringMap(
		"settings.general.locks.user_prefix_map") {
		users = append(users, &global.GeneralUser{
			Name: username,
			LockPrefixes: utils.Settings.GetStringSlice(
				"settings.general.locks.user_prefix_map." + username),
		})
	}
	return users, nil
}

// --------------------------
// gcp general resolver
// --------------------------
func (r *locksResolver) ExpiresAt(ctx context.Context,
	obj *general.Lock) (*library.Datetime, error) {
	return library.NewDatetimeFromTime(obj.ExpiresAt), nil
}

// ============================
// mutations
// ============================

func validateLockName(ownerName, lockName string) (ok bool) {
	for _, prefix := range utils.Settings.GetStringSlice(
		"settings.general.locks.user_prefix_map." + ownerName) {
		if strings.HasPrefix(lockName, prefix) {
			return true
		}
	}

	return false
}

/*
token (`general` in cookie):
::
	{
		"uid": "laisky",
		"exp": 4701974400
	}
*/
func validateAndGetGCPUser(ctx context.Context) (userName string, err error) {
	var token string
	if token, err = ginMw.
		GetGinCtxFromStdCtx(ctx).
		Cookie(generalTokenName); err != nil {
		return "", errors.Wrap(err, "get jwt token from ctx")
	}

	uc := &jwt.UserClaims{}
	if err = jwt.Instance.ParseClaims(token, uc); err != nil {
		return "", errors.Wrap(err, "parse jwt token")
	}

	return uc.Subject, nil
}

// AcquireLock acquire mutex lock with name and duration.
// if `isRenewal=true`, will renewal exists lock.
func (r *mutationResolver) AcquireLock(ctx context.Context,
	lockName string,
	durationSec int,
	isRenewal *bool,
) (ok bool, err error) {
	if durationSec > utils.Settings.GetInt("settings.general.locks.max_duration_sec") {
		return ok, fmt.Errorf("duration sec should less than %v",
			utils.Settings.GetInt("settings.general.locks.max_duration_sec"))
	}

	var username string
	if username, err = validateAndGetGCPUser(ctx); err != nil {
		log.Logger.Debug("user invalidate", zap.Error(err))
		return ok, err
	}

	if !validateLockName(username, lockName) {
		log.Logger.Warn("user want to acquire lock out of permission",
			zap.String("user", username),
			zap.String("lock", lockName))
		return ok, fmt.Errorf("`%v` do not have permission to acquire `%v`",
			username, lockName)
	}

	return general.Service.AcquireLock(ctx,
		lockName,
		username,
		time.Duration(durationSec)*time.Second,
		false)
}

// CreateGeneralToken generate genaral token than should be set as cookie `general`
func (r *mutationResolver) CreateGeneralToken(ctx context.Context,
	username string,
	durationSec int,
) (token string, err error) {
	log.Logger.Debug("CreateGeneralToken",
		zap.String("username", username),
		zap.Int("durationSec", durationSec))
	if time.Duration(durationSec)*time.Second > maxTokenExpireDuration {
		return "", errors.Errorf(
			"duration should less than %d, got %d",
			maxTokenExpireDuration,
			durationSec)
	}

	if _, err = blogSvc.Instance.ValidateAndGetUser(ctx); err != nil {
		return "", errors.Wrapf(err, "user `%v` invalidate", username)
	}

	uc := &jwt.UserClaims{
		StandardClaims: jwtLib.StandardClaims{
			Subject:   username,
			ExpiresAt: utils.Clock.GetUTCNow().Add(time.Duration(durationSec)).Unix(),
		},
	}
	if token, err = jwt.Instance.Sign(uc); err != nil {
		return "", errors.Wrap(err, "generate token")
	}

	return token, nil
}