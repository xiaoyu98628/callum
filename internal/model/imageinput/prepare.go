package imageinput

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	"image/png"

	"github.com/xiaoyu98628/callum/internal/model"
)

const (
	maxSourcePixels    = 16_000_000
	maxSourceDimension = 16_384
	maxDimension       = 2048
	patchSize          = 32
	maxPatches         = 2500
)

// Prepare 在本地约束内保留原编码，或等比缩小 PNG/JPEG 的发送版本。
// GIF/WebP 保留原始帧及编码，超过发送边界时要求用户转换。
func Prepare(ctx context.Context, data []byte, mime string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	width, height, err := dimensions(data, mime)
	if err != nil {
		return nil, err
	}
	if width <= 0 || height <= 0 || width > maxSourceDimension || height > maxSourceDimension || int64(width) > maxSourcePixels/int64(height) {
		return nil, model.ErrImageLimit
	}
	w, h := targetSize(width, height)
	if mime == "image/webp" {
		if w != width || h != height || len(data) > MaxBytes {
			return nil, model.ErrImageLimit
		}
		return data, nil
	}
	// Decode 也验证完整的静态图像。GIF 只解码首帧来校验并保留原始编码。
	source, _, err := image.Decode(contextReader{ctx, bytes.NewReader(data)})
	if err != nil {
		return nil, errors.Join(model.ErrImageFormat, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if w == width && h == height && len(data) <= MaxBytes {
		return data, nil
	}
	if mime == "image/gif" {
		return nil, model.ErrImageLimit
	}
	for {
		scaled, err := scale(ctx, source, w, h)
		if err != nil {
			return nil, err
		}
		var buffer bytes.Buffer
		writer := contextWriter{ctx, &buffer}
		if mime == "image/jpeg" {
			err = jpeg.Encode(writer, scaled, &jpeg.Options{Quality: 90})
		} else {
			err = png.Encode(writer, scaled)
		}
		if err != nil {
			return nil, err
		}
		if buffer.Len() <= MaxBytes {
			return buffer.Bytes(), ctx.Err()
		}
		w, h = max(1, w*3/4), max(1, h*3/4)
	}
}

type contextWriter struct {
	ctx    context.Context
	buffer *bytes.Buffer
}

func (w contextWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.buffer.Write(p)
}

func targetSize(width, height int) (int, int) {
	long := max(width, height)
	lo, hi := 1, min(long, maxDimension)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		w, h := max(1, int(int64(width)*int64(mid)/int64(long))), max(1, int(int64(height)*int64(mid)/int64(long)))
		if ((w+patchSize-1)/patchSize)*((h+patchSize-1)/patchSize) <= maxPatches {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return max(1, int(int64(width)*int64(lo)/int64(long))), max(1, int(int64(height)*int64(lo)/int64(long)))
}

// 每个目标像素取对应源区域的平均值，缩小截图时保留细线信息。
func scale(ctx context.Context, source image.Image, width, height int) (*image.RGBA, error) {
	bounds := source.Bounds()
	target := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		y0, y1 := y*bounds.Dy()/height, (y+1)*bounds.Dy()/height
		y1 = min(bounds.Dy(), max(y0+1, y1))
		for x := range width {
			x0, x1 := x*bounds.Dx()/width, (x+1)*bounds.Dx()/width
			x1 = min(bounds.Dx(), max(x0+1, x1))
			var r, g, b, a uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					pr, pg, pb, pa := source.At(bounds.Min.X+sx, bounds.Min.Y+sy).RGBA()
					r, g, b, a = r+uint64(pr), g+uint64(pg), b+uint64(pb), a+uint64(pa)
				}
			}
			n := uint64((x1-x0)*(y1-y0)) * 257
			target.SetRGBA(x, y, color.RGBA{R: uint8(r / n), G: uint8(g / n), B: uint8(b / n), A: uint8(a / n)})
		}
	}
	return target, nil
}

func dimensions(data []byte, mime string) (int, int, error) {
	if mime != "image/webp" {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return 0, 0, errors.Join(model.ErrImageFormat, err)
		}
		return cfg.Width, cfg.Height, nil
	}
	// WebP 的首个块记录静态位流尺寸或扩展画布尺寸。
	if len(data) < 20 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" ||
		uint64(binary.LittleEndian.Uint32(data[4:8]))+8 != uint64(len(data)) {
		return 0, 0, model.ErrImageFormat
	}
	length := binary.LittleEndian.Uint32(data[16:20])
	if uint64(length)+20 > uint64(len(data)) {
		return 0, 0, model.ErrImageFormat
	}
	b := data[20 : 20+int(length)]
	switch string(data[12:16]) {
	case "VP8X":
		if len(b) == 10 {
			return 1 + int(b[4]) + int(b[5])<<8 + int(b[6])<<16, 1 + int(b[7]) + int(b[8])<<8 + int(b[9])<<16, nil
		}
	case "VP8 ":
		if len(b) >= 10 && bytes.Equal(b[3:6], []byte{0x9d, 0x01, 0x2a}) {
			return int(binary.LittleEndian.Uint16(b[6:8]) & 0x3fff), int(binary.LittleEndian.Uint16(b[8:10]) & 0x3fff), nil
		}
	case "VP8L":
		if len(b) >= 5 && b[0] == 0x2f {
			bits := binary.LittleEndian.Uint32(b[1:5])
			return 1 + int(bits&0x3fff), 1 + int((bits>>14)&0x3fff), nil
		}
	}
	return 0, 0, model.ErrImageFormat
}
