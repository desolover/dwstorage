package main

import (
	"bytes"
	"context"
	"flag"
	"log"
	"strconv"

	"github.com/desolover/telegue/dwstorage"
)

func main() {
	var (
		port               int
		redisConn          string
		workingDir         string
		rpsLimit, bpsLimit int
	)
	flag.IntVar(&port, "port", 8080, "listening port")
	flag.StringVar(&redisConn, "redis", "", "redis connection string")
	flag.StringVar(&workingDir, "dir", "./bin/", "working directory")
	flag.IntVar(&rpsLimit, "rps", 2, "requests per second limit")
	flag.IntVar(&bpsLimit, "bps", 1000000, "bytes per second limit")
	flag.Parse()

	server, err := dwstorage.NewFileOperationsServer(workingDir, redisConn, ":"+strconv.Itoa(port))
	if err != nil {
		log.Fatalln(err)
	}
	server.WorkingDir = workingDir
	server.RPSLimit = rpsLimit
	server.BPSLimit = bpsLimit
	server.PreMiddlewareFunctions = []dwstorage.PreMiddlewareFunc{
		capitalize,
	}
	server.PostMiddlewareFunctions = []dwstorage.PostMiddlewareFunc{
		printToLog,
	}
	if err := server.Start(context.Background()); err != nil {
		log.Fatalln(err)
	}
}

func capitalize(data []byte) ([]byte, error) {
	return bytes.ToUpper(data), nil
}

func printToLog(data []byte) error {
	log.Println(string(data))
	return nil
}
