package dwstorage

import (
	"bytes"
	"context"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

var server *FileOperationsServer

const port = ":8080"
const url = "http://localhost" + port

func TestMain(m *testing.M) {
	var err error
	if server, err = NewFileOperationsServer("../bin/", "", port); err != nil {
		panic(err)
	}
	server.PreMiddlewareFunctions = []PreMiddlewareFunc{
		func(data []byte) ([]byte, error) {
			return bytes.ToUpper(data), nil
		},
	}
	go func() {
		if err = server.Start(context.Background()); err != nil {
			log.Fatalln(err)
		}
	}()
	code := m.Run()
	os.Exit(code)
}

func TestFileLifecycle(t *testing.T) {
	var buffer bytes.Buffer
	mp := multipart.NewWriter(&buffer)
	writer, err := mp.CreateFormFile("file", "file")
	if err != nil {
		panic(err)
	}
	if _, err = writer.Write([]byte(`capitalize Proverka`)); err != nil {
		panic(err)
	}
	if err = mp.Close(); err != nil {
		panic(err)
	}

	request, err := http.NewRequest("PUT", url+"/upload", &buffer)
	if err != nil {
		panic(err)
	}
	request.Header.Set("Content-Type", mp.FormDataContentType())
	result, err := http.DefaultClient.Do(request)
	if err != nil {
		panic(err)
	}
	assert.Equal(t, http.StatusOK, result.StatusCode)
	//TODO
}
