package client

import (
	"encoding/binary"
	"io"
	"math"
	"os"
)

// 音视频时长解析（移植自官方 shortcuts/im/helpers.go）：上传 opus/mp4 到 IM 时带上
// duration（毫秒），客户端才能显示语音/视频时长。只读取必要的字节：OGG 读尾部 64KB，
// MP4 逐个读 box 头直到 moov。任何解析失败都返回 0（不影响上传）。

// ParseLocalMediaDurationMs 解析本地音视频时长（毫秒）。fileType 为 IM 上传类型 opus / mp4。
func ParseLocalMediaDurationMs(path, fileType string) int {
	if fileType != "opus" && fileType != "mp4" {
		return 0
	}
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return 0
	}
	var ms int64
	if fileType == "opus" {
		ms = readOggDuration(f, info.Size())
	} else {
		ms = readMp4Duration(f, info.Size())
	}
	if ms <= 0 || ms > math.MaxInt32 {
		return 0
	}
	return int(ms)
}

// readOggDuration 读文件尾部，取最后一个 Ogg 页的 granule position（Opus 固定 48kHz）。
func readOggDuration(f io.ReaderAt, fileSize int64) int64 {
	const maxTail = 65536
	readSize := fileSize
	if readSize > maxTail {
		readSize = maxTail
	}
	buf := make([]byte, readSize)
	if _, err := f.ReadAt(buf, fileSize-readSize); err != nil && err != io.EOF {
		return 0
	}
	return parseOggOpusDuration(buf)
}

func parseOggOpusDuration(data []byte) int64 {
	offset := -1
	for i := len(data) - 4; i >= 0; i-- {
		if data[i] == 'O' && data[i+1] == 'g' && data[i+2] == 'g' && data[i+3] == 'S' {
			offset = i
			break
		}
	}
	if offset < 0 {
		return 0
	}
	granuleOffset := offset + 6
	if granuleOffset+8 > len(data) {
		return 0
	}
	lo := binary.LittleEndian.Uint32(data[granuleOffset:])
	hi := binary.LittleEndian.Uint32(data[granuleOffset+4:])
	granule := uint64(hi)<<32 | uint64(lo)
	if granule == 0 {
		return 0
	}
	return int64(math.Ceil(float64(granule)/48000.0)) * 1000
}

// readMp4Duration 顺序遍历顶层 box，找到 moov 后在其中找 mvhd 读 timescale/duration。
func readMp4Duration(f io.ReaderAt, fileSize int64) int64 {
	hdr := make([]byte, 16)
	var offset int64
	for offset+8 <= fileSize {
		if _, err := f.ReadAt(hdr[:8], offset); err != nil {
			return 0
		}
		size := int64(binary.BigEndian.Uint32(hdr[0:4]))
		typ := string(hdr[4:8])

		var boxEnd, dataStart int64
		switch {
		case size == 0:
			boxEnd = fileSize
			dataStart = offset + 8
		case size == 1:
			if _, err := f.ReadAt(hdr[8:16], offset+8); err != nil {
				return 0
			}
			// 64 位 largesize 含 16 字节头；超出文件的值直接拒绝（同时防止 int64 溢出）。
			largesize := binary.BigEndian.Uint64(hdr[8:16])
			if largesize < 16 || largesize > uint64(fileSize-offset) {
				return 0
			}
			boxEnd = offset + int64(largesize)
			dataStart = offset + 16
		case size < 8:
			return 0
		default:
			boxEnd = offset + size
			dataStart = offset + 8
		}

		if typ == "moov" {
			moovLen := boxEnd - dataStart
			if moovLen <= 0 || moovLen > 10<<20 || dataStart+moovLen > fileSize {
				return 0
			}
			moov := make([]byte, moovLen)
			if _, err := f.ReadAt(moov, dataStart); err != nil && err != io.EOF {
				return 0
			}
			mvhdStart, mvhdEnd := findMP4Box(moov, 0, len(moov), "mvhd")
			if mvhdStart < 0 {
				return 0
			}
			return parseMvhdPayload(moov[mvhdStart:mvhdEnd])
		}
		if boxEnd <= offset {
			return 0
		}
		offset = boxEnd
	}
	return 0
}

func findMP4Box(data []byte, start, end int, boxType string) (int, int) {
	offset := start
	for offset+8 <= end {
		size := int(binary.BigEndian.Uint32(data[offset:]))
		typ := string(data[offset+4 : offset+8])
		var boxEnd, dataStart int
		switch {
		case size == 0:
			boxEnd = end
			dataStart = offset + 8
		case size == 1:
			if offset+16 > end {
				return -1, -1
			}
			largesize := binary.BigEndian.Uint64(data[offset+8:])
			if largesize < 16 || largesize > uint64(end-offset) {
				return -1, -1
			}
			boxEnd = offset + int(largesize)
			dataStart = offset + 16
		default:
			if size < 8 {
				return -1, -1
			}
			boxEnd = offset + size
			dataStart = offset + 8
		}
		if typ == boxType {
			if boxEnd > end {
				boxEnd = end
			}
			return dataStart, boxEnd
		}
		offset = boxEnd
	}
	return -1, -1
}

// parseMvhdPayload 解析 mvhd 负载（version 0：32 位字段；version 1：64 位字段），返回毫秒。
func parseMvhdPayload(data []byte) int64 {
	if len(data) < 1 {
		return 0
	}
	version := data[0]
	var timescale, duration uint64
	if version == 0 {
		if len(data) < 20 {
			return 0
		}
		timescale = uint64(binary.BigEndian.Uint32(data[12:]))
		duration = uint64(binary.BigEndian.Uint32(data[16:]))
	} else {
		if len(data) < 32 {
			return 0
		}
		timescale = uint64(binary.BigEndian.Uint32(data[20:]))
		duration = binary.BigEndian.Uint64(data[24:])
	}
	if timescale == 0 || duration == 0 {
		return 0
	}
	return int64(math.Round(float64(duration) / float64(timescale) * 1000))
}
