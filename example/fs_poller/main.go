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

type FSEvent struct {
	typ  string
	path string
}

func (e FSEvent) Event() string {
	return fmt.Sprintf("%s: %s", e.typ, e.path)
}

type FilePoller struct {
	infos map[string]os.FileInfo
}

func (fp *FilePoller) PollFunc(evt chan poller.Eventer, errc chan error) {
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
			evt <- &FSEvent{"Modified", path}
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
			case e := <-p.Event:
				fmt.Println(e.Event())
			case err := <-p.Error:
				fmt.Println(err)
			case <-p.Closed:
				return
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

	go func() {
		if err := p.Start(time.Millisecond * 100); err != nil {
			log.Fatalln(err)
		}
	}()

	<-done
}
