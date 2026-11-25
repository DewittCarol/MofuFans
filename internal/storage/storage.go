package storage

import (
	"bytes"
	"context"
	"github.com/spf13/viper"
)

type Storage interface {
	Upload(string, *bytes.Reader) (string, error)
	Delete(string) error
}
type uploads struct{}

func Uploads() *Assets {
	dir := viper.GetString("upload_directory")
	if dir == "" {
		dir = "data/uploads"
	}
	return NewAssets("uploads", dir)
}
func Get() Storage { return uploads{} }
func (uploads) Upload(name string, r *bytes.Reader) (string, error) {
	if err := Uploads().Put(context.Background(), name, r); err != nil {
		return "", err
	}
	return "/upload-img/" + name, nil
}
func (uploads) Delete(name string) error { return Uploads().Delete(context.Background(), name) }
