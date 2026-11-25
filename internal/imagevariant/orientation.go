package imagevariant

import (
	"encoding/binary"
	"image"
	"image/color"
)

// Read the small TIFF orientation tag in JPEG EXIF; malformed metadata is ignored.
func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return 1
	}
	for offset := 2; offset+4 <= len(data); {
		if data[offset] != 0xff {
			break
		}
		marker := data[offset+1]
		if marker == 0xda || marker == 0xd9 {
			break
		}
		size := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		if size < 2 || offset+2+size > len(data) {
			break
		}
		segment := data[offset+4 : offset+2+size]
		if marker == 0xe1 && len(segment) >= 14 && string(segment[:6]) == "Exif\x00\x00" {
			tiff := segment[6:]
			var order binary.ByteOrder
			switch string(tiff[:2]) {
			case "II":
				order = binary.LittleEndian
			case "MM":
				order = binary.BigEndian
			default:
				return 1
			}
			if order.Uint16(tiff[2:4]) != 42 {
				return 1
			}
			start := uint64(order.Uint32(tiff[4:8]))
			if start+2 > uint64(len(tiff)) {
				return 1
			}
			count := uint64(order.Uint16(tiff[start : start+2]))
			for i := uint64(0); i < count; i++ {
				entry := start + 2 + i*12
				if entry+12 > uint64(len(tiff)) {
					return 1
				}
				field := tiff[entry : entry+12]
				if order.Uint16(field[:2]) == 0x112 && order.Uint16(field[2:4]) == 3 && order.Uint32(field[4:8]) == 1 {
					value := int(order.Uint16(field[8:10]))
					if value >= 1 && value <= 8 {
						return value
					}
				}
			}
		}
		offset += size + 2
	}
	return 1
}

type orientedImage struct {
	image.Image
	orientation int
}

func (im orientedImage) Bounds() image.Rectangle {
	b := im.Image.Bounds()
	if im.orientation >= 5 {
		return image.Rect(0, 0, b.Dy(), b.Dx())
	}
	return image.Rect(0, 0, b.Dx(), b.Dy())
}

func (im orientedImage) At(x, y int) color.Color {
	b := im.Image.Bounds()
	w, h := b.Dx(), b.Dy()
	switch im.orientation {
	case 2:
		x = w - 1 - x
	case 3:
		x, y = w-1-x, h-1-y
	case 4:
		y = h - 1 - y
	case 5:
		x, y = y, x
	case 6:
		x, y = y, h-1-x
	case 7:
		x, y = w-1-y, h-1-x
	case 8:
		x, y = w-1-y, x
	}
	return im.Image.At(b.Min.X+x, b.Min.Y+y)
}
