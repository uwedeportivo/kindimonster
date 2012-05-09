// Copyright (c) 2012 Uwe Hoffmann. All rights reserved.

package kindi

import (
	"appengine"
	"appengine/datastore"
	"appengine/memcache"
	"appengine/user"

	"fmt"
	"net/http"
)

type KindiAccount struct {
	KindiCoins int
	Email      string
}

func getAccount(c appengine.Context, userId string) (*KindiAccount, error) {
	var account KindiAccount

	_, err := memcache.JSON.Get(c, userId, &account)
	if err != nil && err != memcache.ErrCacheMiss {
		return nil, err
	}

	if err == memcache.ErrCacheMiss {
		key := datastore.NewKey(c, "KindiAccount", userId, 0, nil)
		err := datastore.Get(c, key, &account)
		if err != nil {
			return nil, err
		}
		memcacheItem := new(memcache.Item)
		memcacheItem.Key = userId
		memcacheItem.Object = account

		err = memcache.JSON.Set(c, memcacheItem)
		if err != nil {
			return nil, err
		}
	}
	return &account, nil
}

func getOrCreateAccount(c appengine.Context, user *user.User) (*KindiAccount, error) {
	var account KindiAccount

	_, err := memcache.JSON.Get(c, user.ID, &account)
	if err != nil && err != memcache.ErrCacheMiss {
		return nil, err
	}

	if err == memcache.ErrCacheMiss {
		key := datastore.NewKey(c, "KindiAccount", user.ID, 0, nil)

		return &account, datastore.RunInTransaction(c, func(c appengine.Context) error {
			err := datastore.Get(c, key, &account)
			if err != nil && err != datastore.ErrNoSuchEntity {
				return err
			}

			if err == datastore.ErrNoSuchEntity {
				account.Email = user.Email
				account.KindiCoins = 0
				_, err = datastore.Put(c, key, &account)
				if err != nil {
					return err
				}
			}

			memcacheItem := new(memcache.Item)
			memcacheItem.Key = user.ID
			memcacheItem.Object = account

			return memcache.JSON.Set(c, memcacheItem)
		}, nil)
	}
	return &account, nil
}

func coinsHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	u := user.Current(c)
	if u == nil {
		http.Error(w, "no user", http.StatusInternalServerError)
		return
	}

	account, err := getAccount(c, u.ID)
	if err != nil {
		c.Errorf("error retrieving account: %v", err)
		http.Error(w, "error retrieving account", http.StatusInternalServerError)
		return
	}

	var coins int = 0

	if account != nil {
		coins = account.KindiCoins
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "%d", coins)
}
