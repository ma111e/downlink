package store

import (
	"context"
	"fmt"
	"reflect"

	"github.com/klauspost/compress/zstd"
	"gorm.io/gorm/schema"
)

// Blob columns in this store are zstd-compressed. zstd beats gzip on both ratio
// and speed for the HTML/XML/JSON/markdown text we persist (raw feed bodies, LLM
// prompts/responses, analysis syntheses). The encoder/decoder are safe for
// concurrent use, so a single shared instance backs every call.
var (
	zstdEncoder *zstd.Encoder
	zstdDecoder *zstd.Decoder
)

func init() {
	var err error
	if zstdEncoder, err = zstd.NewWriter(nil); err != nil {
		panic(fmt.Sprintf("init zstd encoder: %v", err))
	}
	if zstdDecoder, err = zstd.NewReader(nil); err != nil {
		panic(fmt.Sprintf("init zstd decoder: %v", err))
	}
	schema.RegisterSerializer("zstd", ZstdSerializer{})
}

// compressString zstd-compresses s. Empty input yields nil so empty values stay
// cheap and round-trip back to "".
func compressString(s string) []byte {
	if s == "" {
		return nil
	}
	return zstdEncoder.EncodeAll([]byte(s), nil)
}

// decompressBytes reverses compressString. Empty input decodes to "".
func decompressBytes(b []byte) (string, error) {
	if len(b) == 0 {
		return "", nil
	}
	out, err := zstdDecoder.DecodeAll(b, nil)
	if err != nil {
		return "", fmt.Errorf("zstd decode: %w", err)
	}
	return string(out), nil
}

// ZstdSerializer transparently zstd-compresses string fields tagged
// `serializer:zstd`, so callers keep reading and writing plain Go strings while
// the column stores a compressed blob. Register once (init above); apply per
// field via the gorm tag.
type ZstdSerializer struct{}

// Value compresses the field value on write.
func (ZstdSerializer) Value(_ context.Context, _ *schema.Field, _ reflect.Value, fieldValue interface{}) (interface{}, error) {
	switch v := fieldValue.(type) {
	case string:
		return compressString(v), nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("zstd serializer: unsupported field type %T", fieldValue)
	}
}

// Scan decompresses the stored blob back into the string field on read.
func (ZstdSerializer) Scan(_ context.Context, field *schema.Field, dst reflect.Value, dbValue interface{}) error {
	var raw []byte
	switch v := dbValue.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	case nil:
		raw = nil
	default:
		return fmt.Errorf("zstd serializer: unsupported db type %T", dbValue)
	}

	s, err := decompressBytes(raw)
	if err != nil {
		return err
	}
	field.ReflectValueOf(context.Background(), dst).SetString(s)
	return nil
}
