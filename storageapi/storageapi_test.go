package storageapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// Может быть, здесь тест лучше было сделать более модульным - на каждую операцию свою функцию.
// Но сделал такой просто из соображений экономии своего времени.

var server *FileOperationsServer

const port = ":8080"
const url = "http://localhost" + port
const workingDir = "../bin/"

func TestMain(m *testing.M) {
	var err error
	if server, err = NewFileOperationsServer(workingDir, "", port); err != nil {
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

	time.Sleep(100 * time.Millisecond)
	code := m.Run()
	os.Exit(code)
}

func TestFileLifecycle(t *testing.T) {
	const fileData = `capitalize Proverka`

	// Загрузка.
	var buffer bytes.Buffer
	mp := multipart.NewWriter(&buffer)
	writer, err := mp.CreateFormFile("file", "file")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = writer.Write([]byte(fileData)); err != nil {
		t.Fatal(err)
	}
	if err = mp.Close(); err != nil {
		t.Fatal(err)
	}

	request, err := http.NewRequest("PUT", url+"/upload", &buffer)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", mp.FormDataContentType())
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatal("expected", http.StatusOK, "result", resp.StatusCode)
	}

	var uploadingResponse UploadHandlerResponse
	if err := json.NewDecoder(resp.Body).Decode(&uploadingResponse); err != nil {
		t.Fatal(err)
	}

	// Скачивание.
	resp, err = http.Get(url + "/download?filename=" + uploadingResponse.Filename)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatal("expected", http.StatusOK, "result", resp.StatusCode)
	}

	downloadedData, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(downloadedData) != strings.ToUpper(fileData) {
		t.Fatal("downloaded data doesn't match", err)
	}

	// Удаление.
	request, err = http.NewRequest("DELETE", url+"/delete?filename="+uploadingResponse.Filename, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatal("expected", http.StatusOK, "result", resp.StatusCode)
	}

	if _, err := os.Stat(workingDir + uploadingResponse.Filename[2:] + "/" + uploadingResponse.Filename); err == nil {
		t.Fatal("file was not removed")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
