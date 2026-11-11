package chatcompletions

import (
	"context"
	"encoding/base64"

	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/imageinput"
)

// MaxInputBytes 限制包含内联图片的实际请求 JSON。
const MaxInputBytes = 12 << 20

// 按已校验的引用缓存准备结果。适配器实例归属单次执行。
func (c *Client) imageData(ctx context.Context, block model.Block, used map[string]string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var data []byte
	var mime string
	var err error
	key := block.AttachmentRef
	if key != "" {
		if c.readAttachment == nil {
			return "", model.ErrImageRead
		}
		if encoded, ok := used[key]; ok {
			return encoded, nil
		}
		if encoded, ok := c.images[key]; ok {
			used[key] = encoded
			return encoded, nil
		}
		data, mime, err = c.readAttachment(ctx, key)
	} else {
		data, mime, err = imageinput.Read(ctx, block.Path)
	}
	if err != nil {
		return "", err
	}
	data, err = imageinput.Prepare(ctx, data, mime)
	if err != nil {
		return "", err
	}
	encoded := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
	if key != "" {
		used[key] = encoded
	}
	return encoded, nil
}
