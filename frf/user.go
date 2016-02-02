package frf

import (
	"encoding/json"
	"io"
	"net/http"
)

type User struct {
	Name        string // freffeed username
	AccessToken string
	DirectFeed  string
}

func (u *User) Sign(r *http.Request) *http.Request {
	r.Header.Add("X-Authentication-Token", u.AccessToken)
	return r
}

func (u *User) SendRequest(method string, url string, reqObj interface{}, respObj interface{}) error {
	var req *http.Request
	if reqObj == nil {
		req, _ = http.NewRequest(method, url, nil)
	} else {
		r, w := io.Pipe()
		go func() { json.NewEncoder(w).Encode(reqObj); w.Close() }()
		req, _ = http.NewRequest(method, url, r)
		req.Header.Add("Content-Type", "application/json; charset=utf-8")
	}
	u.Sign(req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ReadErrorResponse(resp)
	}

	if respObj != nil {
		return json.NewDecoder(resp.Body).Decode(respObj)
	}

	return nil
}
