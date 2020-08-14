package frf

import (
	"net/http"
)

type User struct {
	Name        string // freefeed username
	AccessToken string
	DirectFeed  string
}

func (u *User) Sign(r *http.Request) *http.Request {
	r.Header.Add("Authorization", "Bearer "+u.AccessToken)
	return r
}
