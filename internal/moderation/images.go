package moderation

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/spf13/viper"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"math"
	"mofufans/internal/ai"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var attachmentName = regexp.MustCompile(`^[a-fA-F0-9]{20,64}\.(jpg|jpeg|png|webp|gif)$`)

// Read only the configured upload directory. Never fetch a URL supplied by a submitter.
func LoadImages(paths string, expected int) ([]ai.Image, error) {
	names := strings.Split(paths, ";")
	if expected < 1 || expected > 6 || len(names) != expected {
		return nil, errors.New("附图数量不一致，需人工检查")
	}
	root := viper.GetString("upload_directory")
	if root == "" {
		root = "frontend/public/upload-img"
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, errors.New("无法读取投稿图片目录")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, errors.New("投稿图片目录无效")
	}
	out := []ai.Image{}
	for i, raw := range names {
		path := strings.TrimPrefix(raw, "/")
		if !strings.HasPrefix(path, "upload-img/") {
			return nil, fmt.Errorf("附图 %d 不是本站可读取的图片，需人工检查", i+1)
		}
		name := strings.TrimPrefix(path, "upload-img/")
		if !attachmentName.MatchString(name) {
			return nil, fmt.Errorf("附图 %d 路径无效，需人工检查", i+1)
		}
		resolved, e := filepath.EvalSymlinks(filepath.Join(root, name))
		if e != nil {
			return nil, fmt.Errorf("附图 %d 文件缺失或不可读取", i+1)
		}
		rel, e := filepath.Rel(root, resolved)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, errors.New("附图不在本站上传目录")
		}
		f, e := os.Open(resolved)
		if e != nil {
			return nil, fmt.Errorf("附图 %d 读取失败", i+1)
		}
		b, e := io.ReadAll(io.LimitReader(f, (10<<20)+1))
		f.Close()
		if e != nil || len(b) > 10<<20 {
			return nil, fmt.Errorf("附图 %d 超出 10 MB 读取限制", i+1)
		}
		encoded, e := prepareImage(b)
		if e != nil {
			return nil, fmt.Errorf("附图 %d：%s", i+1, e.Error())
		}
		out = append(out, ai.Image{MIME: "image/jpeg", Data: encoded})
	}
	return out, nil
}
func animated(b []byte, format string) bool {
	switch format {
	case "gif":
		// Walk bounded GIF blocks instead of decoding every frame into memory.
		if len(b) < 13 {
			return true
		}
		p := 13
		if b[10]&0x80 != 0 {
			p += 3 * (1 << ((b[10] & 7) + 1))
		}
		frames := 0
		skipBlocks := func() bool {
			for p < len(b) {
				n := int(b[p])
				p++
				if n == 0 {
					return true
				}
				if n > len(b)-p {
					return false
				}
				p += n
			}
			return false
		}
		for p < len(b) {
			kind := b[p]
			p++
			switch kind {
			case 0x3b:
				return frames != 1
			case 0x21:
				if p >= len(b) {
					return true
				}
				p++
				if !skipBlocks() {
					return true
				}
			case 0x2c:
				frames++
				if frames > 1 || len(b)-p < 9 {
					return true
				}
				packed := b[p+8]
				p += 9
				if packed&0x80 != 0 {
					p += 3 * (1 << ((packed & 7) + 1))
				}
				if p >= len(b) {
					return true
				}
				p++
				if !skipBlocks() {
					return true
				}
			default:
				return true
			}
		}
		return true
	case "png":
		for p := 8; p+12 <= len(b); {
			n := int(binary.BigEndian.Uint32(b[p : p+4]))
			if n < 0 || n > len(b)-p-12 {
				return true
			}
			if string(b[p+4:p+8]) == "acTL" {
				return true
			}
			p += n + 12
		}
	case "webp":
		for p := 12; p+8 <= len(b); {
			n := int(binary.LittleEndian.Uint32(b[p+4 : p+8]))
			if n < 0 || n > len(b)-p-8 {
				return true
			}
			if string(b[p:p+4]) == "ANIM" {
				return true
			}
			p += 8 + n + (n % 2)
		}
	}
	return false
}
func prepareImage(b []byte) ([]byte, error) {
	cfg, format, e := image.DecodeConfig(bytes.NewReader(b))
	if e != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 16384 || cfg.Height > 16384 || int64(cfg.Width)*int64(cfg.Height) > 40000000 {
		return nil, errors.New("图片格式或尺寸异常，需人工检查")
	}
	if animated(b, format) {
		return nil, errors.New("动图暂不自动审核，需人工查看全部帧")
	}
	im, _, e := image.Decode(bytes.NewReader(b))
	if e != nil {
		return nil, errors.New("图片解码失败")
	}
	scale := math.Min(1, math.Min(2048/float64(max(cfg.Width, cfg.Height)), math.Sqrt(2000000/float64(cfg.Width*cfg.Height))))
	dst := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(cfg.Width)*scale)), max(1, int(float64(cfg.Height)*scale))))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), im, im.Bounds(), draw.Over, nil)
	var out bytes.Buffer
	if jpeg.Encode(&out, dst, &jpeg.Options{Quality: 88}) != nil || out.Len() > 2<<20 {
		return nil, errors.New("图片处理失败或过大，请人工检查")
	}
	return out.Bytes(), nil
}
