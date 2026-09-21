package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"wacallerapi/internal/voip/media"
)

// DecodeWAVToPCM16k reads a WAV audio stream and converts it to 16 kHz mono float32 PCM.
func DecodeWAVToPCM16k(r io.Reader) ([]float32, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(data) < 44 {
		return nil, errors.New("wav data too short")
	}

	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, errors.New("invalid wav header")
	}

	var audioFormat uint16
	var numChannels uint16
	var sampleRate uint32
	var bitsPerSample uint16
	var pcmOffset int = -1
	var pcmSize int

	// Parse chunks
	offset := 12
	for offset+8 <= len(data) {
		chunkID := string(data[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		offset += 8

		if chunkID == "fmt " && chunkSize >= 16 && offset+chunkSize <= len(data) {
			audioFormat = binary.LittleEndian.Uint16(data[offset : offset+2])
			numChannels = binary.LittleEndian.Uint16(data[offset+2 : offset+4])
			sampleRate = binary.LittleEndian.Uint32(data[offset+4 : offset+8])
			bitsPerSample = binary.LittleEndian.Uint16(data[offset+14 : offset+16])
		} else if chunkID == "data" {
			pcmOffset = offset
			pcmSize = chunkSize
			if pcmOffset+pcmSize > len(data) {
				pcmSize = len(data) - pcmOffset
			}
			break
		}
		offset += chunkSize
	}

	if pcmOffset == -1 {
		return nil, errors.New("wav data chunk not found")
	}
	if audioFormat != 1 { // 1 = PCM
		return nil, fmt.Errorf("unsupported wav format: %d (only uncompressed PCM is supported)", audioFormat)
	}
	if bitsPerSample != 16 {
		return nil, fmt.Errorf("unsupported bit depth: %d (only 16-bit PCM is supported)", bitsPerSample)
	}

	rawPCM := data[pcmOffset : pcmOffset+pcmSize]
	numSamples := len(rawPCM) / 2
	samples := make([]float32, 0, numSamples/int(numChannels))

	// Convert 16-bit integers to float32 and downmix stereo to mono if needed
	if numChannels == 1 {
		for i := 0; i+2 <= len(rawPCM); i += 2 {
			val := int16(binary.LittleEndian.Uint16(rawPCM[i : i+2]))
			samples = append(samples, float32(val)/32768.0)
		}
	} else if numChannels == 2 {
		for i := 0; i+4 <= len(rawPCM); i += 4 {
			left := int16(binary.LittleEndian.Uint16(rawPCM[i : i+2]))
			right := int16(binary.LittleEndian.Uint16(rawPCM[i+2 : i+4]))
			mixed := (float32(left) + float32(right)) / (2.0 * 32768.0)
			samples = append(samples, mixed)
		}
	} else {
		return nil, fmt.Errorf("unsupported channel count: %d", numChannels)
	}

	// Resample if sample rate != 16000
	if sampleRate != 16000 && sampleRate > 0 {
		samples = resampleLinear(samples, int(sampleRate), 16000)
	}

	return samples, nil
}

// FetchAudioFromURL downloads audio from an HTTP URL and decodes it to 16 kHz float32 PCM.
func FetchAudioFromURL(url string) ([]float32, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch audio url, status: %d", resp.StatusCode)
	}

	return DecodeWAVToPCM16k(resp.Body)
}

// EncodePCM16kToWAV encodes 16 kHz mono float32 PCM samples to standard 16-bit WAV bytes.
func EncodePCM16kToWAV(samples []float32) []byte {
	pcmBytes := media.PCMFloat32ToInt16LE(samples)
	dataSize := uint32(len(pcmBytes))
	fileSize := 36 + dataSize

	buf := new(bytes.Buffer)
	buf.Grow(int(44 + dataSize))

	// RIFF Header
	buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, fileSize)
	buf.WriteString("WAVE")

	// fmt subchunk
	buf.WriteString("fmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))     // Subchunk1Size
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))      // AudioFormat: PCM
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))      // NumChannels: 1
	_ = binary.Write(buf, binary.LittleEndian, uint32(16000))  // SampleRate: 16000
	_ = binary.Write(buf, binary.LittleEndian, uint32(32000))  // ByteRate: 16000 * 1 * 2
	_ = binary.Write(buf, binary.LittleEndian, uint16(2))      // BlockAlign: 1 * 2
	_ = binary.Write(buf, binary.LittleEndian, uint16(16))     // BitsPerSample: 16

	// data subchunk
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, dataSize)
	buf.Write(pcmBytes)

	return buf.Bytes()
}

func resampleLinear(in []float32, inRate, outRate int) []float32 {
	if inRate == outRate || len(in) == 0 {
		return in
	}
	ratio := float64(inRate) / float64(outRate)
	outLen := int(float64(len(in)) / ratio)
	out := make([]float32, outLen)

	for i := 0; i < outLen; i++ {
		srcIdx := float64(i) * ratio
		idx0 := int(srcIdx)
		idx1 := idx0 + 1
		if idx1 >= len(in) {
			idx1 = len(in) - 1
		}
		frac := float32(srcIdx - float64(idx0))
		out[i] = in[idx0]*(1.0-frac) + in[idx1]*frac
	}
	return out
}
