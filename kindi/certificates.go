// Copyright (c) 2012 Uwe Hoffmann. All rights reserved.

package kindi

import (
	"appengine"
	"appengine/datastore"
	"appengine/memcache"
	"appengine/user"

	"crypto/x509"
	"encoding/json"
	"encoding/pem"

	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/uwedeportivo/shared/kindi"
	"github.com/uwedeportivo/shared/util"
)

type KindiCertificate struct {
	ID        string
	Email     string
	Name      string
	CertBytes []byte `datastore:",noindex"`
	Processed time.Time
	Effective time.Time
	Expires   time.Time
}

func parsePem(pemBytes []byte) (*pem.Block, error) {
	pemBlock, _ := pem.Decode(pemBytes)
	if pemBlock == nil {
		return nil, errors.New("Failed to decode pem")
	}
	return pemBlock, nil
}

func earlier(ta time.Time, tb time.Time) time.Time {
	if ta.After(tb) {
		return tb
	}
	return ta
}

func getUserCertificates(c appengine.Context, user *user.User) ([]KindiCertificate, error) {
	r := make([]KindiCertificate, 0)

	_, err := memcache.JSON.Get(c, user.ID+"-certs", &r)
	if err != nil && err != memcache.ErrCacheMiss {
		return nil, err
	}

	if err == memcache.ErrCacheMiss {
		accountKey := datastore.NewKey(c, "KindiAccount", user.ID, 0, nil)

		q := datastore.NewQuery("KindiCertificate").Ancestor(accountKey)
		_, err = q.GetAll(c, &r)
		if err != nil {
			return nil, err
		}

		memcacheItem := new(memcache.Item)
		memcacheItem.Key = user.ID + "-certs"
		memcacheItem.Object = r

		err = memcache.JSON.Set(c, memcacheItem)
		if err != nil {
			return nil, err
		}
	}

	return r, err
}

func rpcHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	emailStr := r.FormValue("emails")
	if emailStr == "" {
		http.Error(w, "no emails given", http.StatusInternalServerError)
		return
	}

	emails := strings.Split(emailStr, ",")
	if len(emails) == 0 {
		http.Error(w, "no emails given", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	jsonCerts := make([]kindi.JSONKindiCertificate, 0)

	for _, email := range emails {
		q := datastore.NewQuery("KindiCertificate").Filter("Email=", email)

		certs := make([]KindiCertificate, 0)
		_, err := q.GetAll(c, &certs)
		if err != nil {
			c.Errorf("error fetching certs: %v", err)
			http.Error(w, "error fetching certs", http.StatusInternalServerError)
			return
		}

		for _, cert := range certs {

			if cert.Expires.After(now) && cert.Effective.Before(now) {
				jsonCert := kindi.JSONKindiCertificate{
					Email: cert.Email,
					Bytes: cert.CertBytes,
				}
				jsonCerts = append(jsonCerts, jsonCert)
			}
		}
	}

	bodyJson, err := json.Marshal(jsonCerts)
	if err != nil {
		c.Errorf("error marshalling certs: %v", err)
		http.Error(w, "error marshalling certs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, string(bodyJson))
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	u := user.Current(c)
	if u == nil {
		http.Error(w, "no user", http.StatusInternalServerError)
		return
	}

	err := r.ParseForm()
	if err != nil {
		c.Errorf("error parsing form: %v", err)
		http.Error(w, "error parsing form", http.StatusInternalServerError)
		return
	}

	certIDs := r.Form["certs[]"]

	if certIDs == nil || len(certIDs) == 0 {
		c.Errorf("no certIDs found")
		http.Error(w, "no certIDs found", http.StatusInternalServerError)
		return
	}

	accountKey := datastore.NewKey(c, "KindiAccount", u.ID, 0, nil)
	keys := make([]*datastore.Key, len(certIDs))

	for i, id := range certIDs {
		keys[i] = datastore.NewKey(c, "KindiCertificate", id, 0, accountKey)
	}

	err = datastore.RunInTransaction(c, func(c appengine.Context) error {
		err := datastore.DeleteMulti(c, keys)
		if err != nil {
			return err
		}

		err = memcache.Delete(c, u.ID+"-certs")
		if err != nil && err != memcache.ErrCacheMiss {
			return err
		}

		return nil
	}, nil)

	if err != nil {
		c.Errorf("error deleting certs: %v", err)
		http.Error(w, "error deleting certs", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, "ok")
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	u := user.Current(c)
	if u == nil {
		http.Error(w, "no user", http.StatusInternalServerError)
		return
	}

	certStr := r.FormValue("certificate")
	if certStr == "" {
		http.Error(w, "no certificate", http.StatusInternalServerError)
		return
	}

	certName := r.FormValue("name")
	if certName == "" {
		certName = "Untitled"
	}

	pemBlock, err := parsePem([]byte(certStr))
	if err != nil {
		c.Errorf("error parsing PEM block: %v", err)
		http.Error(w, "error parsing PEM block", http.StatusInternalServerError)
		return
	}

	x509Cert, err := x509.ParseCertificate(pemBlock.Bytes)
	if err != nil {
		c.Errorf("error parsing certificate: %v", err)
		http.Error(w, "error parsing certificate", http.StatusInternalServerError)
		return
	}

	now := time.Now()

	kindiCert := KindiCertificate{
		ID:        util.UUID(),
		Email:     u.Email,
		Name:      certName,
		CertBytes: pemBlock.Bytes,
		Processed: now,
		Effective: x509Cert.NotBefore,
		Expires:   earlier(x509Cert.NotAfter, now.AddDate(1, 0, 0)),
	}

	account, err := getAccount(c, u.ID)
	if err != nil {
		c.Errorf("error retrieving account: %v", err)
		http.Error(w, "error retrieving account", http.StatusInternalServerError)
		return
	}

	if account.KindiCoins <= 0 {
		http.Error(w, "no kindi coins available", http.StatusInternalServerError)
		return
	}

	account.KindiCoins -= 1
	accountKey := datastore.NewKey(c, "KindiAccount", u.ID, 0, nil)
	certKey := datastore.NewKey(c, "KindiCertificate", kindiCert.ID, 0, accountKey)

	err = datastore.RunInTransaction(c, func(c appengine.Context) error {
		_, err := datastore.Put(c, accountKey, account)
		if err != nil {
			return err
		}

		_, err = datastore.Put(c, certKey, &kindiCert)
		if err != nil {
			return err
		}

		memcacheItem := &memcache.Item{
			Key:    u.ID,
			Object: *account,
		}

		err = memcache.Delete(c, u.ID+"-certs")
		if err != nil && err != memcache.ErrCacheMiss {
			return err
		}

		return memcache.JSON.Set(c, memcacheItem)
	}, nil)

	if err != nil {
		c.Errorf("error saving certificate: %v", err)
		http.Error(w, "error saving certificate", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, "ok")
}
