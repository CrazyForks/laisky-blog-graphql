// Package jwt converts jwt to internal jwt.
package jwt

import (
	"github.com/Laisky/errors/v2"
	gjwt "github.com/Laisky/go-utils/v5/jwt"
)

var Instance gjwt.JWT

func Initialize(secret []byte) (err error) {
	if Instance, err = gjwt.New(
		gjwt.WithSecretByte(secret),
		gjwt.WithSignMethod(gjwt.SignMethodHS256),
	); err != nil {
		return errors.Wrap(err, "new jwt")
	}

	return nil
}
