package audio

import (
	"bytes"
	"math"
	"testing"
)

func TestWAVRoundtrip(t *testing.T) {
	// Generate 16 kHz 440 Hz sine wave for 0.5s
	sampleRate := 16000
	duration := 0.5
	numSamples := int(float64(sampleRate) * duration)
	samples := make([]float32, numSamples)
	for i := range samples {
		samples[i] = float32(math.Sin(2 * math.Pi * 440 * float64(i) / float64(sampleRate)))
	}

	wavBytes := EncodePCM16kToWAV(samples)
	if len(wavBytes) <= 44 {
		t.Fatalf("expected wav bytes > 44, got %d", len(wavBytes))
	}

	decoded, err := DecodeWAVToPCM16k(bytes.NewReader(wavBytes))
	if err != nil {
		t.Fatalf("failed to decode wav: %v", err)
	}

	if len(decoded) != len(samples) {
		t.Fatalf("sample count mismatch: expected %d, got %d", len(samples), len(decoded))
	}

	// Verify sample difference is negligible
	var maxDiff float32
	for i := range samples {
		diff := float32(math.Abs(float64(samples[i] - decoded[i])))
		if diff > maxDiff {
			maxDiff = diff
		}
	}
	if maxDiff > 0.001 {
		t.Fatalf("max diff too large: %v", maxDiff)
	}
}
