package zstd

import (
	"bytes"
	"encoding/base64"
	"io"
	"math/rand"
	"regexp"
	"strings"
	"testing"
)

var dictBase64 string = `
	N6Qw7IsuFDIdENCSQjr//////4+QlekuNkmXbUBIkIDiVRX7H4AzAFCgQCFCO9oHAAAEQEuSikaK
	Dg51OYghBYgBAAAAAAAAAAAAAAAAAAAAANQVpmRQGQAAAAAAAAAAAAAAAAABAAAABAAAAAgAAABo
	ZWxwIEpvaW4gZW5naW5lZXJzIGVuZ2luZWVycyBmdXR1cmUgbG92ZSB0aGF0IGFyZWlsZGluZyB1
	c2UgaGVscCBoZWxwIHVzaGVyIEpvaW4gdXNlIGxvdmUgdXMgSm9pbiB1bmQgaW4gdXNoZXIgdXNo
	ZXIgYSBwbGF0Zm9ybSB1c2UgYW5kIGZ1dHVyZQ==`
var dict []byte
var compressedPayload []byte

func init() {
	var err error
	dict, err = base64.StdEncoding.DecodeString(regexp.MustCompile(`\s+`).ReplaceAllString(dictBase64, ""))
	if err != nil {
		panic("failed to create dictionary")
	}
	p, err := NewBulkProcessor(dict, BestSpeed)
	if err != nil {
		panic("failed to create bulk processor")
	}
	compressedPayload, err = p.Compress(nil, []byte("We're building a platform that engineers love to use. Join us, and help usher in the future."))
	if err != nil {
		panic("failed to compress payload")
	}
}

func newBulkProcessor(t testing.TB, dict []byte, level int) *BulkProcessor {
	p, err := NewBulkProcessor(dict, level)
	if err != nil {
		t.Fatal("failed to create a BulkProcessor")
	}
	return p
}

func getRandomText() string {
	words := []string{"We", "are", "building", "a platform", "that", "engineers", "love", "to", "use", "Join", "us", "and", "help", "usher", "in", "the", "future"}
	wordCount := 10 + rand.Intn(100) // 10 - 109
	result := []string{}
	for i := 0; i < wordCount; i++ {
		result = append(result, words[rand.Intn(len(words))])
	}

	return strings.Join(result, " ")
}

func TestBulkDictionary(t *testing.T) {
	if len(dict) < 1 {
		t.Error("dictionary is empty")
	}
}

func TestBulkCompressAndDecompress(t *testing.T) {
	p := newBulkProcessor(t, dict, BestSpeed)
	for i := 0; i < 100; i++ {
		payload := []byte(getRandomText())

		compressed, err := p.Compress(nil, payload)
		if err != nil {
			t.Error("failed to compress")
		}

		uncompressed, err := p.Decompress(nil, compressed)
		if err != nil {
			t.Error("failed to decompress")
		}

		if bytes.Compare(payload, uncompressed) != 0 {
			t.Error("uncompressed payload didn't match")
		}
	}
}

func TestBulkEmptyOrNilDictionary(t *testing.T) {
	p, err := NewBulkProcessor(nil, BestSpeed)
	if p != nil {
		t.Error("nil is expected")
	}
	if err != ErrEmptyDictionary {
		t.Error("ErrEmptyDictionary is expected")
	}

	p, err = NewBulkProcessor([]byte{}, BestSpeed)
	if p != nil {
		t.Error("nil is expected")
	}
	if err != ErrEmptyDictionary {
		t.Error("ErrEmptyDictionary is expected")
	}
}

func TestBulkCompressDecompressEmptyOrNilContent(t *testing.T) {
	p := newBulkProcessor(t, dict, BestSpeed)
	compressed, err := p.Compress(nil, nil)
	if err != nil {
		t.Error("failed to compress")
	}
	if len(compressed) < 4 {
		t.Error("magic number doesn't exist")
	}

	compressed, err = p.Compress(nil, []byte{})
	if err != nil {
		t.Error("failed to compress")
	}
	if len(compressed) < 4 {
		t.Error("magic number doesn't exist")
	}

	decompressed, err := p.Decompress(nil, compressed)
	if err != nil {
		t.Error("failed to decompress")
	}
	if len(decompressed) != 0 {
		t.Error("content was not decompressed correctly")
	}
}

func TestBulkCompressIntoGivenDestination(t *testing.T) {
	p := newBulkProcessor(t, dict, BestSpeed)
	dst := make([]byte, 100000)
	compressed, err := p.Compress(dst, []byte(getRandomText()))
	if err != nil {
		t.Error("failed to compress")
	}
	if len(compressed) < 4 {
		t.Error("magic number doesn't exist")
	}
	if &dst[0] != &compressed[0] {
		t.Error("'dst' and 'compressed' are not the same object")
	}
}

func TestBulkCompressNotEnoughDestination(t *testing.T) {
	p := newBulkProcessor(t, dict, BestSpeed)
	dst := make([]byte, 1)
	compressed, err := p.Compress(dst, []byte(getRandomText()))
	if err != nil {
		t.Error("failed to compress")
	}
	if len(compressed) < 4 {
		t.Error("magic number doesn't exist")
	}
	if &dst[0] == &compressed[0] {
		t.Error("'dst' and 'compressed' are the same object")
	}
}

func TestBulkDecompressIntoGivenDestination(t *testing.T) {
	p := newBulkProcessor(t, dict, BestSpeed)
	dst := make([]byte, 100000)
	decompressed, err := p.Decompress(dst, compressedPayload)
	if err != nil {
		t.Error("failed to decompress")
	}
	if &dst[0] != &decompressed[0] {
		t.Error("'dst' and 'decompressed' are not the same object")
	}
}

func TestBulkDecompressNotEnoughDestination(t *testing.T) {
	p := newBulkProcessor(t, dict, BestSpeed)
	dst := make([]byte, 1)
	decompressed, err := p.Decompress(dst, compressedPayload)
	if err != nil {
		t.Error("failed to decompress")
	}
	if &dst[0] == &decompressed[0] {
		t.Error("'dst' and 'decompressed' are the same object")
	}
}

func TestBulkDecompressEmptyOrNilContent(t *testing.T) {
	p := newBulkProcessor(t, dict, BestSpeed)
	decompressed, err := p.Decompress(nil, nil)
	if err != ErrEmptySlice {
		t.Error("ErrEmptySlice is expected")
	}
	if decompressed != nil {
		t.Error("nil is expected")
	}

	decompressed, err = p.Decompress(nil, []byte{})
	if err != ErrEmptySlice {
		t.Error("ErrEmptySlice is expected")
	}
	if decompressed != nil {
		t.Error("nil is expected")
	}
}

func TestBulkCompressAndDecompressInReverseOrder(t *testing.T) {
	p := newBulkProcessor(t, dict, BestSpeed)
	payloads := [][]byte{}
	compressedPayloads := [][]byte{}
	for i := 0; i < 100; i++ {
		payloads = append(payloads, []byte(getRandomText()))

		compressed, err := p.Compress(nil, payloads[i])
		if err != nil {
			t.Error("failed to compress")
		}
		compressedPayloads = append(compressedPayloads, compressed)
	}

	for i := 99; i >= 0; i-- {
		uncompressed, err := p.Decompress(nil, compressedPayloads[i])
		if err != nil {
			t.Error("failed to decompress")
		}

		if bytes.Compare(payloads[i], uncompressed) != 0 {
			t.Error("uncompressed payload didn't match")
		}
	}
}

func TestBulkDecompressHighlyCompressable(t *testing.T) {
	p := newBulkProcessor(t, dict, BestSpeed)

	// Generate a big payload
	msgSize := 10 * 1000 * 1000 // 10 MiB
	msg := make([]byte, msgSize)
	compressed, err := Compress(nil, msg)
	if err != nil {
		t.Error("failed to compress")
	}

	// Regular decompression would trigger zipbomb prevention
	_, err = p.Decompress(nil, compressed)
	if !IsDstSizeTooSmallError(err) {
		t.Error("expected too small error")
	}

	// Passing an output should suceed the decompression
	dst := make([]byte, 10*msgSize)
	_, err = p.Decompress(dst, compressed)
	if err != nil {
		t.Errorf("failed to decompress: %s", err)
	}
}

// BenchmarkBulkCompress-8   	  780148	      1505 ns/op	  61.14 MB/s	     208 B/op	       5 allocs/op
func BenchmarkBulkCompress(b *testing.B) {
	p := newBulkProcessor(b, dict, BestSpeed)

	payload := []byte("We're building a platform that engineers love to use. Join us, and help usher in the future.")
	b.SetBytes(int64(len(payload)))
	for n := 0; n < b.N; n++ {
		_, err := p.Compress(nil, payload)
		if err != nil {
			b.Error("failed to compress")
		}
	}
}

// BenchmarkBulkDecompress-8   	  817425	      1412 ns/op	  40.37 MB/s	     192 B/op	       7 allocs/op
func BenchmarkBulkDecompress(b *testing.B) {
	p := newBulkProcessor(b, dict, BestSpeed)

	b.SetBytes(int64(len(compressedPayload)))
	for n := 0; n < b.N; n++ {
		_, err := p.Decompress(nil, compressedPayload)
		if err != nil {
			b.Error("failed to decompress")
		}
	}
}

// --- TDD: BulkProcessor Stream API Tests ---

func TestBulkStreamWriterSimple(t *testing.T) {
	p := newBulkProcessor(t, dict, BestSpeed)
	payload := []byte("We're building a platform that engineers love to use.")

	var buf bytes.Buffer
	w := p.NewWriter(&buf)

	n, err := w.Write(payload)
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("expected to write %d bytes, wrote %d", len(payload), n)
	}

	err = w.Close()
	if err != nil {
		t.Fatalf("failed to close: %v", err)
	}

	// Decompress using the same BulkProcessor
	decompressed, err := p.Decompress(nil, buf.Bytes())
	if err != nil {
		t.Fatalf("failed to decompress: %v", err)
	}

	if !bytes.Equal(payload, decompressed) {
		t.Fatalf("payload mismatch: expected %q, got %q", payload, decompressed)
	}
}

func TestBulkStreamWriterEmpty(t *testing.T) {
	p := newBulkProcessor(t, dict, BestSpeed)

	var buf bytes.Buffer
	w := p.NewWriter(&buf)

	n, err := w.Write([]byte{})
	if err != nil {
		t.Fatalf("failed to write empty: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected to write 0 bytes, wrote %d", n)
	}

	err = w.Close()
	if err != nil {
		t.Fatalf("failed to close: %v", err)
	}

	// Should produce valid zstd frame
	if len(buf.Bytes()) == 0 {
		t.Fatal("expected non-empty output for empty input")
	}
}

func TestBulkStreamWriterFlush(t *testing.T) {
	p := newBulkProcessor(t, dict, BestSpeed)
	payload := []byte("test flush")

	var buf bytes.Buffer
	w := p.NewWriter(&buf)

	_, err := w.Write(payload)
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	err = w.Flush()
	if err != nil {
		t.Fatalf("failed to flush: %v", err)
	}

	// Data should be available after flush
	if buf.Len() == 0 {
		t.Fatal("expected data after flush")
	}

	err = w.Close()
	if err != nil {
		t.Fatalf("failed to close: %v", err)
	}
}

func TestBulkStreamReaderSimple(t *testing.T) {
	p := newBulkProcessor(t, dict, BestSpeed)
	payload := []byte("We're building a platform that engineers love to use.")

	// Compress using BulkProcessor
	compressed, err := p.Compress(nil, payload)
	if err != nil {
		t.Fatalf("failed to compress: %v", err)
	}

	// Decompress using BulkReader
	r := p.NewReader(bytes.NewReader(compressed))
	decompressed, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}

	err = r.Close()
	if err != nil {
		t.Fatalf("failed to close: %v", err)
	}

	if !bytes.Equal(payload, decompressed) {
		t.Fatalf("payload mismatch: expected %q, got %q", payload, decompressed)
	}
}

func TestBulkStreamReaderEmpty(t *testing.T) {
	p := newBulkProcessor(t, dict, BestSpeed)

	// Compress empty data
	compressed, err := p.Compress(nil, []byte{})
	if err != nil {
		t.Fatalf("failed to compress: %v", err)
	}

	// Decompress using BulkReader
	r := p.NewReader(bytes.NewReader(compressed))
	decompressed, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}

	err = r.Close()
	if err != nil {
		t.Fatalf("failed to close: %v", err)
	}

	if len(decompressed) != 0 {
		t.Fatalf("expected empty output, got %d bytes", len(decompressed))
	}
}

func TestBulkStreamWriterReaderRoundtrip(t *testing.T) {
	p := newBulkProcessor(t, dict, BestSpeed)

	for i := 0; i < 100; i++ {
		payload := []byte(getRandomText())

		// Compress using BulkWriter
		var buf bytes.Buffer
		w := p.NewWriter(&buf)
		_, err := w.Write(payload)
		if err != nil {
			t.Fatalf("failed to write: %v", err)
		}
		err = w.Close()
		if err != nil {
			t.Fatalf("failed to close writer: %v", err)
		}

		// Decompress using BulkReader
		r := p.NewReader(&buf)
		decompressed, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("failed to read: %v", err)
		}
		err = r.Close()
		if err != nil {
			t.Fatalf("failed to close reader: %v", err)
		}

		if !bytes.Equal(payload, decompressed) {
			t.Fatalf("roundtrip failed: payload mismatch")
		}
	}
}

func TestBulkDecompressWriterSimple(t *testing.T) {
	p := newBulkProcessor(t, dict, BestSpeed)
	payload := []byte("We're building a platform that engineers love to use.")

	// Compress using BulkProcessor
	compressed, err := p.Compress(nil, payload)
	if err != nil {
		t.Fatalf("failed to compress: %v", err)
	}

	// Decompress using BulkDecompressWriter
	var buf bytes.Buffer
	w := p.NewDecompressWriter(&buf)

	n, err := w.Write(compressed)
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}
	if n != len(compressed) {
		t.Fatalf("expected to write %d bytes, wrote %d", len(compressed), n)
	}

	err = w.Close()
	if err != nil {
		t.Fatalf("failed to close: %v", err)
	}

	if !bytes.Equal(payload, buf.Bytes()) {
		t.Fatalf("payload mismatch: expected %q, got %q", payload, buf.Bytes())
	}
}

func TestBulkDecompressWriterEmpty(t *testing.T) {
	p := newBulkProcessor(t, dict, BestSpeed)

	// Compress empty data
	compressed, err := p.Compress(nil, []byte{})
	if err != nil {
		t.Fatalf("failed to compress: %v", err)
	}

	// Decompress using BulkDecompressWriter
	var buf bytes.Buffer
	w := p.NewDecompressWriter(&buf)

	_, err = w.Write(compressed)
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	err = w.Close()
	if err != nil {
		t.Fatalf("failed to close: %v", err)
	}

	if len(buf.Bytes()) != 0 {
		t.Fatalf("expected empty output, got %d bytes", len(buf.Bytes()))
	}
}

func TestBulkDecompressWriterChunks(t *testing.T) {
	p := newBulkProcessor(t, dict, BestSpeed)
	payload := []byte("We're building a platform that engineers love to use. Join us!")

	// Compress using BulkProcessor
	compressed, err := p.Compress(nil, payload)
	if err != nil {
		t.Fatalf("failed to compress: %v", err)
	}

	// Split compressed data into chunks
	chunkSize := 4
	var chunks [][]byte
	for i := 0; i < len(compressed); i += chunkSize {
		end := i + chunkSize
		if end > len(compressed) {
			end = len(compressed)
		}
		chunks = append(chunks, compressed[i:end])
	}

	// Decompress using BulkDecompressWriter with chunks
	var buf bytes.Buffer
	w := p.NewDecompressWriter(&buf)

	for _, chunk := range chunks {
		_, err := w.Write(chunk)
		if err != nil {
			t.Fatalf("failed to write chunk: %v", err)
		}
	}

	err = w.Close()
	if err != nil {
		t.Fatalf("failed to close: %v", err)
	}

	if !bytes.Equal(payload, buf.Bytes()) {
		t.Fatalf("payload mismatch: expected %q, got %q", payload, buf.Bytes())
	}
}

func TestBulkStreamWriterDecompressWriterRoundtrip(t *testing.T) {
	p := newBulkProcessor(t, dict, BestSpeed)

	for i := 0; i < 100; i++ {
		payload := []byte(getRandomText())

		// Compress using BulkWriter
		var compBuf bytes.Buffer
		w := p.NewWriter(&compBuf)
		_, err := w.Write(payload)
		if err != nil {
			t.Fatalf("failed to write: %v", err)
		}
		err = w.Close()
		if err != nil {
			t.Fatalf("failed to close writer: %v", err)
		}

		// Decompress using BulkDecompressWriter
		var decompBuf bytes.Buffer
		dw := p.NewDecompressWriter(&decompBuf)
		_, err = dw.Write(compBuf.Bytes())
		if err != nil {
			t.Fatalf("failed to decompress write: %v", err)
		}
		err = dw.Close()
		if err != nil {
			t.Fatalf("failed to close decompress writer: %v", err)
		}

		if !bytes.Equal(payload, decompBuf.Bytes()) {
			t.Fatalf("roundtrip failed: payload mismatch")
		}
	}
}
