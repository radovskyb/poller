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

type Event struct {
	typ  string
	msg  string
	addr net.Addr
}

func (e *Event) Type() string {
	return e.typ
}

func (e *Event) Message() string {
	return e.msg
}

func (e *Event) Addr() string {
	return e.addr.String()
}

type NetPoller struct {
	mu    *sync.Mutex
	conns []net.Conn
}

func NewNetPoller() *NetPoller {
	return &NetPoller{mu: &sync.Mutex{}}
}

func (np *NetPoller) PollFunc(evt chan poller.Event, errc chan error) {
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
			evt <- &Event{
				typ:  "connection close",
				msg:  fmt.Sprintf("closing connection: %d\n", i),
				addr: conn.RemoteAddr(),
			}
			continue
		}
		evt <- &Event{
			typ:  "heartbeat",
			msg:  fmt.Sprintf("Sent heartbeat to connection: %d\n", i),
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
			case event := <-p.Event:
				e := event.(*Event)
				fmt.Printf("type: %s, addr: %s\n", e.Type(), e.Addr())
			case err := <-p.Error:
				fmt.Println(err)
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

	if err := p.Start(time.Second * 2); err != nil {
		log.Fatalln(err)
	}

	<-done
}
