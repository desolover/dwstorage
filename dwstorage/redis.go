package dwstorage

import (
	"context"
	"errors"
	"time"
)

// FileEntity сущность с мета-данными файла, которая хранится в Redis.
type FileEntity struct {
	Name           string    `json:"filename" redis:"filename"`
	UploadDate     time.Time `json:"upload_date" redis:"upload_date"`
	RemoveDate     time.Time `json:"remove_date" redis:"remove_date,omitempty"`
	IsRemoved      bool      `json:"is_removed" redis:"is_removed"`
	DownloadsCount int       `json:"downloads_count" redis:"downloads_count"`
}

func (fs *FileOperationsServer) createRedisFileEntity(ctx context.Context, fileName string) error {
	if fs.redisClient == nil {
		return nil
	}
	entity := FileEntity{
		Name:       fileName,
		UploadDate: time.Now(),
	}
	return fs.redisClient.HSet(ctx, fileName, entity).Err()
}
func (fs *FileOperationsServer) updateRedisFileEntity(ctx context.Context, fileName string) error {
	if fs.redisClient == nil {
		return nil
	}
	var entity FileEntity
	if err := fs.redisClient.HGetAll(ctx, fileName).Scan(&entity); err != nil {
		return err
	}
	entity.DownloadsCount++
	return fs.redisClient.HSet(ctx, fileName, entity).Err()
}
func (fs *FileOperationsServer) setAsDeletedRedisFileEntity(ctx context.Context, fileName string) error {
	if fs.redisClient == nil {
		return nil
	}
	var entity FileEntity
	if err := fs.redisClient.HGetAll(ctx, fileName).Scan(&entity); err != nil {
		return err
	}
	entity.RemoveDate = time.Now()
	entity.IsRemoved = true
	return fs.redisClient.HSet(ctx, fileName, entity).Err()
}

var ErrFileEntityNotFound = errors.New("file entity isn't found")

func (fs *FileOperationsServer) loadRedisFileEntity(ctx context.Context, fileName string) (*FileEntity, error) {
	if fs.redisClient == nil {
		return nil, nil
	}
	var entity FileEntity
	result := fs.redisClient.HGetAll(ctx, fileName)
	if len(result.Val()) == 0 {
		return nil, ErrFileEntityNotFound
	}
	if err := result.Scan(&entity); err != nil {
		return nil, err
	}
	return &entity, nil
}
