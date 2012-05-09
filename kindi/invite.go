// Copyright (c) 2012 Uwe Hoffmann. All rights reserved.

package kindi

import (
	"bytes"
	"fmt"
	"appengine"
	"appengine/mail"
	"appengine/user"
	"appengine/urlfetch"

	"io/ioutil"
	"html/template"
	"net/http"
	"net/url"
	"strings"
)

const (
	recaptchaPrivateKey = "........."
	captchaURL = "http://www.google.com/recaptcha/api/verify"
)

var inviteTmpl *template.Template
var mailTmpl *template.Template

func init() {
	root := template.New("root")
	root = root.Funcs(template.FuncMap{"formatTime": FormatTime})
	root = template.Must(root.ParseFiles("tmpl/invite.html", "tmpl/payments.html"))
	inviteTmpl = root.Lookup("invite.html")
	mailTmpl = template.Must(template.ParseFiles("tmpl/mail.html"))
}

type InviteTmplData struct {
	Username     string
	KindiCoins   int
}

type MailTmplData struct {
	Recipient string
	Sender string
	Note string
}

func inviteHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	u := user.Current(c)
	if u == nil {
		c.Errorf("error sending email: no user")
		http.Error(w, "no user", http.StatusInternalServerError)
		return
	}

	recipient := r.FormValue("recipient")
	if recipient == "" {
		c.Errorf("error sending email: no recipient")
		http.Error(w, "no recipient", http.StatusInternalServerError)
		return	
	}

	captchaValues := url.Values{}
	captchaValues.Set("privatekey", recaptchaPrivateKey)
	captchaValues.Set("remoteip", r.RemoteAddr)
	captchaValues.Set("challenge", r.FormValue("recaptcha_challenge_field"))
	captchaValues.Set("response", r.FormValue("recaptcha_response_field"))

	captchaClient := urlfetch.Client(c)
	captchaResponse, err := captchaClient.PostForm(captchaURL, captchaValues)

	defer captchaResponse.Body.Close()
	captchaBody, err := ioutil.ReadAll(captchaResponse.Body)
	captchaLines := strings.Split(string(captchaBody), "\n")

	if len(captchaLines) > 0 && captchaLines[0] == "true" {
		mailData := MailTmplData{
			Recipient: recipient,
			Sender: u.Email,
			Note: r.FormValue("note"),
		}

		buf := new(bytes.Buffer)
		err = mailTmpl.Execute(buf, mailData)
		if err != nil {
			c.Errorf("error composing email: %v", err)
		    http.Error(w, "error composing email", http.StatusInternalServerError)
			return
		}

		msg := &mail.Message{
	        Sender:  u.Email,
	        To:      []string{recipient, u.Email},
	        Subject: "Upload certificate to kindi",
	        Body:    buf.String(),
		}
		if err := mail.Send(c, msg); err != nil {
			c.Errorf("error sending email: %v", err)
		    http.Error(w, "error sending email", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, "ok")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, "recaptcha")
}

func lookupHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	u := user.Current(c)
	if u == nil {
		url, err := user.LoginURL(c, r.URL.String())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Location", url)
		w.WriteHeader(http.StatusFound)
		return
	}

	account, err := getOrCreateAccount(c, u)
	if err != nil {
		c.Errorf("error retrieving account: %v", err)
		http.Error(w, "error retrieving account", http.StatusInternalServerError)
		return
	}

	data := InviteTmplData{
		Username:     u.String(),
		KindiCoins:   account.KindiCoins,
	}

	err = inviteTmpl.Execute(w, data)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}
}
