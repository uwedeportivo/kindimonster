// Copyright (c) 2012 Uwe Hoffmann. All rights reserved.

package kindi

import (
	"appengine"
	"appengine/datastore"
	"appengine/memcache"
	"appengine/user"

	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/uwedeportivo/shared/jwt"
)

const (
	sellerIdentifier = "......"
	sellerSecret     = "......"
	promo            = "......"
	promoCount       = 100
)

type KindiOrder struct {
	Email      string
	OrderId    string
	Processed  time.Time
	KindiCoins int
}

type KindiPromo struct {
	Code string
}

func parseSellerData(sellerData string) (string, int, error) {
	parts := strings.Split(sellerData, ",")
	if len(parts) != 2 {
		return "", 0, errors.New("invalid seller data string")
	}

	var userId string
	var quantity int

	for i := 0; i < 2; i++ {
		kv := strings.Split(parts[i], ":")
		if len(kv) != 2 {
			return "", 0, errors.New("invalid seller data string")
		}
		if kv[0] == "userId" {
			userId = kv[1]
		} else if kv[0] == "quantity" {
			q, err := strconv.Atoi(kv[1])
			if err != nil {
				return "", 0, err
			}
			quantity = q
		} else {
			return "", 0, errors.New("invalid seller data string")
		}
	}
	return userId, quantity, nil
}

func processCoinOrder(c appengine.Context, orderId string, sellerData string) error {
	userId, quantity, err := parseSellerData(sellerData)
	if err != nil {
		return err
	}

	return processCoins(c, orderId, userId, quantity)
}

func processCoins(c appengine.Context, orderId string, userId string, quantity int) error {
	account, err := getAccount(c, userId)
	if err != nil {
		return err
	}

	account.KindiCoins += quantity

	order := KindiOrder{
		Email:      account.Email,
		OrderId:    orderId,
		Processed:  time.Now(),
		KindiCoins: quantity,
	}

	accountKey := datastore.NewKey(c, "KindiAccount", userId, 0, nil)
	orderKey := datastore.NewIncompleteKey(c, "KindiOrder", accountKey)

	return datastore.RunInTransaction(c, func(c appengine.Context) error {
		_, err := datastore.Put(c, accountKey, account)
		if err != nil {
			return err
		}
		_, err = datastore.Put(c, orderKey, &order)

		memcacheItem := &memcache.Item{
			Key:    userId,
			Object: *account,
		}

		return memcache.JSON.Set(c, memcacheItem)
	}, nil)
}

func buyHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	jot := r.FormValue("jwt")

	token, err := jwt.Decode(jot, sellerIdentifier, sellerSecret, false)
	if err != nil {
		c.Errorf("error decoding jwt: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// TODO(uwe): verify jwt more thoroughly
	if token.Response != nil && token.Request != nil {
		orderId := token.Response["orderId"]
		sellerData := token.Request["sellerData"]

		if orderId != "" && sellerData != "" {
			err = processCoinOrder(c, orderId, sellerData)

			if err != nil {
				c.Errorf("error processing jwt: %v %v", *token, err)
				http.Error(w, "error processing jwt", http.StatusInternalServerError)
				return
			}
			fmt.Fprint(w, orderId)
		} else {
			c.Errorf("invalid jwt: %v", *token)
			http.Error(w, "invalid jwt", http.StatusInternalServerError)
		}
	} else {
		c.Errorf("invalid jwt: %v", *token)
		http.Error(w, "invalid jwt", http.StatusInternalServerError)
	}
}

func promoHandler(c appengine.Context, u *user.User, w http.ResponseWriter, r *http.Request) {
	accountKey := datastore.NewKey(c, "KindiAccount", u.ID, 0, nil)
	q := datastore.NewQuery("KindiOrder").Ancestor(accountKey).Filter("OrderId=", promo)
	n, err := q.Count(c)
	if err != nil {
		c.Errorf("error processing promo: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if n > 0 {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, "promo used")
		return
	}

	q = datastore.NewQuery("KindiPromo").Filter("Code=", promo)
	n, err = q.Count(c)
	if err != nil {
		c.Errorf("error processing promo: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if n > promoCount {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, "promo expired")
		return
	}

	promoKey := datastore.NewIncompleteKey(c, "KindiPromo", nil)
	promoValue := &KindiPromo {
		Code: promo,
	}
	_, err = datastore.Put(c, promoKey, promoValue)
	if err != nil {
		c.Errorf("error processing promo: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = processCoins(c, promo, u.ID, 1)
	if err != nil {
		c.Errorf("error processing promo: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, "promo accepted")
	return
}

func jotHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	u := user.Current(c)
	if u == nil {
		http.Error(w, "no user", http.StatusInternalServerError)
		return
	}

	promoStr := r.FormValue("promo")
	if promoStr == promo {
		promoHandler(c, u, w, r)
		return
	}

	quantityStr := r.FormValue("quantity")
	if quantityStr == "" {
		quantityStr = "1"
	}

	quantity, err := strconv.Atoi(quantityStr)
	if err != nil {
		c.Errorf("error parsing quantity: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if quantity < 1 || quantity > 5 {
		c.Errorf("invalid quantity: %v", quantity)
		http.Error(w, "invalid quantity", http.StatusInternalServerError)
		return
	}

	sellerData := fmt.Sprintf("userId:%s,quantity:%d", u.ID, quantity)

	request := map[string]string{
		"name":         "......",
		"description":  ".......",
		"price":        quantityStr,
		"currencyCode": "......",
		"sellerData":   sellerData,
	}

	issued := time.Now()

	token := jwt.Token{
		Request:          request,
		Issued:           issued,
		Expires:          issued.Add(time.Hour),
		SellerIdentifier: sellerIdentifier,
		SellerSecret:     sellerSecret,
	}

	jot, err := jwt.Encode(token)
	if err != nil {
		c.Errorf("error encoding jwt: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, jot)
}
