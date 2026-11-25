// Package imagevariant creates bounded, reusable display copies while preserving uploads.
package imagevariant

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

type profile struct{ width, height, quality int }

var profiles = map[string]profile{
	"small-v1":            {160, 160, 82},
	"thumb-v1":            {720, 1200, 85},
	"preview-v1":          {2048, 2048, 88},
	"background-v1":       {3840, 3840, 85},
	"background-small-v1": {1920, 1920, 85},
}

// Bound peak decode memory and CPU even when many uncached images arrive together.
var processing = make(chan struct{}, 1)
var extensions = []string{"jpg", "png", "webp"}

const maxBytes = 12 << 20

func cachePrefix(source string, stat os.FileInfo, kind string) string {
	key := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d", filepath.Base(source), stat.Size(), stat.ModTime().UnixNano())))
	return filepath.Join(filepath.Dir(source), ".previews", "v1", fmt.Sprintf("%x-%s", key, kind))
}

func cached(prefix string) string {
	for _, ext := range extensions {
		path := prefix + "." + ext
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
			return path
		}
	}
	return ""
}

// File requires the caller to authorize the original first. Cache files must never
// be exposed through an unguarded static route.
func File(ctx context.Context, source, kind string) (string, error) {
	p, ok := profiles[kind]
	if !ok {
		return "", errors.New("unknown image variant")
	}
	stat, err := os.Lstat(source)
	if err != nil || !stat.Mode().IsRegular() || stat.Size() < 1 || stat.Size() > maxBytes {
		return "", errors.New("invalid source image")
	}
	prefix := cachePrefix(source, stat, kind)
	if path := cached(prefix); path != "" {
		return path, nil
	}
	select {
	case processing <- struct{}{}:
		defer func() { <-processing }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if path := cached(prefix); path != "" {
		return path, nil
	}
	f, err := os.Open(source)
	if err != nil {
		return "", err
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	_ = f.Close()
	if err != nil || len(data) > maxBytes {
		return "", errors.New("image read failed")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 8192 || config.Height > 8192 || int64(config.Width)*int64(config.Height) > 32000000 {
		return "", errors.New("invalid image dimensions")
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	orientation := jpegOrientation(data)
	if orientation != 1 {
		src = orientedImage{Image: src, orientation: orientation}
	}
	bounds := src.Bounds()
	ratio := math.Min(1, math.Min(float64(p.width)/float64(bounds.Dx()), float64(p.height)/float64(bounds.Dy())))
	width, height := max(1, int(math.Round(float64(bounds.Dx())*ratio))), max(1, int(math.Round(float64(bounds.Dy())*ratio)))
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Src, nil)
	var output bytes.Buffer
	ext := "png"
	if dst.Opaque() {
		ext = "jpg"
		err = jpeg.Encode(&output, dst, &jpeg.Options{Quality: p.quality})
	} else {
		err = png.Encode(&output, dst)
	}
	if err != nil {
		return "", err
	}
	encoded := output.Bytes()
	// Do not increase transfer size for an already efficient static image.
	if orientation == 1 && format != "gif" && len(data) <= len(encoded) {
		encoded, ext = data, format
		if ext == "jpeg" {
			ext = "jpg"
		}
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(prefix), 0755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(prefix), ".preview-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	_, writeErr := tmp.Write(encoded)
	modeErr := tmp.Chmod(0644)
	closeErr := tmp.Close()
	if writeErr != nil || modeErr != nil || closeErr != nil {
		return "", errors.New("image cache write failed")
	}
	target := prefix + "." + ext
	if err = os.Rename(tmp.Name(), target); err != nil {
		return "", err
	}
	return target, nil
}

// Remove removes only this source's derived copies; callers delete the source separately.
func Remove(source string) {
	stat, err := os.Lstat(source)
	if err != nil {
		return
	}
	for kind := range profiles {
		prefix := cachePrefix(source, stat, kind)
		for _, ext := range extensions {
			_ = os.Remove(prefix + "." + ext)
		}
	}
}
