package dwstorage

import (
	"context"
	"net"
	"net/http"
	"sync"

	"github.com/redis/go-redis/v9"
)

// FileOperationsServer сервер файловых операций, который позволяет сохранять файлы в заданную директорию и метаданные в Redis.
// TODO: В случае, если бы это был просто обыкновенный сервис - вероятно, было бы логичнее написать этот код в императивном стиле.
// Но в пункте "реализовать сервис в виде отдельной библиотеки" меня немного смутило слово "библиотека", и такой вариант показался более подходящим.
// Т.к. тогда его будет более удобно использовать из других сервисов в случае импорта.
type FileOperationsServer struct {
	WorkingDir              string               // Директория для хранения каталогов с файлами.
	RPSLimit                int                  // Запросы в секунду, 0 означает отсутствие лимита.
	BPSLimit                int                  // Байты в секунду, 0 означает отсутствие лимита.
	PreMiddlewareFunctions  []PreMiddlewareFunc  // Список функций пред-обработки, которые будут вызваны обработчиком.
	PostMiddlewareFunctions []PostMiddlewareFunc // Список функций пост-обработки, которые будут вызваны обработчиком.
	address                 string
	redisClient             *redis.Client
	mux                     *http.ServeMux
	currentOperations       sync.Map
}

// NewFileOperationsServer создаёт новый экземпляр сервера.
func NewFileOperationsServer(workingDir string, redisConnString string, address string) (*FileOperationsServer, error) {
	server := FileOperationsServer{
		WorkingDir: workingDir,
		mux:        http.NewServeMux(),
		address:    address,
	}
	if redisConnString != "" {
		redisOptions, err := redis.ParseURL(redisConnString)
		if err != nil {
			return nil, err
		}
		server.redisClient = redis.NewClient(redisOptions)
	}
	server.mux.HandleFunc("PUT /upload", server.WrapHandler(uploadHandler))
	server.mux.HandleFunc("GET /download", server.WrapHandler(downloadHandler))
	server.mux.HandleFunc("DELETE /delete", server.WrapHandler(deleteHandler))
	server.mux.HandleFunc("GET /info", server.WrapHandler(infoHandler))
	return &server, nil
}

// Start проверяет подключение к Redis и запускает HTTP-сервер.
func (fs *FileOperationsServer) Start(ctx context.Context) error {
	if fs.RPSLimit > 0 || fs.BPSLimit > 0 {
		ticketsCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		go fs.StartTicketsCleaner(ticketsCtx)
	}

	if fs.redisClient != nil {
		if err := fs.redisClient.Ping(ctx).Err(); err != nil {
			return err
		}
	}

	srv := http.Server{
		Addr:        fs.address,
		Handler:     fs.mux,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	// параллельная обработка запросов обеспечивается пакетом 'http'
	return srv.ListenAndServe()
}
