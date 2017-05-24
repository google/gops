package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	"net/url"

	"github.com/google/gops/signal"
	"github.com/pkg/errors"
)

type Client interface {
	Run(byte) ([]byte, error)
	RunReader(byte) (io.ReadCloser, error)
}

type ClientTCP struct {
	addr net.TCPAddr
}

func (c *ClientTCP) Run(sig byte) ([]byte, error) {
	return c.run(sig)
}

func (c *ClientTCP) RunReader(sig byte) (io.ReadCloser, error) {
	return c.runLazy(sig)
}

func (c *ClientTCP) runLazy(sig byte) (io.ReadCloser, error) {
	conn, err := net.DialTCP("tcp", nil, &c.addr)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write([]byte{sig}); err != nil {
		return nil, err
	}
	return conn, nil
}

func (c *ClientTCP) run(sig byte) ([]byte, error) {
	r, err := c.runLazy(sig)
	defer r.Close()
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(r)
}

type ClientHTTP struct {
	baseAddr string
}

func (c *ClientHTTP) Run(sig byte) ([]byte, error) {
	r, err := c.RunReader(sig)
	defer r.Close()
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(r)
}

func (c *ClientHTTP) RunReader(sig byte) (io.ReadCloser, error) {
	action, ok := signal.ToParam(sig)
	if !ok {
		return nil, fmt.Errorf("unknown signal %v", sig)
	}
	client := &http.Client{
		Timeout: time.Second * 10,
	}

	u, err := url.Parse(c.baseAddr)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't parse dest URI")
	}
	u.Query().Add("action", action)

	rsp, err := client.Get(u.String())
	if err != nil {
		return nil, errors.Wrap(err, "error when making HTTP call")
	}
	return rsp.Body, nil
}
