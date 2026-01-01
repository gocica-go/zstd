package zstd

/*
#include "zstd.h"

typedef struct bulkCompressStream_result_s {
	size_t return_code;
	size_t bytes_consumed;
	size_t bytes_written;
} bulkCompressStream_result;

static void ZSTD_bulkCompressStream_wrapper(bulkCompressStream_result* result, ZSTD_CCtx* ctx,
		void* dst, size_t maxDstSize, const void* src, size_t srcSize) {
	ZSTD_outBuffer outBuffer = { dst, maxDstSize, 0 };
	ZSTD_inBuffer inBuffer = { src, srcSize, 0 };
	size_t retCode = ZSTD_compressStream2(ctx, &outBuffer, &inBuffer, ZSTD_e_continue);

	result->return_code = retCode;
	result->bytes_consumed = inBuffer.pos;
	result->bytes_written = outBuffer.pos;
}

static void ZSTD_bulkCompressStream_flush(bulkCompressStream_result* result, ZSTD_CCtx* ctx,
		void* dst, size_t maxDstSize, const void* src, size_t srcSize) {
	ZSTD_outBuffer outBuffer = { dst, maxDstSize, 0 };
	ZSTD_inBuffer inBuffer = { src, srcSize, 0 };
	size_t retCode = ZSTD_compressStream2(ctx, &outBuffer, &inBuffer, ZSTD_e_flush);

	result->return_code = retCode;
	result->bytes_consumed = inBuffer.pos;
	result->bytes_written = outBuffer.pos;
}

static void ZSTD_bulkCompressStream_finish(bulkCompressStream_result* result, ZSTD_CCtx* ctx,
		void* dst, size_t maxDstSize, const void* src, size_t srcSize) {
	ZSTD_outBuffer outBuffer = { dst, maxDstSize, 0 };
	ZSTD_inBuffer inBuffer = { src, srcSize, 0 };
	size_t retCode = ZSTD_compressStream2(ctx, &outBuffer, &inBuffer, ZSTD_e_end);

	result->return_code = retCode;
	result->bytes_consumed = inBuffer.pos;
	result->bytes_written = outBuffer.pos;
}

typedef struct bulkDecompressStream_result_s {
	size_t return_code;
	size_t bytes_consumed;
	size_t bytes_written;
} bulkDecompressStream_result;

static void ZSTD_bulkDecompressStream_wrapper(bulkDecompressStream_result* result, ZSTD_DCtx* ctx,
		void* dst, size_t maxDstSize, const void* src, size_t srcSize) {
	ZSTD_outBuffer outBuffer = { dst, maxDstSize, 0 };
	ZSTD_inBuffer inBuffer = { src, srcSize, 0 };
	size_t retCode = ZSTD_decompressStream(ctx, &outBuffer, &inBuffer);

	result->return_code = retCode;
	result->bytes_consumed = inBuffer.pos;
	result->bytes_written = outBuffer.pos;
}
*/
import "C"
import (
	"errors"
	"io"
	"runtime"
	"unsafe"
)

var (
	// ErrEmptyDictionary is returned when the given dictionary is empty
	ErrEmptyDictionary = errors.New("Dictionary is empty")
	// ErrBadDictionary is returned when cannot load the given dictionary
	ErrBadDictionary = errors.New("Cannot load dictionary")
)

// BulkProcessor implements Bulk processing dictionary API.
// When compressing multiple messages or blocks using the same dictionary,
// it's recommended to digest the dictionary only once, since it's a costly operation.
// NewBulkProcessor() will create a state from digesting a dictionary.
// The resulting state can be used for future compression/decompression operations with very limited startup cost.
// BulkProcessor can be created once and shared by multiple threads concurrently, since its usage is read-only.
// The state will be freed when gc cleans up BulkProcessor.
type BulkProcessor struct {
	cDict *C.struct_ZSTD_CDict_s
	dDict *C.struct_ZSTD_DDict_s
}

// NewBulkProcessor creates a new BulkProcessor with a pre-trained dictionary and compression level
func NewBulkProcessor(dictionary []byte, compressionLevel int) (*BulkProcessor, error) {
	if len(dictionary) < 1 {
		return nil, ErrEmptyDictionary
	}

	p := &BulkProcessor{}
	runtime.SetFinalizer(p, finalizeBulkProcessor)

	p.cDict = C.ZSTD_createCDict(
		unsafe.Pointer(&dictionary[0]),
		C.size_t(len(dictionary)),
		C.int(compressionLevel),
	)
	if p.cDict == nil {
		return nil, ErrBadDictionary
	}
	p.dDict = C.ZSTD_createDDict(
		unsafe.Pointer(&dictionary[0]),
		C.size_t(len(dictionary)),
	)
	if p.dDict == nil {
		return nil, ErrBadDictionary
	}

	return p, nil
}

// Compress compresses `src` into `dst` with the dictionary given when creating the BulkProcessor.
// If you have a buffer to use, you can pass it to prevent allocation.
// If it is too small, or if nil is passed, a new buffer will be allocated and returned.
func (p *BulkProcessor) Compress(dst, src []byte) ([]byte, error) {
	bound := CompressBound(len(src))
	if cap(dst) >= bound {
		dst = dst[0:bound]
	} else {
		dst = make([]byte, bound)
	}

	cctx := C.ZSTD_createCCtx()
	// We need unsafe.Pointer(&src[0]) in the Cgo call to avoid "Go pointer to Go pointer" panics.
	// This means we need to special case empty input. See:
	// https://github.com/golang/go/issues/14210#issuecomment-346402945
	var cWritten C.size_t
	if len(src) == 0 {
		cWritten = C.ZSTD_compress_usingCDict(
			cctx,
			unsafe.Pointer(&dst[0]),
			C.size_t(len(dst)),
			unsafe.Pointer(nil),
			C.size_t(len(src)),
			p.cDict,
		)
	} else {
		cWritten = C.ZSTD_compress_usingCDict(
			cctx,
			unsafe.Pointer(&dst[0]),
			C.size_t(len(dst)),
			unsafe.Pointer(&src[0]),
			C.size_t(len(src)),
			p.cDict,
		)
	}

	C.ZSTD_freeCCtx(cctx)

	written := int(cWritten)
	if err := getError(written); err != nil {
		return nil, err
	}
	return dst[:written], nil
}

// Decompress decompresses `src` into `dst` with the dictionary given when creating the BulkProcessor.
// If you have a buffer to use, you can pass it to prevent allocation.
// If it is too small, or if nil is passed, a new buffer will be allocated and returned.
func (p *BulkProcessor) Decompress(dst, src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, ErrEmptySlice
	}

	contentSize := decompressSizeHint(src)
	if cap(dst) >= contentSize {
		dst = dst[0:cap(dst)]
	} else {
		dst = make([]byte, contentSize)
	}

	if len(dst) == 0 {
		return dst, nil
	}

	dctx := C.ZSTD_createDCtx()
	cWritten := C.ZSTD_decompress_usingDDict(
		dctx,
		unsafe.Pointer(&dst[0]),
		C.size_t(len(dst)),
		unsafe.Pointer(&src[0]),
		C.size_t(len(src)),
		p.dDict,
	)
	C.ZSTD_freeDCtx(dctx)

	written := int(cWritten)
	if err := getError(written); err != nil {
		return nil, err
	}

	return dst[:written], nil
}

// finalizeBulkProcessor frees compression and decompression dictionaries from memory
func finalizeBulkProcessor(p *BulkProcessor) {
	if p.cDict != nil {
		C.ZSTD_freeCDict(p.cDict)
	}
	if p.dDict != nil {
		C.ZSTD_freeDDict(p.dDict)
	}
}

// BulkWriter is an io.WriteCloser that zstd-compresses using a pre-digested dictionary.
type BulkWriter struct {
	ctx              *C.ZSTD_CCtx
	cDict            *C.struct_ZSTD_CDict_s
	dstBuffer        []byte
	firstError       error
	underlyingWriter io.Writer
	resultBuffer     *C.bulkCompressStream_result
}

// NewWriter creates a new streaming compressor using the BulkProcessor's dictionary.
func (p *BulkProcessor) NewWriter(w io.Writer) *BulkWriter {
	ctx := C.ZSTD_createCCtx()

	// Reference the pre-digested dictionary
	err := getError(int(C.ZSTD_CCtx_refCDict(ctx, p.cDict)))

	return &BulkWriter{
		ctx:              ctx,
		cDict:            p.cDict,
		dstBuffer:        make([]byte, CompressBound(1024)),
		firstError:       err,
		underlyingWriter: w,
		resultBuffer:     new(C.bulkCompressStream_result),
	}
}

// Write compresses p and writes to the underlying writer.
func (w *BulkWriter) Write(p []byte) (int, error) {
	if w.firstError != nil {
		return 0, w.firstError
	}
	if len(p) == 0 {
		return 0, nil
	}

	total := len(p)
	w.dstBuffer = w.dstBuffer[0:cap(w.dstBuffer)]
	if len(w.dstBuffer) < CompressBound(len(p)) {
		w.dstBuffer = make([]byte, CompressBound(len(p)))
	}

	dstoff := 0
	consumed := 0
	for len(p) > 0 {
		C.ZSTD_bulkCompressStream_wrapper(
			w.resultBuffer,
			w.ctx,
			unsafe.Pointer(&w.dstBuffer[dstoff]),
			C.size_t(len(w.dstBuffer[dstoff:])),
			unsafe.Pointer(&p[0]),
			C.size_t(len(p)),
		)
		ret := int(w.resultBuffer.return_code)
		if err := getError(ret); err != nil {
			w.firstError = err
			return 0, err
		}
		p = p[w.resultBuffer.bytes_consumed:]
		dstoff += int(w.resultBuffer.bytes_written)
		consumed += int(w.resultBuffer.bytes_consumed)
		if len(p) > 0 && dstoff == len(w.dstBuffer) {
			newbuf := make([]byte, len(w.dstBuffer)+CompressBound(total-consumed))
			copy(newbuf, w.dstBuffer)
			w.dstBuffer = newbuf
		}
	}

	_, err := w.underlyingWriter.Write(w.dstBuffer[:dstoff])
	if err != nil {
		return 0, err
	}
	return total, nil
}

// Flush writes any buffered data to the underlying writer.
func (w *BulkWriter) Flush() error {
	if w.firstError != nil {
		return w.firstError
	}

	ret := 1
	for ret > 0 {
		C.ZSTD_bulkCompressStream_flush(
			w.resultBuffer,
			w.ctx,
			unsafe.Pointer(&w.dstBuffer[0]),
			C.size_t(len(w.dstBuffer)),
			unsafe.Pointer(uintptr(0)),
			C.size_t(0),
		)
		ret = int(w.resultBuffer.return_code)
		if err := getError(ret); err != nil {
			return err
		}
		written := int(w.resultBuffer.bytes_written)
		_, err := w.underlyingWriter.Write(w.dstBuffer[:written])
		if err != nil {
			return err
		}

		if ret > 0 {
			w.dstBuffer = w.dstBuffer[:cap(w.dstBuffer)]
			if len(w.dstBuffer) < ret {
				w.dstBuffer = make([]byte, ret)
			}
		}
	}

	return nil
}

// Close flushes remaining data and frees resources.
func (w *BulkWriter) Close() error {
	if w.firstError != nil {
		return w.firstError
	}

	ret := 1
	for ret > 0 {
		C.ZSTD_bulkCompressStream_finish(
			w.resultBuffer,
			w.ctx,
			unsafe.Pointer(&w.dstBuffer[0]),
			C.size_t(len(w.dstBuffer)),
			unsafe.Pointer(uintptr(0)),
			C.size_t(0),
		)
		ret = int(w.resultBuffer.return_code)
		if err := getError(ret); err != nil {
			return err
		}
		written := int(w.resultBuffer.bytes_written)
		_, err := w.underlyingWriter.Write(w.dstBuffer[:written])
		if err != nil {
			C.ZSTD_freeCCtx(w.ctx)
			return err
		}

		if ret > 0 {
			w.dstBuffer = w.dstBuffer[:cap(w.dstBuffer)]
			if len(w.dstBuffer) < ret {
				w.dstBuffer = make([]byte, ret)
			}
		}
	}

	return getError(int(C.ZSTD_freeCCtx(w.ctx)))
}

// BulkReader is an io.ReadCloser that decompresses using a pre-digested dictionary.
type BulkReader struct {
	ctx                 *C.ZSTD_DCtx
	dDict               *C.struct_ZSTD_DDict_s
	compressionBuffer   []byte
	compressionLeft     int
	decompressionBuffer []byte
	decompOff           int
	decompSize          int
	remainingBytes      int
	firstError          error
	recommendedSrcSize  int
	resultBuffer        *C.bulkDecompressStream_result
	underlyingReader    io.Reader
}

// NewReader creates a new streaming decompressor using the BulkProcessor's dictionary.
func (p *BulkProcessor) NewReader(r io.Reader) *BulkReader {
	ctx := C.ZSTD_createDCtx()

	// Reference the pre-digested dictionary
	err := getError(int(C.ZSTD_DCtx_refDDict(ctx, p.dDict)))

	return &BulkReader{
		ctx:                 ctx,
		dDict:               p.dDict,
		compressionBuffer:   make([]byte, cSize),
		decompressionBuffer: make([]byte, dSize),
		firstError:          err,
		recommendedSrcSize:  cSize,
		resultBuffer:        new(C.bulkDecompressStream_result),
		underlyingReader:    r,
	}
}

// Read decompresses data into p.
func (r *BulkReader) Read(p []byte) (int, error) {
	if r.firstError != nil {
		return 0, r.firstError
	}

	if len(p) == 0 {
		return 0, nil
	}

	// If we already have some uncompressed bytes, return without blocking
	if r.decompSize > r.decompOff {
		if r.decompSize-r.decompOff > len(p) {
			copy(p, r.decompressionBuffer[r.decompOff:])
			r.decompOff += len(p)
			return len(p), nil
		}
		copy(p, r.decompressionBuffer[r.decompOff:r.decompSize])
		got := r.decompSize - r.decompOff
		r.decompOff = r.decompSize
		return got, nil
	}

	for {
		needsData := r.compressionLeft == 0

		var src []byte
		if !needsData {
			src = r.compressionBuffer[:r.compressionLeft]
		} else {
			src = r.compressionBuffer
			var n int
			var err error
			for n == 0 && err == nil {
				n, err = r.underlyingReader.Read(src[r.compressionLeft:])
			}
			if err != nil && err != io.EOF {
				return 0, err
			}

			if n == 0 && r.compressionLeft == 0 {
				if r.remainingBytes > 0 {
					return 0, io.ErrUnexpectedEOF
				}
				return 0, io.EOF
			}
			src = src[:r.compressionLeft+n]
		}

		var srcPtr *byte
		if len(src) > 0 {
			srcPtr = &src[0]
		}

		C.ZSTD_bulkDecompressStream_wrapper(
			r.resultBuffer,
			r.ctx,
			unsafe.Pointer(&r.decompressionBuffer[0]),
			C.size_t(len(r.decompressionBuffer)),
			unsafe.Pointer(srcPtr),
			C.size_t(len(src)),
		)
		retCode := int(r.resultBuffer.return_code)

		runtime.KeepAlive(src)
		if err := getError(retCode); err != nil {
			return 0, err
		}

		bytesConsumed := int(r.resultBuffer.bytes_consumed)
		if bytesConsumed < len(src) {
			left := src[bytesConsumed:]
			copy(r.compressionBuffer, left)
		}
		r.compressionLeft = len(src) - bytesConsumed
		r.decompSize = int(r.resultBuffer.bytes_written)
		r.decompOff = copy(p, r.decompressionBuffer[:r.decompSize])
		r.remainingBytes = retCode

		nsize := retCode
		if nsize <= 0 {
			nsize = r.recommendedSrcSize
		}
		if nsize < r.compressionLeft {
			nsize = r.compressionLeft
		}
		r.compressionBuffer = resize(r.compressionBuffer, nsize)

		if r.decompOff > 0 {
			return r.decompOff, nil
		}
	}
}

// Close frees the decompression context.
func (r *BulkReader) Close() error {
	if r.firstError != nil {
		return r.firstError
	}
	return getError(int(C.ZSTD_freeDCtx(r.ctx)))
}

// BulkDecompressWriter is an io.WriteCloser that decompresses using a pre-digested dictionary.
type BulkDecompressWriter struct {
	ctx                 *C.ZSTD_DCtx
	dDict               *C.struct_ZSTD_DDict_s
	underlyingWriter    io.Writer
	compressionBuffer   []byte
	decompressionBuffer []byte
	firstError          error
	resultBuffer        *C.bulkDecompressStream_result
}

// NewDecompressWriter creates a new streaming decompressor that writes decompressed data to w.
func (p *BulkProcessor) NewDecompressWriter(w io.Writer) *BulkDecompressWriter {
	ctx := C.ZSTD_createDCtx()

	// Reference the pre-digested dictionary
	err := getError(int(C.ZSTD_DCtx_refDDict(ctx, p.dDict)))

	return &BulkDecompressWriter{
		ctx:                 ctx,
		dDict:               p.dDict,
		underlyingWriter:    w,
		compressionBuffer:   make([]byte, cSize),
		decompressionBuffer: make([]byte, dSize),
		firstError:          err,
		resultBuffer:        new(C.bulkDecompressStream_result),
	}
}

// Write decompresses p and writes the result to the underlying writer.
func (w *BulkDecompressWriter) Write(p []byte) (int, error) {
	if w.firstError != nil {
		return 0, w.firstError
	}

	if len(p) == 0 {
		return 0, nil
	}

	originalLen := len(p)

	for len(p) > 0 {
		if len(w.compressionBuffer) < len(p) {
			w.compressionBuffer = resize(w.compressionBuffer, len(p))
		}

		copy(w.compressionBuffer, p)

		var srcPtr *byte
		if len(p) > 0 {
			srcPtr = &w.compressionBuffer[0]
		}

		C.ZSTD_bulkDecompressStream_wrapper(
			w.resultBuffer,
			w.ctx,
			unsafe.Pointer(&w.decompressionBuffer[0]),
			C.size_t(len(w.decompressionBuffer)),
			unsafe.Pointer(srcPtr),
			C.size_t(len(p)),
		)
		retCode := int(w.resultBuffer.return_code)

		runtime.KeepAlive(w.compressionBuffer)

		if err := getError(retCode); err != nil {
			w.firstError = err
			return 0, w.firstError
		}

		bytesDecompressed := int(w.resultBuffer.bytes_written)
		if bytesDecompressed > 0 {
			_, err := w.underlyingWriter.Write(w.decompressionBuffer[:bytesDecompressed])
			if err != nil {
				w.firstError = err
				return 0, w.firstError
			}
		}

		bytesConsumed := int(w.resultBuffer.bytes_consumed)
		p = p[bytesConsumed:]

		nsize := retCode
		if nsize > 0 && len(w.decompressionBuffer) < nsize {
			w.decompressionBuffer = resize(w.decompressionBuffer, nsize)
		}
	}

	return originalLen, nil
}

// Close frees resources.
func (w *BulkDecompressWriter) Close() error {
	if w.firstError != nil {
		return w.firstError
	}
	return getError(int(C.ZSTD_freeDCtx(w.ctx)))
}
