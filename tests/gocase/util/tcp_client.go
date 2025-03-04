/*
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package util

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type TCPClient struct {
	c net.Conn
	r *bufio.Reader
	w *bufio.Writer
}

func newTCPClient(c net.Conn) *TCPClient {
	return &TCPClient{
		c: c,
		r: bufio.NewReader(c),
		w: bufio.NewWriter(c),
	}
}

func (c *TCPClient) Close() error {
	return c.c.Close()
}

func (c *TCPClient) ReadLine() (string, error) {
	r, err := c.r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(r, "\r\n"), nil
}

func (c *TCPClient) MustRead(t testing.TB, s string) {
	r, err := c.ReadLine()
	require.NoError(t, err)
	require.Equal(t, s, r)
}

func (c *TCPClient) MustMatch(t testing.TB, rx string) {
	r, err := c.ReadLine()
	require.NoError(t, err)
	require.Regexp(t, rx, r)
}

func (c *TCPClient) MustFail(t testing.TB) {
	r, err := c.ReadLine()
	require.Error(t, err)
	require.Empty(t, r)
}

func (c *TCPClient) Write(s string) error {
	_, err := c.w.WriteString(s)
	if err != nil {
		return err
	}
	return c.w.Flush()
}

func (c *TCPClient) WriteArgs(args ...string) error {
	if args == nil {
		return errors.New("args cannot be nil")
	}

	if len(args) == 0 {
		return errors.New("args cannot be empty")
	}

	cmd := fmt.Sprintf("*%d\r\n", len(args))
	for _, arg := range args {
		cmd = cmd + fmt.Sprintf("$%d\r\n%s\r\n", len(arg), arg)
	}

	return c.Write(cmd)
}
