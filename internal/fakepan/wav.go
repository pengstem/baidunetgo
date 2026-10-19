package fakepan

import (
	"bytes"
	"encoding/binary"
	"math"
)

// SampleWAV makes a WAV of a soft chord, seconds long, 16-bit mono at
// 22.05 kHz.
func SampleWAV(seconds float64) []byte {
	const rate = 22050
	n := int(seconds * rate)
	var b bytes.Buffer
	w := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("RIFF")
	w(uint32(36 + n*2))
	b.WriteString("WAVEfmt ")
	w(uint32(16))
	w(uint16(1))
	w(uint16(1))
	w(uint32(rate))
	w(uint32(rate * 2))
	w(uint16(2))
	w(uint16(16))
	b.WriteString("data")
	w(uint32(n * 2))
	for i := range n {
		t := float64(i) / rate
		env := math.Min(1, t*8) * math.Min(1, (seconds-t)*4)
		v := (math.Sin(2*math.Pi*261.6*t) + math.Sin(2*math.Pi*329.6*t) + math.Sin(2*math.Pi*392*t)) / 3
		w(int16(v * env * 9000))
	}
	return b.Bytes()
}
