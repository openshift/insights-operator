// Copyright 2020 Red Hat, Inc
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"net/http"

	"github.com/gorilla/mux"
)

// MicroHTTPServer in an implementation of ServerInitializer interface
// This small implementation could help implementing tests without using
// a real HTTP server implementation
type MicroHTTPServer struct {
	Serv      *http.Server
	Router    *mux.Router
	APIPrefix string
}

// NewMicroHTTPServer creates a MicroHTTPServer for the given address and prefix
func NewMicroHTTPServer(address string, apiPrefix string) *MicroHTTPServer {
	router := mux.NewRouter().StrictSlash(true)
	server := &http.Server{Addr: address, Handler: router}
	return &MicroHTTPServer{
		APIPrefix: apiPrefix,
		Router:    router,
		Serv:      server,
	}
}

// TODO: consider renaming to something more obvious, it does not initialize anything

// Initialize returns the Handler instance in order to be modified
func (server *MicroHTTPServer) Initialize() http.Handler {
	return server.Router
}

// TODO: make it more flexible, at least an array of methods should be passed through arguments

// AddEndpoint adds a handler function to the router in order to response to the given endpoint
func (server *MicroHTTPServer) AddEndpoint(endpoint string, f func(http.ResponseWriter, *http.Request)) {
	realEndpoint := server.APIPrefix + endpoint
	server.Router.HandleFunc(realEndpoint, f).Methods(http.MethodGet)
}
