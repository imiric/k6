/*
 *
 * k6 - a next-generation load testing tool
 * Copyright (C) 2020 Load Impact
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as
 * published by the Free Software Foundation, either version 3 of the
 * License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package testutils

import (
	"net"
	"net/http"
	"net/http/httptest"
)

type HTTPServer struct {
	*httptest.Server
	addr string
}

func NewHTTPServer(handler http.HandlerFunc) *HTTPServer {
	return &HTTPServer{Server: &httptest.Server{
		Config: &http.Server{Handler: handler},
	}}
}

func (s *HTTPServer) ListenAndServe(addr string) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.addr = l.Addr().String()
	s.Server.Listener = l
	s.Start()
	return nil
}

func (s *HTTPServer) Addr() string {
	return s.addr
}
