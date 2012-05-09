// Copyright (c) 2012 Uwe Hoffmann. All rights reserved.

package kindi

import (
	"net/http"
)

func init() {
	http.HandleFunc("/manage", manageHandler)
	http.HandleFunc("/jot", jotHandler)
	http.HandleFunc("/coins", coinsHandler)
	http.HandleFunc("/buy", buyHandler)
	http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/delete", deleteHandler)
	http.HandleFunc("/invite", inviteHandler)
	http.HandleFunc("/lookup", lookupHandler)
	http.HandleFunc("/rpc/v1", rpcHandler)
}
