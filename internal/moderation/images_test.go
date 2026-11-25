package moderation

import (
	"bytes"
	"github.com/spf13/viper"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestImagesReadOnlyFromUploadsAndResize(t *testing.T) {
	root := t.TempDir()
	viper.Set("upload_directory", root)
	t.Cleanup(func() { viper.Set("upload_directory", "") })
	var raw bytes.Buffer
	png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 1800, 2400)))
	name := "abcdef0123456789abcdef0123456789.png"
	os.WriteFile(filepath.Join(root, name), raw.Bytes(), 0600)
	images, e := LoadImages("upload-img/"+name, 1)
	if e != nil {
		t.Fatal(e)
	}
	info, _, e := image.DecodeConfig(bytes.NewReader(images[0].Data))
	if e != nil || info.Width > 2048 || info.Height > 2048 || info.Width*info.Height > 2000000 {
		t.Fatal("image bounds", info, e)
	}
	after, _ := os.ReadFile(filepath.Join(root, name))
	if !bytes.Equal(after, raw.Bytes()) {
		t.Fatal("original modified")
	}
	for _, path := range []string{"https://127.0.0.1/private.png", "upload-img/../config.json", "upload-img/missing.png", "upload-img/" + name + ";upload-img/" + name} {
		if _, e := LoadImages(path, 1); e == nil {
			t.Fatal("accepted invalid attachment", path)
		}
	}
	external := filepath.Join(t.TempDir(), name)
	os.WriteFile(external, raw.Bytes(), 0600)
	link := filepath.Join(root, "ffffffffffffffffffffffffffffffff.png")
	if os.Symlink(external, link) == nil {
		if _, e := LoadImages("upload-img/"+filepath.Base(link), 1); e == nil {
			t.Fatal("accepted escaped symlink")
		}
	}
}

func TestAnimatedGIFFallsBackToManual(t *testing.T) {
	frame := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.White, color.Black})
	var raw bytes.Buffer
	if e := gif.Encode(&raw, frame, nil); e != nil {
		t.Fatal(e)
	}
	if _, e := prepareImage(raw.Bytes()); e != nil {
		t.Fatal("static GIF rejected", e)
	}
	raw.Reset()
	if e := gif.EncodeAll(&raw, &gif.GIF{Image: []*image.Paletted{frame, frame}, Delay: []int{1, 1}}); e != nil {
		t.Fatal(e)
	}
	if _, e := prepareImage(raw.Bytes()); e == nil {
		t.Fatal("animated GIF accepted without examining all frames")
	}
}
