package storageapi

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// checkHashSum сверяет хэш-суммы файла с заданными значениями.
func checkHashSum(data []byte, md5 string, sha1 string, sha256 string) error {
	if md5 != "" {
		if hash, err := getMD5(data); err != nil {
			return err
		} else if hash != md5 {
			return errors.New("md5-hashsum doesn't match")
		}
	}
	if sha1 != "" {
		if hash, err := getSHA1(data); err != nil {
			return err
		} else if hash != sha1 {
			return errors.New("sha1-hashsum doesn't match")
		}
	}
	if sha256 != "" {
		if hash, err := getSHA256(data); err != nil {
			return err
		} else if hash != sha256 {
			return errors.New("sha256-hashsum doesn't match")
		}
	}
	return nil
}

func getMD5(data []byte) (string, error) {
	hash := md5.New()
	if _, err := hash.Write(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func getSHA1(data []byte) (string, error) {
	hash := sha1.New()
	if _, err := hash.Write(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func getSHA256(data []byte) (string, error) {
	hash := sha256.New()
	if _, err := hash.Write(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
