package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
)

const (
	port         = 5003
	printRequest = false
)

func main() {
	l, err := newRecordingListener("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Panicln(err)
	}

	conns := make(map[string]*recordingConn)
	handler := http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		c := conns[req.RemoteAddr]
		log.Printf("%p: request received\n", c)

		defer req.Body.Close()
		body, err := ioutil.ReadAll(req.Body)
		if err != nil {
			log.Printf("%p: reading body failed with %v\n", c, err)
			if printRequest {
				io.Copy(os.Stdout, bytes.NewReader(c.recording))
				fmt.Println()
			}
			return
		}

		if printRequest {
			log.Printf("%p: recorded %d bytes, body is %d bytes:\n", c, len(c.recording), len(body))
			io.Copy(os.Stdout, bytes.NewReader(c.recording))
			fmt.Println()
		}

		res.Header().Set("Connection", "close")
		res.Header().Set("Content-Length", fmt.Sprintf("%d", len(c.recording)))
		res.WriteHeader(200)

		io.Copy(res, bytes.NewReader(c.recording))
		log.Printf("%p: done\n", c)
	})

	srv := &http.Server{
		Handler: handler,
		ConnState: func(conn net.Conn, state http.ConnState) {
			c := conn.(*recordingConn)
			log.Printf("%p: transitioning to %s\n", c, state)

			switch state {
			case http.StateNew:
				conns[c.RemoteAddr().String()] = c
			case http.StateClosed:
				delete(conns, c.RemoteAddr().String())
			}
		},
	}
	err = srv.Serve(l)
	if err != nil {
		log.Panicln(err)
	}
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		if e, ok := dst[k]; ok {
			dst[k] = append(e, vv...)
		} else {
			dst[k] = vv
		}
	}
}

func newRecordingListener(network, laddr string) (net.Listener, error) {
	l, err := net.Listen(network, laddr)
	if err != nil {
		return nil, err
	}
	log.Printf("listening on %s/%s\n", network, laddr)
	return &recordingListener{l}, nil
}

type recordingListener struct {
	net.Listener
}

func (l *recordingListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	c := &recordingConn{Conn: conn}
	log.Printf("%p: accepted %s -> %s\n", c, c.RemoteAddr(), c.LocalAddr())
	return c, nil
}

type recordingConn struct {
	net.Conn
	recording []byte
}

func (c *recordingConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	if n > 0 {
		c.recording = append(c.recording, b[:n]...)
	}
	return
}
