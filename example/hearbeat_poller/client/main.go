package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
)

func main() {
	conn, err := net.Dial("tcp", "localhost:9000")
	if err != nil {
		log.Fatalln(err)
	}
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		fmt.Println(strings.TrimSpace(scanner.Text()))
	}
	if err := scanner.Err(); err != nil {
		log.Fatalln(err)
	}
}
