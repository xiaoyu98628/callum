package imageinput

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"testing"
)

func TestImagePreparationRespectsPatchAndPixelBudget(t *testing.T) {
	var original bytes.Buffer
	if err := png.Encode(&original, image.NewRGBA(image.Rect(0, 0, 1800, 1800))); err != nil {
		t.Fatal(err)
	}
	prepared, err := Prepare(t.Context(), original.Bytes(), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(prepared))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != 1600 || config.Height != 1600 || len(prepared) > MaxBytes {
		t.Fatalf("图片未按 patch 预算缩放: %dx%d %d bytes", config.Width, config.Height, len(prepared))
	}
}

func TestImagePreparationAcceptsLargeEncodedSourceWithinPixelBudget(t *testing.T) {
	var original bytes.Buffer
	if err := png.Encode(&original, image.NewRGBA(image.Rect(0, 0, 12, 9))); err != nil {
		t.Fatal(err)
	}
	// 合法的 PNG 文本块使原文件超过发送边界，像素尺寸仍然很小。
	chunk := make([]byte, 12+(5<<20))
	binary.BigEndian.PutUint32(chunk, uint32(len(chunk)-12))
	copy(chunk[4:], "tEXt")
	copy(chunk[8:], "Description\x00")
	binary.BigEndian.PutUint32(chunk[len(chunk)-4:], crc32.ChecksumIEEE(chunk[4:len(chunk)-4]))
	data := append(append(append([]byte(nil), original.Bytes()[:33]...), chunk...), original.Bytes()[33:]...)
	prepared, err := Prepare(t.Context(), data, "image/png")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(prepared))
	if err != nil || cfg.Width != 12 || cfg.Height != 9 || len(prepared) > MaxBytes {
		t.Fatalf("小尺寸原图重编码不正确: %+v %v", cfg, err)
	}
}
