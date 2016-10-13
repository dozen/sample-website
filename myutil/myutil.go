package myutil

import (
	"io"
	"encoding/base64"
	"crypto/rand"
)

func RandStr(len int) string {
	r := make([]byte, len)
	_, err := io.ReadFull(rand.Reader, r)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(r)
}
