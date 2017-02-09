package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/radovskyb/poller"
)

type NetEvent struct {
	typ  string
	msg  string
	addr net.Addr
}

func (e *NetEvent) Event() string {
	return fmt.Sprintf("%s: %s [%v]", e.typ, e.msg, e.addr)
}

type NetPoller struct {
	mu    *sync.Mutex
	conns []net.Conn
}

func NewNetPoller() *NetPoller {
	return &NetPoller{mu: &sync.Mutex{}}
}

func (np *NetPoller) PollFunc(evt chan poller.Eventer, errc chan error) {
	np.mu.Lock()
	defer np.mu.Unlock()

	// send a heartbeat message to every stored connection and
	// for any that return an error, remove them from the list
	// of stored conns.
	heartbeat := []byte("heartbeat\n")
	for i, conn := range np.conns {
		n, err := conn.Write(heartbeat)
		if err != nil || n < len(heartbeat) {
			if err := conn.Close(); err != nil {
				errc <- err
			}
			np.conns = append(np.conns[:i], np.conns[i+1:]...)
			evt <- &NetEvent{
				typ:  "connection close",
				msg:  fmt.Sprintf("closing connection %d", i),
				addr: conn.RemoteAddr(),
			}
			continue
		}
		evt <- &NetEvent{
			typ:  "heartbeat",
			msg:  fmt.Sprintf("Sent heartbeat to connection %d", i),
			addr: conn.RemoteAddr(),
		}
	}
}

func (np *NetPoller) AddConn(conn net.Conn) {
	fmt.Printf("added connection %s\n", conn.RemoteAddr().String())
	np.mu.Lock()
	np.conns = append(np.conns, conn)
	np.mu.Unlock()
}

func (np *NetPoller) Close() error {
	for _, conn := range np.conns {
		if err := conn.Close(); err != nil {
			log.Println(err)
		}
	}
	return nil
}

func main() {
	np := NewNetPoller()

	p := poller.New(np)
	go func() {
		for {
			select {
			case e := <-p.Event:
				fmt.Println(e.Event())
			case err := <-p.Error:
				fmt.Println(err)
			case <-p.Closed:
				return
			}
		}
	}()

	ln, err := net.Listen("tcp", "localhost:9000")
	if err != nil {
		log.Fatalln(err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Fatalln(err)
			}
			np.AddConn(conn)
		}
	}()

	c, done := make(chan os.Signal), make(chan struct{})
	signal.Notify(c, os.Interrupt, os.Kill)
	go func() {
		<-c
		if err := p.Close(); err != nil {
			log.Fatalln(err)
		}
		close(done)
	}()

	go func() {
		if err := p.Start(time.Second / 2); err != nil {
			log.Fatalln(err)
		}
	}()

	<-done
}
