package recording

import "encoding/binary"

// wavHeaderSize is the size of the canonical 44-byte PCM WAV header.
const wavHeaderSize = 44

// EncodeWAV wraps mono PCM16LE samples in a WAV container so the stored object
// plays in any browser or audio tool without a codec. pcm must hold a whole
// number of 16-bit samples.
func EncodeWAV(pcm []byte, sampleRate int) []byte {
	const (
		channels      = 1
		bitsPerSample = 16
	)
	blockAlign := channels * bitsPerSample / 8
	byteRate := sampleRate * blockAlign

	out := make([]byte, wavHeaderSize+len(pcm))
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(36+len(pcm)))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:20], 16) // fmt chunk size
	binary.LittleEndian.PutUint16(out[20:22], 1)  // PCM
	binary.LittleEndian.PutUint16(out[22:24], channels)
	binary.LittleEndian.PutUint32(out[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(out[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(out[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(out[34:36], bitsPerSample)
	copy(out[36:40], "data")
	binary.LittleEndian.PutUint32(out[40:44], uint32(len(pcm)))
	copy(out[wavHeaderSize:], pcm)
	return out
}

// DurationMillis is the playing time of mono PCM16 audio.
func DurationMillis(pcmBytes, sampleRate int) int {
	if sampleRate <= 0 {
		return 0
	}
	return int(int64(pcmBytes) / 2 * 1000 / int64(sampleRate))
}
