// transfer/transfer.go
package transfer

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/juju/ratelimit"
	"github.com/pierrec/lz4/v4"
	"go.uber.org/zap"

	"lvmsync_go/common"
)

var Logger *zap.Logger

func SetLogger(logger *zap.Logger) {
	Logger = logger
}

type Range struct {
	Start int64
	End   int64
}

type BlockTask struct {
	Index int
	R     Range
}

type BlockResult struct {
	Index  int
	Offset uint64
	Size   uint32
	Data   []byte
	Err    error
}

const maxRetries = 3

func ReadMetadataHeader(metadataPath string) (int64, error) {
	file, err := os.Open(metadataPath)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	buf := make([]byte, 16)
	if _, err := io.ReadFull(file, buf); err != nil {
		return 0, err
	}
	magic := binary.LittleEndian.Uint32(buf[0:4])
	valid := binary.LittleEndian.Uint32(buf[4:8])
	version := binary.LittleEndian.Uint32(buf[8:12])
	chunk := binary.LittleEndian.Uint32(buf[12:16])
	if magic != 0x70416e53 {
		return 0, fmt.Errorf("invalid snapshot magic number")
	}
	if valid != 1 {
		return 0, fmt.Errorf("snapshot is marked as invalid")
	}
	if version != 1 {
		return 0, fmt.Errorf("incompatible snapshot metadata version")
	}
	return int64(chunk) * 512, nil
}

func GetDifferences(metadataPath string, chunkSize int64) ([]Range, error) {
	file, err := os.Open(metadataPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if _, err := file.Seek(chunkSize, io.SeekStart); err != nil {
		return nil, err
	}
	var diffs []uint64
	buf := make([]byte, 16)
	for {
		_, err := io.ReadFull(file, buf)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return nil, err
		}
		originOffset := binary.LittleEndian.Uint64(buf[0:8])
		snapOffset := binary.LittleEndian.Uint64(buf[8:16])
		if snapOffset == 0 {
			break
		}
		diffs = append(diffs, originOffset)
	}
	var ranges []Range
	for _, block := range diffs {
		start := int64(block) * chunkSize
		end := (int64(block)+1)*chunkSize - 1
		ranges = append(ranges, Range{Start: start, End: end})
	}
	return ranges, nil
}

func GetMetadataDevice(snapshot string) string {
	base := filepath.Base(snapshot)
	parts := strings.SplitN(base, "-", 2)
	if len(parts) < 2 {
		return ""
	}
	vg := strings.ReplaceAll(parts[0], "-", "--")
	lv := strings.ReplaceAll(parts[1], "-", "--")
	return "/dev/mapper/" + vg + "-" + lv + "-cow"
}

func ZeroCopyTransfer(src *os.File, dst *os.File, offset int64, length int64) error {
	pipeFds := make([]int, 2)
	if err := syscall.Pipe(pipeFds); err != nil {
		return fmt.Errorf("pipe creation failed: %v", err)
	}
	defer syscall.Close(pipeFds[0])
	defer syscall.Close(pipeFds[1])
	remaining := length
	off := offset
	for remaining > 0 {
		n, err := syscall.Splice(int(src.Fd()), &off, pipeFds[1], nil, int(remaining), 0)
		if err != nil {
			return fmt.Errorf("splice read failed: %v", err)
		}
		if n == 0 {
			break
		}
		_, err = syscall.Splice(pipeFds[0], nil, int(dst.Fd()), nil, int(n), 0)
		if err != nil {
			return fmt.Errorf("splice write failed: %v", err)
		}
		remaining -= int64(n)
	}
	return nil
}

func ReadBlock(src *os.File, offset int64, size int) ([]byte, error) {
	buf := make([]byte, size)
	n, err := src.ReadAt(buf, offset)
	if err != nil {
		return nil, err
	}
	if n != size {
		return nil, fmt.Errorf("short read: expected %d, got %d", size, n)
	}
	return buf, nil
}

func ReadBlockWithRetries(src *os.File, offset int64, size int, maxRetries int, useZeroCopy bool) ([]byte, error) {
	var data []byte
	var err error
	if useZeroCopy {
		r, w, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		defer r.Close()
		for attempt := 0; attempt < maxRetries; attempt++ {
			err = ZeroCopyTransfer(src, w, offset, int64(size))
			if err == nil {
				break
			}
			Logger.Warn("Zero-copy transfer failed", zap.Int64("offset", offset), zap.Int("size", size), zap.Int("attempt", attempt+1), zap.Error(err))
			time.Sleep(100 * time.Millisecond)
		}
		w.Close()
		if err != nil {
			return nil, err
		}
		data, err = ioutil.ReadAll(r)
		if err != nil {
			return nil, err
		}
		if len(data) != size {
			return nil, fmt.Errorf("zero-copy short read: expected %d, got %d", size, len(data))
		}
		return data, nil
	}
	for attempt := 0; attempt < maxRetries; attempt++ {
		data, err = ReadBlock(src, offset, size)
		if err == nil {
			return data, nil
		}
		Logger.Warn("Failed to read block", zap.Int64("offset", offset), zap.Int("size", size), zap.Int("attempt", attempt+1), zap.Error(err))
		time.Sleep(100 * time.Millisecond)
	}
	return nil, err
}

func WrapRateLimitedWriter(w io.Writer, speedLimit int) io.Writer {
	if speedLimit > 0 {
		bucket := ratelimit.NewBucketWithRate(float64(speedLimit), int64(speedLimit))
		return ratelimit.Writer(w, bucket)
	}
	return w
}

func DumpChangesSequential(snapshot, source string, out io.Writer, verbose bool, useZeroCopy bool, verifyChecksum bool, compress string, speedLimit int, resumeState string, parallel int) error {
	metadataDevice := GetMetadataDevice(snapshot)
	if metadataDevice == "" {
		return fmt.Errorf("failed to determine metadata device from snapshot %s", snapshot)
	}
	chunkSize, err := ReadMetadataHeader(metadataDevice)
	if err != nil {
		return fmt.Errorf("error reading metadata header: %v", err)
	}
	Logger.Info("Metadata header read", zap.Int64("chunkSize", chunkSize))
	ranges, err := GetDifferences(metadataDevice, chunkSize)
	if err != nil {
		return fmt.Errorf("error getting differences: %v", err)
	}
	Logger.Info("Changed blocks determined", zap.Int("blockCount", len(ranges)))
	handshake := common.ProtocolVersion
	if verifyChecksum {
		handshake += " checksum"
	}
	fmt.Fprintln(out, handshake)
	limitedOut := WrapRateLimitedWriter(out, speedLimit)
	bufOut := bufio.NewWriter(limitedOut)
	if compress == "lz4" {
		bufOut = bufio.NewWriter(lz4.NewWriter(bufOut))
	}
	srcFile, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("failed to open source device %s: %v", source, err)
	}
	defer srcFile.Close()
	startTime := time.Now()
	var totalBytes int64
	for i, r := range ranges {
		size := int(r.End - r.Start + 1)
		data, err := ReadBlockWithRetries(srcFile, r.Start, size, maxRetries, useZeroCopy)
		if err != nil {
			return fmt.Errorf("error reading block at offset %d: %v", r.Start, err)
		}
		header := make([]byte, 12)
		binary.BigEndian.PutUint64(header[0:8], uint64(r.Start))
		binary.BigEndian.PutUint32(header[8:12], uint32(size))
		if verifyChecksum {
			sum := sha256.Sum256(data)
			header = append(header, sum[:]...)
		}
		if _, err := bufOut.Write(header); err != nil {
			return fmt.Errorf("failed to write header: %v", err)
		}
		if _, err := bufOut.Write(data); err != nil {
			return fmt.Errorf("failed to write block data: %v", err)
		}
		totalBytes += int64(size)
		if resumeState != "" {
			err := ioutil.WriteFile(resumeState, []byte(fmt.Sprintf("%d", i+1)), 0644)
			if err != nil {
				Logger.Warn("Failed to update resume state", zap.Error(err))
			}
		}
		if verbose && i > 0 && i%100 == 0 {
			elapsed := time.Since(startTime).Seconds()
			speed := float64(totalBytes) / elapsed / 1048576.0
			Logger.Info("Sequential dump progress", zap.Int("chunk", i+1), zap.Float64("MB/s", speed))
		}
	}
	if err := bufOut.Flush(); err != nil {
		return fmt.Errorf("failed to flush output: %v", err)
	}
	elapsed := time.Since(startTime).Seconds()
	Logger.Info("Sequential transfer complete", zap.Int64("bytes", totalBytes), zap.Float64("seconds", elapsed),
		zap.Float64("MB/s", float64(totalBytes)/elapsed/1048576.0))
	return nil
}

func DumpChangesParallel(snapshot, source string, out io.Writer, verbose bool, verifyChecksum bool, compress string, speedLimit int, resumeState string, parallel int) error {
	metadataDevice := GetMetadataDevice(snapshot)
	if metadataDevice == "" {
		return fmt.Errorf("failed to determine metadata device from snapshot %s", snapshot)
	}
	chunkSize, err := ReadMetadataHeader(metadataDevice)
	if err != nil {
		return fmt.Errorf("error reading metadata header: %v", err)
	}
	Logger.Info("Metadata header read", zap.Int64("chunkSize", chunkSize))
	ranges, err := GetDifferences(metadataDevice, chunkSize)
	if err != nil {
		return fmt.Errorf("error getting differences: %v", err)
	}
	Logger.Info("Changed blocks determined", zap.Int("blockCount", len(ranges)))
	handshake := common.ProtocolVersion
	if verifyChecksum {
		handshake += " checksum"
	}
	fmt.Fprintln(out, handshake)
	limitedOut := WrapRateLimitedWriter(out, speedLimit)
	bufOut := bufio.NewWriter(limitedOut)
	if compress == "lz4" {
		bufOut = bufio.NewWriter(lz4.NewWriter(bufOut))
	}
	srcFile, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("failed to open source device %s: %v", source, err)
	}
	defer srcFile.Close()
	numBlocks := len(ranges)
	tasks := make(chan BlockTask, numBlocks)
	results := make(chan *BlockResult, numBlocks)
	resumeStart := 0
	if resumeState != "" {
		data, err := ioutil.ReadFile(resumeState)
		if err == nil {
			if val, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
				resumeStart = val
				Logger.Info("Resuming from block", zap.Int("resumeStart", resumeStart))
			}
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range tasks {
				size := int(task.R.End - task.R.Start + 1)
				data, err := ReadBlockWithRetries(srcFile, task.R.Start, size, maxRetries, false)
				results <- &BlockResult{
					Index:  task.Index,
					Offset: uint64(task.R.Start),
					Size:   uint32(size),
					Data:   data,
					Err:    err,
				}
			}
		}()
	}
	for i := resumeStart; i < numBlocks; i++ {
		tasks <- BlockTask{Index: i, R: ranges[i]}
	}
	close(tasks)
	go func() {
		wg.Wait()
		close(results)
	}()
	blockResults := make([]*BlockResult, numBlocks-resumeStart)
	count := 0
	for res := range results {
		if res.Err != nil {
			return fmt.Errorf("error in block %d: %v", res.Index, res.Err)
		}
		blockResults[res.Index-resumeStart] = res
		count++
	}
	if count != numBlocks-resumeStart {
		return fmt.Errorf("expected %d blocks, got %d", numBlocks-resumeStart, count)
	}
	startTime := time.Now()
	var totalBytes int64
	for i, res := range blockResults {
		header := make([]byte, 12)
		binary.BigEndian.PutUint64(header[0:8], res.Offset)
		binary.BigEndian.PutUint32(header[8:12], res.Size)
		if verifyChecksum {
			sum := sha256.Sum256(res.Data)
			header = append(header, sum[:]...)
		}
		if _, err := bufOut.Write(header); err != nil {
			return fmt.Errorf("failed to write header for block %d: %v", i, err)
		}
		if _, err := bufOut.Write(res.Data); err != nil {
			return fmt.Errorf("failed to write data for block %d: %v", i, err)
		}
		totalBytes += int64(res.Size)
		if resumeState != "" {
			err := ioutil.WriteFile(resumeState, []byte(fmt.Sprintf("%d", i+resumeStart+1)), 0644)
			if err != nil {
				Logger.Warn("Failed to update resume state", zap.Error(err))
			}
		}
		if verbose && i > 0 && i%100 == 0 {
			elapsed := time.Since(startTime).Seconds()
			speed := float64(totalBytes) / elapsed / 1048576.0
			Logger.Info("Parallel dump progress", zap.Int("block", i+resumeStart+1), zap.Float64("MB/s", speed))
		}
	}
	if err := bufOut.Flush(); err != nil {
		return fmt.Errorf("failed to flush output: %v", err)
	}
	elapsed := time.Since(startTime).Seconds()
	Logger.Info("Parallel transfer complete", zap.Int64("bytes", totalBytes), zap.Float64("seconds", elapsed),
		zap.Float64("MB/s", float64(totalBytes)/elapsed/1048576.0))
	return nil
}

func ProcessDumpData(in io.Reader, destPath string, verbose bool, verifyChecksum bool, compress string) error {
	var rdr io.Reader = in
	if compress == "lz4" {
		rdr = lz4.NewReader(rdr)
	}
	reader := bufio.NewReader(rdr)
	handshake, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read protocol handshake: %v", err)
	}
	handshake = strings.TrimSpace(handshake)
	verify := false
	if strings.Contains(handshake, "checksum") {
		verify = true
	}
	if handshake != common.ProtocolVersion && handshake != (common.ProtocolVersion+" checksum") {
		return fmt.Errorf("protocol mismatch: got %q, expected %q or %q", handshake, common.ProtocolVersion, common.ProtocolVersion+" checksum")
	}
	destFile, err := os.OpenFile(destPath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("failed to open destination device %s: %v", destPath, err)
	}
	defer destFile.Close()
	startTime := time.Now()
	var totalBytes int64
	headerLen := 12
	if verify {
		headerLen += 32
	}
	headerBuf := make([]byte, headerLen)
	for {
		_, err := io.ReadFull(reader, headerBuf)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read chunk header: %v", err)
		}
		offset := binary.BigEndian.Uint64(headerBuf[0:8])
		chunkSize := binary.BigEndian.Uint32(headerBuf[8:12])
		var transmittedSum [32]byte
		if verify {
			copy(transmittedSum[:], headerBuf[12:44])
		}
		data := make([]byte, chunkSize)
		if _, err := io.ReadFull(reader, data); err != nil {
			return fmt.Errorf("failed to read chunk data: %v", err)
		}
		if verify {
			computed := sha256.Sum256(data)
			if transmittedSum != computed {
				return fmt.Errorf("checksum mismatch at offset %d", offset)
			}
		}
		if _, err := destFile.Seek(int64(offset), os.SEEK_SET); err != nil {
			Logger.Warn("Seek error", zap.Uint64("offset", offset), zap.Error(err))
			continue
		}
		if _, err := destFile.Write(data); err != nil {
			return fmt.Errorf("failed to write data at offset %d: %v", offset, err)
		}
		totalBytes += int64(chunkSize)
	}
	elapsed := time.Since(startTime).Seconds()
	Logger.Info("Applied changes", zap.Int64("bytes", totalBytes), zap.Float64("seconds", elapsed),
		zap.Float64("MB/s", float64(totalBytes)/elapsed/1048576.0))
	return nil
}

func RunApply(applyFile, destDevice string, verbose bool, verifyChecksum bool, compress string) error {
	var in io.Reader
	if applyFile == "-" {
		in = os.Stdin
	} else {
		f, err := os.Open(applyFile)
		if err != nil {
			return fmt.Errorf("failed to open apply file %s: %v", applyFile, err)
		}
		defer f.Close()
		in = f
	}
	return ProcessDumpData(in, destDevice, verbose, verifyChecksum, compress)
}
