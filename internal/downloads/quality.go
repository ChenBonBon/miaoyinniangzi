package downloads

import (
	"bufio"
	"io"
	"os"
)

// Resolve labels alone cannot establish an MP3 bitrate. Walk actual MPEG Layer
// III frames, excluding ID3 metadata. Unsupported/truncated layouts stay generic
// MP3 and must never cause a 320K/128K local-file deduplication hit.
func mp3Quality(file *os.File, size int64) string {
	reader := bufio.NewReaderSize(io.NewSectionReader(file, 0, size), 4096)
	header, err := reader.Peek(10)
	if err != nil {
		return "mp3"
	}
	if string(header[:3]) == "ID3" {
		length := int64(10)
		for _, v := range header[6:10] {
			if v&128 != 0 {
				return "mp3"
			}
		}
		length += int64(header[6])<<21 | int64(header[7])<<14 | int64(header[8])<<7 | int64(header[9])
		if header[3] == 4 && header[5]&16 != 0 {
			length += 10
		}
		if length > 32*1024*1024 || length >= size {
			return "mp3"
		}
		if _, err = reader.Discard(int(length)); err != nil {
			return "mp3"
		}
	}
	var bytes int64
	var seconds float64
	for {
		frame, err := reader.Peek(4)
		if err == io.EOF {
			break
		}
		if err != nil {
			return "mp3"
		}
		if string(frame[:3]) == "TAG" || string(frame) == "APET" {
			break
		}
		if frame[0] != 255 || frame[1]&224 != 224 || frame[1]&6 != 2 {
			return "mp3"
		}
		version := (frame[1] >> 3) & 3
		index := (frame[2] >> 4) & 15
		rateIndex := (frame[2] >> 2) & 3
		if version == 1 || index == 0 || index == 15 || rateIndex == 3 {
			return "mp3"
		}
		rates := [3]int{44100, 48000, 32000}
		rate := rates[rateIndex]
		samples, factor := 1152, 144
		bits := [16]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
		if version != 3 {
			rate /= 2
			samples = 576
			factor = 72
			bits = [16]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0}
			if version == 0 {
				rate /= 2
			}
		}
		length := factor*bits[index]*1000/rate + int((frame[2]>>1)&1)
		if length < 4 {
			return "mp3"
		}
		if _, err = reader.Discard(length); err != nil {
			return "mp3"
		}
		bytes += int64(length)
		seconds += float64(samples) / float64(rate)
	}
	if seconds < 1 {
		return "mp3"
	}
	bitrate := float64(bytes) * 8 / seconds
	if bitrate >= 310000 && bitrate <= 330000 {
		return "320k"
	}
	if bitrate >= 122000 && bitrate <= 134000 {
		return "128k"
	}
	return "mp3"
}
