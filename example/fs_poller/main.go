package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/radovskyb/poller"
)

type Event struct {
	typ string
}

func (e *Event) Type() string {
	return e.typ
}

type FilePoller struct {
	infos map[string]os.FileInfo
}

func (fp *FilePoller) PollFunc(evt chan poller.Event, errc chan error) {
	newinfos, err := fp.getFileInfos()
	if err != nil {
		errc <- err
		return
	}
	for path, info := range fp.infos {
		newinfo, found := newinfos[path]
		if !found {
			continue
		}
		if info.ModTime() != newinfo.ModTime() && !info.IsDir() {
			evt <- &Event{"Modified: " + path}
		}
	}
	fp.infos = newinfos
}

func (fp *FilePoller) getFileInfos() (map[string]os.FileInfo, error) {
	m := make(map[string]os.FileInfo)
	root, err := filepath.Abs(".")
	if err != nil {
		return nil, err
	}
	return m, filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(info.Name(), ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		m[path] = info
		return nil
	})
}

func (fp *FilePoller) Close() error {
	fmt.Println("closing file poller")
	return nil
}

func main() {
	fp := &FilePoller{infos: make(map[string]os.FileInfo)}
	infos, err := fp.getFileInfos()
	if err != nil {
		log.Fatalln(err)
	}
	fp.infos = infos

	p := poller.New(fp)

	go func() {
		for {
			select {
			case event := <-p.Event:
				fmt.Println(event.Type())
			case err := <-p.Error:
				fmt.Println(err)
			}
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

	if err := p.Start(time.Millisecond * 100); err != nil {
		log.Fatalln(err)
	}

	<-done
}
