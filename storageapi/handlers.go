package storageapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"

	"github.com/google/uuid"
)

type HandlerFunc func(fs *FileOperationsServer, w http.ResponseWriter, r *http.Request) (any, error)

// Не уверен, что я полностью верно понял требование о вызове callback'ов:
// понимаю, что под коллбэком может подразумеваться, допустим,
// обратный HTTP-запрос на сторону клиента - к примеру, по завершению работы post-обработчика.

// PreMiddlewareFunc функция для pre-обработки файла - будет вызываться перед его сохранения с возможностью изменить данные.
type PreMiddlewareFunc func(data []byte) ([]byte, error)

// PostMiddlewareFunc функция для post-обработки файла - будет вызываться после его сохранения.
// TODO возможен другой вариант, где пост-обработчику передаются не данные файла,
// а путь сохранённого файла на диске (тогда в случае модификации файла придётся отказаться от распараллеливания работы с ним).
type PostMiddlewareFunc func(data []byte) error

func (fs *FileOperationsServer) WrapHandler(f HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := f(fs, w, r)
		if err != nil {
			w.WriteHeader(resp.(int))
			w.Write([]byte(err.Error()))
			return
		}
		if rawData, ok := resp.([]byte); ok {
			w.WriteHeader(http.StatusOK)
			w.Write(rawData)
			return
		}

		jsonResp, err := json.Marshal(resp)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(jsonResp)
	}
}

type UploadHandlerResponse struct {
	Filename string `json:"filename"`
}

func uploadHandler(fs *FileOperationsServer, _ http.ResponseWriter, r *http.Request) (any, error) {
	formFile, _, err := r.FormFile(`file`)
	if err != nil {
		return http.StatusBadRequest, fmt.Errorf(`param 'file' is invalid (must be a multipart-form file): %e`, err)
	}

	fileData, err := io.ReadAll(formFile)
	if err != nil {
		return http.StatusInternalServerError, err
	} else if len(fileData) == 0 {
		return http.StatusBadRequest, errors.New(`param 'file' is empty`)
	}

	if code, err := fs.checkLimitError(r, UploadOperationIndex, len(fileData)); err != nil {
		return code, err
	}

	if err := checkHashSum(fileData, r.FormValue("md5"), r.FormValue("sha1"), r.FormValue("sha256")); err != nil {
		return http.StatusBadRequest, err
	}

	for _, f := range fs.PreMiddlewareFunctions {
		if fileData, err = f(fileData); err != nil {
			return http.StatusInternalServerError, err
		}
	}

	var dstFile *os.File
	var fileName string
	for {
		// в цикле, т.к. теоретически возможно совпадение с названием уже существующего файла
		fileName = uuid.New().String()
		dirPath := fs.WorkingDir + getDirectoryName(fileName)
		if err := os.Mkdir(dirPath, os.ModePerm); err != nil && !os.IsExist(err) {
			return http.StatusInternalServerError, err
		}
		dstFile, err = os.OpenFile(dirPath+`/`+fileName, os.O_WRONLY|os.O_CREATE, os.ModePerm)
		if err == nil {
			break
		} else if !os.IsExist(err) {
			return http.StatusInternalServerError, err
		}
	}
	defer dstFile.Close() // в данном случае игнорирование потенциальной ошибки закрытия некритично, т.к. данные в файл всё равно будут сохранены

	if _, err := dstFile.Write(fileData); err != nil {
		return http.StatusInternalServerError, err
	}

	// в зависимости от того, нужно ли клиенту знать обо всех ошибках, либо хотя бы об одной, либо вообще нет,
	// можно заменить WaitGroup на канал с ошибками, либо вообще убрать
	var lastErr error
	var wg sync.WaitGroup
	for _, f := range fs.PostMiddlewareFunctions {
		wg.Add(1)
		function := f
		go func() {
			defer wg.Done()
			if lastErr = function(fileData); err != nil {
				// можно, к примеру, логгировать ошибку
			}
		}()
	}

	if err = fs.createRedisFileEntity(r.Context(), fileName); err != nil {
		return http.StatusInternalServerError, err
	}

	wg.Wait()
	if lastErr != nil {
		return http.StatusInternalServerError, err
	}

	return UploadHandlerResponse{Filename: fileName}, nil
}

func downloadHandler(fs *FileOperationsServer, _ http.ResponseWriter, r *http.Request) (any, error) {
	fileName := r.URL.Query().Get("filename")
	if len(fileName) < 2 {
		return http.StatusBadRequest, errors.New(`too short file name (url-value 'file')`)
	}
	dirPath := fs.WorkingDir + getDirectoryName(fileName)
	filePath := dirPath + `/` + fileName

	// TODO быть может, более целесообразно вместо двух запросов к ОС использовать один - сразу читать файл
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if code, err := fs.checkLimitError(r, DownloadOperationIndex, int(fileInfo.Size())); err != nil {
		return code, err
	}

	if err = fs.updateRedisFileEntity(r.Context(), fileName); err != nil {
		return http.StatusInternalServerError, err
	}

	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return http.StatusNotFound, err
	} else if err != nil {
		return http.StatusInternalServerError, err
	}

	return data, nil
}

func deleteHandler(fs *FileOperationsServer, _ http.ResponseWriter, r *http.Request) (any, error) {
	fileName := r.URL.Query().Get("filename")
	if len(fileName) < 2 {
		return http.StatusBadRequest, errors.New(`too short file name (url-value 'file')`)
	}

	dirName := getDirectoryName(fileName)
	fileDir := fs.WorkingDir + dirName
	filePath := fs.WorkingDir + dirName + `/` + fileName
	// Проверка "байт в секунду" при удалении - немножко странная метрика, но тоже сделана.
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if code, err := fs.checkLimitError(r, DeleteOperationIndex, int(fileInfo.Size())); err != nil {
		return code, err
	}

	if err := os.Remove(filePath); os.IsNotExist(err) {
		return http.StatusNotFound, err
	} else if err != nil {
		return http.StatusInternalServerError, err
	}

	dir, err := os.ReadDir(fileDir)
	if os.IsNotExist(err) {
		return http.StatusNotFound, err
	} else if err != nil {
		return http.StatusInternalServerError, err
	}
	if len(dir) == 0 {
		// потенциально возможна некая гонка состояний,
		// но к сожалению, не получается использовать os.Remove(), который бы удалял директорию, лишь если она пустая,
		// под Windows - ошибку не выдаёт, но и удаления директории тоже не происходит
		if err := os.RemoveAll(fileDir); err != nil {
			// ошибку можно залогировать, но не возвращать
			return http.StatusInternalServerError, err
		}
	}

	if err = fs.setAsDeletedRedisFileEntity(r.Context(), fileName); err != nil {
		return nil, err
	}

	return nil, nil
}

func infoHandler(fs *FileOperationsServer, _ http.ResponseWriter, r *http.Request) (any, error) {
	fileName := r.URL.Query().Get("filename")
	if len(fileName) < 2 {
		return http.StatusBadRequest, errors.New(`too short file name (url-value 'file')`)
	}

	if code, err := fs.checkLimitError(r, InfoOperationIndex, 0); err != nil {
		return code, err
	}

	if fs.redisClient == nil {
		return http.StatusMethodNotAllowed, errors.New("service is running in without-meta-data-mode")
	}

	info, err := fs.loadRedisFileEntity(r.Context(), fileName)
	if err == ErrFileEntityNotFound {
		return http.StatusNotFound, err
	} else if err != nil {
		return http.StatusInternalServerError, err
	}
	return info, nil
}

func (fs *FileOperationsServer) checkLimitError(r *http.Request, operation int, dataLength int) (int, error) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	isRPSLimited, isBPSLimited := fs.IsRequestAllowed(ClientOperationKey{IP: ip, Operation: operation}, dataLength)
	if isRPSLimited {
		return http.StatusTooManyRequests, errors.New("too many requests per second")
	} else if isBPSLimited {
		return http.StatusTooManyRequests, errors.New("too many bytes per second")
	}
	return 0, nil
}

func getDirectoryName(fileName string) string {
	return string([]rune(fileName)[:2])
}
