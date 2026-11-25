package storage

import (
	"bytes"
	"github.com/spf13/viper"
	"io"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

type Local struct {
}

func localDirectory() string {
	if dir := viper.GetString("upload_directory"); dir != "" {
		return dir
	}
	return "frontend/public/upload-img"
}

func (l *Local) Upload(filename string, content *bytes.Reader) (string, error) {
	data, err := io.ReadAll(content)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(localDirectory(), 0755); err != nil {
		return "", err
	}
	err = os.WriteFile(filepath.Join(localDirectory(), filepath.Base(filename)), data, 0644)
	if err != nil {
		return "", err
	}
	return "upload-img/" + filename, nil
}

func (l *Local) Delete(filename string) error {
	err := os.Remove(filepath.Join(localDirectory(), filepath.Base(filename)))
	if err != nil {
		logrus.Error("remove file failed: ", err)
	}
	return err
}

func NewLocal() *Local {
	return &Local{}
}
