// Package songcache provides private, revocable three-slot playback caching.
package songcache

import (
	"encoding/binary"
	"errors"
)

var errMore = errors.New("more audio frames required")
var ErrCache = errors.New("音频缓存暂时不可用，请重试")

const maxPrefix = 32 * 1024 * 1024

// prefixEnd returns an original-file byte boundary covering at least seconds.
// The retained prefix is not advertised as a standalone, shortened audio file.
func prefixEnd(data []byte, codec string, seconds float64) (int, error) {
	if codec == "mp3" {
		return mp3Prefix(data, seconds)
	}
	if codec == "flac" {
		return flacPrefix(data, seconds)
	}
	return 0, ErrCache
}
func mp3Prefix(data []byte, seconds float64) (int, error) {
	pos := 0
	if len(data) < 10 {
		return 0, errMore
	}
	if string(data[:3]) == "ID3" {
		for _, b := range data[6:10] {
			if b&128 != 0 {
				return 0, ErrCache
			}
		}
		length := int(data[6])<<21 | int(data[7])<<14 | int(data[8])<<7 | int(data[9])
		pos = 10 + length
		if data[5]&16 != 0 {
			pos += 10
		}
		if pos > 16*1024*1024 {
			return 0, ErrCache
		}
	}
	elapsed := 0.0
	started := false
	for {
		if pos+4 > len(data) {
			return 0, errMore
		}
		b := data[pos : pos+4]
		version := (b[1] >> 3) & 3
		layer := (b[1] >> 1) & 3
		rateIndex := (b[2] >> 2) & 3
		bitIndex := b[2] >> 4
		if b[0] != 255 || b[1]&224 != 224 || version == 1 || layer != 1 || rateIndex == 3 || bitIndex == 0 || bitIndex == 15 {
			if started || pos > 16*1024*1024 {
				return 0, ErrCache
			}
			pos++
			continue
		}
		rates := [3]int{44100, 48000, 32000}
		rate := rates[rateIndex]
		bitrate := 0
		samples := 1152
		factor := 144
		if version == 3 {
			bitrate = ([16]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320})[bitIndex]
		} else {
			bitrate = ([16]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160})[bitIndex]
			rate /= 2
			samples = 576
			factor = 72
			if version == 0 {
				rate /= 2
			}
		}
		frame := factor*bitrate*1000/rate + int((b[2]>>1)&1)
		if pos+frame > len(data) {
			return 0, errMore
		}
		pos += frame
		elapsed += float64(samples) / float64(rate)
		started = true
		if elapsed >= seconds+.06 {
			return pos, nil
		} // decoder delay/padding allowance
	}
}
func crc8(data []byte) byte {
	var crc byte
	for _, b := range data {
		crc ^= b
		for range 8 {
			if crc&128 != 0 {
				crc = crc<<1 ^ 7
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
func crc16(data []byte) uint16 {
	var crc uint16
	for _, b := range data {
		crc ^= uint16(b) << 8
		for range 8 {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x8005
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
func flacHeader(data []byte, blockMax int) (sample uint64, block int, length int, ok bool) {
	if len(data) < 6 || data[0] != 255 || data[1]&254 != 248 || data[3]&1 != 0 {
		return
	}
	code := data[2] >> 4
	sampleCode := data[2] & 15
	if code == 0 || sampleCode == 15 || data[3]>>4 > 10 {
		return
	}
	pos := 4
	b := data[pos]
	var count int
	if b < 128 {
		sample = uint64(b)
		count = 1
	} else {
		mask := byte(128)
		for b&mask != 0 {
			count++
			mask >>= 1
		}
		if count < 2 || count > 7 || pos+count >= len(data) {
			return 0, 0, 0, false
		}
		sample = uint64(b & ((1 << uint(7-count)) - 1))
		for i := 1; i < count; i++ {
			v := data[pos+i]
			if v&192 != 128 {
				return 0, 0, 0, false
			}
			sample = sample<<6 | uint64(v&63)
		}
	}
	pos += count
	switch {
	case code == 1:
		block = 192
	case code >= 2 && code <= 5:
		block = 576 << uint(code-2)
	case code == 6:
		if pos >= len(data) {
			return 0, 0, 0, false
		}
		block = int(data[pos]) + 1
		pos++
	case code == 7:
		if pos+2 > len(data) {
			return 0, 0, 0, false
		}
		block = int(binary.BigEndian.Uint16(data[pos:])) + 1
		pos += 2
	default:
		block = 256 << uint(code-8)
	}
	if sampleCode == 12 {
		pos++
	}
	if sampleCode == 13 || sampleCode == 14 {
		pos += 2
	}
	if pos >= len(data) || crc8(data[:pos+1]) != 0 {
		return 0, 0, 0, false
	}
	if data[1]&1 == 0 {
		sample *= uint64(blockMax)
	}
	return sample, block, pos + 1, true
}
func flacPrefix(data []byte, seconds float64) (int, error) {
	if len(data) < 42 {
		return 0, errMore
	}
	if string(data[:4]) != "fLaC" || data[4]&127 != 0 || data[7] != 34 {
		return 0, ErrCache
	}
	maxBlock := int(binary.BigEndian.Uint16(data[10:12]))
	rate := binary.BigEndian.Uint64(data[18:26]) >> 44
	if maxBlock == 0 || rate == 0 {
		return 0, ErrCache
	}
	pos := 4
	for {
		if pos+4 > len(data) {
			return 0, errMore
		}
		length := int(data[pos+1])<<16 | int(data[pos+2])<<8 | int(data[pos+3])
		last := data[pos]&128 != 0
		pos += 4 + length
		if pos > 16*1024*1024 {
			return 0, ErrCache
		}
		if pos > len(data) {
			return 0, errMore
		}
		if last {
			break
		}
	}
	first, block, _, ok := flacHeader(data[pos:], maxBlock)
	if !ok {
		if len(data)-pos < 32 {
			return 0, errMore
		}
		return 0, ErrCache
	}
	if first != 0 {
		return 0, ErrCache
	}
	start := pos
	expected := uint64(block)
	for scan := pos + 6; scan+6 <= len(data); scan++ {
		if data[scan] != 255 || data[scan+1]&254 != 248 {
			continue
		}
		sample, nextBlock, _, valid := flacHeader(data[scan:], maxBlock)
		if !valid || sample != expected || crc16(data[start:scan]) != 0 {
			continue
		}
		if float64(sample)/float64(rate) >= seconds {
			return scan, nil
		}
		start = scan
		expected = sample + uint64(nextBlock)
	}
	return 0, errMore
}
