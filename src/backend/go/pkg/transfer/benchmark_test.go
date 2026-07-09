package transfer

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/klauspost/compress/zstd"
)

var zstdEncoder *zstd.Encoder
var zstdDecoder *zstd.Decoder

func init() {
	var err error
	zstdEncoder, err = zstd.NewWriter(nil)
	if err != nil {
		panic(err)
	}
	zstdDecoder, err = zstd.NewReader(nil)
	if err != nil {
		panic(err)
	}
}

const CompressionThreshold = 1024 * 1024 // 1MB

func compressData(data []byte) ([]byte, error) {
	return zstdEncoder.EncodeAll(data, nil), nil
}

func decompressData(data []byte) ([]byte, error) {
	return zstdDecoder.DecodeAll(data, nil)
}

func BenchmarkCompressData(b *testing.B) {
	data := make([]byte, 10*1024*1024) // 10MB
	_, _ = rand.Read(data)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		compressed, err := compressData(data)
		if err != nil {
			b.Fatal(err)
		}
		_, err = decompressData(compressed)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompressSmallData(b *testing.B) {
	data := make([]byte, 100*1024) // 100KB
	_, _ = rand.Read(data)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		compressed, err := compressData(data)
		if err != nil {
			b.Fatal(err)
		}
		_, err = decompressData(compressed)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkInlineTransfer(b *testing.B) {
	data := make([]byte, 512*1024) // 512KB
	_, _ = rand.Read(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		compressed := zstdEncoder.EncodeAll(data, nil)
		if len(compressed) == 0 {
			b.Fatal("compression produced empty output")
		}
		decoded, err := zstdDecoder.DecodeAll(compressed, nil)
		if err != nil {
			b.Fatal(err)
		}
		if !bytes.Equal(decoded, data) {
			b.Fatal("roundtrip mismatch")
		}
	}
}

func BenchmarkCompressThreshold(b *testing.B) {
	sizes := []int64{500 * 1024, 2 * 1024 * 1024, 10 * 1024 * 1024}
	for _, size := range sizes {
		b.Run("size="+humanSize(size), func(b *testing.B) {
			data := make([]byte, size)
			_, _ = rand.Read(data)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if size > CompressionThreshold {
					compressed := zstdEncoder.EncodeAll(data, nil)
					if len(compressed) == 0 {
						b.Fatal("empty compression")
					}
				}
			}
		})
	}
}

func humanSize(size int64) string {
	if size < 1024 {
		return "B"
	}
	if size < 1024*1024 {
		return "KB"
	}
	return "MB"
}
