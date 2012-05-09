// Copyright (c) 2012 Uwe Hoffmann. All rights reserved.

package kindi

import (
	"appengine"
	"appengine/user"

	"fmt"
	"html/template"
	"net/http"
	"time"
)

var manageTmpl *template.Template
var tableTmpl *template.Template

func init() {
	root := template.New("root")
	root = root.Funcs(template.FuncMap{"formatTime": FormatTime})
	root = template.Must(root.ParseFiles("tmpl/manage.html", "tmpl/certificates_table.html", "tmpl/payments.html"))
	manageTmpl = root.Lookup("manage.html")
	tableTmpl = manageTmpl.Lookup("certificates_table.html")
}

type ManageTmplData struct {
	Username     string
	KindiCoins   int
	Certificates []KindiCertificate
}

func FormatTime(args ...interface{}) string {
	ok := false
	var t time.Time
	if len(args) == 1 {
		t, ok = args[0].(time.Time)
	}
	if !ok {
		return fmt.Sprint(args...)
	}

	return t.Format("January 2, 2006 at 3:04 pm")
}

func manageHandler(w http.ResponseWriter, r *http.Request) {
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

	certs, err := getUserCertificates(c, u)
	if err != nil {
		c.Errorf("error retrieving certificates: %v", err)
		http.Error(w, "error retrieving certificates", http.StatusInternalServerError)
		return
	}

	data := ManageTmplData{
		Username:     u.String(),
		KindiCoins:   account.KindiCoins,
		Certificates: certs,
	}

	tableOnly := r.FormValue("tableOnly")

	if tableOnly == "" {
		err = manageTmpl.Execute(w, data)
	} else {
		if len(certs) > 0 {
			err = tableTmpl.Execute(w, data)
		} else {
			fmt.Fprintf(w, "<p></p>")
		}
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
	}
}
