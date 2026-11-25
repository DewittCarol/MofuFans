package imagevariant

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func testImage(t *testing.T, alpha bool) (string, []byte) {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, 1000, 1600))
	seed := uint32(12345)
	for y := 0; y < 1600; y++ {
		for x := 0; x < 1000; x++ {
			a := uint8(255)
			if alpha && x < 100 {
				a = 0
			}
			seed ^= seed << 13
			seed ^= seed >> 17
			seed ^= seed << 5
			im.SetNRGBA(x, y, color.NRGBA{uint8(seed), uint8(seed >> 8), uint8(seed >> 16), a})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "source.png")
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	return path, buf.Bytes()
}

func TestVariantsPreserveOriginalCacheAndBounds(t *testing.T) {
	for _, alpha := range []bool{false, true} {
		source, original := testImage(t, alpha)
		var wg sync.WaitGroup
		paths := make([]string, 4)
		for i := range paths {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				var err error
				paths[i], err = File(context.Background(), source, "thumb-v1")
				if err != nil {
					t.Error(err)
				}
			}(i)
		}
		wg.Wait()
		for _, path := range paths {
			if path != paths[0] {
				t.Fatal("parallel generation produced different paths")
			}
		}
		data, err := os.ReadFile(paths[0])
		if err != nil {
			t.Fatal(err)
		}
		im, format, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if im.Bounds().Dx() != 720 || im.Bounds().Dy() != 1152 {
			t.Fatalf("wrong bounds: %v", im.Bounds())
		}
		if alpha {
			if format != "png" {
				t.Fatal("alpha lost")
			}
			_, _, _, a := im.At(0, 0).RGBA()
			if a != 0 {
				t.Fatal("transparent corner flattened")
			}
		} else if format != "jpeg" {
			t.Fatal("opaque image not compressed")
		}
		if len(data) >= len(original) {
			t.Fatal("display copy was not smaller")
		}
		after, _ := os.ReadFile(source)
		if sha256.Sum256(after) != sha256.Sum256(original) {
			t.Fatal("original modified")
		}
		stat, _ := os.Stat(paths[0])
		again, err := File(context.Background(), source, "thumb-v1")
		if err != nil {
			t.Fatal(err)
		}
		stat2, _ := os.Stat(again)
		if !stat.ModTime().Equal(stat2.ModTime()) {
			t.Fatal("cache was regenerated")
		}
		Remove(source)
		if _, err := os.Stat(paths[0]); !os.IsNotExist(err) {
			t.Fatal("display cache not removed")
		}
		if _, err := os.Stat(source); err != nil {
			t.Fatal("cleanup removed original")
		}
	}
}

func TestVariantRejectsInvalidAndCancelledWork(t *testing.T) {
	source := filepath.Join(t.TempDir(), "bad.png")
	_ = os.WriteFile(source, []byte("not an image"), 0644)
	if _, err := File(context.Background(), source, "thumb-v1"); err == nil {
		t.Fatal("invalid input accepted")
	}
	if _, err := File(context.Background(), source, "../anything"); err == nil {
		t.Fatal("unbounded variant accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := File(ctx, source, "thumb-v1"); err == nil {
		t.Fatal("cancelled work processed")
	}
}

func TestJPEGOrientationIsAppliedWithoutChangingOriginal(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 180, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 180; x++ {
			im.Set(x, y, color.NRGBA{R: 230, B: uint8(x), A: 255})
		}
	}
	var original bytes.Buffer
	_ = jpeg.Encode(&original, im, &jpeg.Options{Quality: 90})
	tiff := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0}
	segment := append([]byte("Exif\x00\x00"), tiff...)
	header := []byte{0xff, 0xe1, 0, 0}
	binary.BigEndian.PutUint16(header[2:], uint16(len(segment)+2))
	data := append(append(append(append([]byte{}, original.Bytes()[:2]...), header...), segment...), original.Bytes()[2:]...)
	path := filepath.Join(t.TempDir(), "rotated.jpg")
	_ = os.WriteFile(path, data, 0644)
	variant, err := File(context.Background(), path, "preview-v1")
	if err != nil {
		t.Fatal(err)
	}
	preview, _ := os.ReadFile(variant)
	out, _, err := image.Decode(bytes.NewReader(preview))
	if err != nil {
		t.Fatal(err)
	}
	if out.Bounds().Dx() != 60 || out.Bounds().Dy() != 180 {
		t.Fatalf("EXIF rotation not applied: %v", out.Bounds())
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, data) {
		t.Fatal("EXIF original changed")
	}
	for _, bad := range [][]byte{nil, {0xff, 0xd8, 0xff}, {0xff, 0xd8, 0xff, 0xe1, 0xff, 0xff}} {
		if jpegOrientation(bad) != 1 {
			t.Fatal("malformed metadata accepted")
		}
	}
}
