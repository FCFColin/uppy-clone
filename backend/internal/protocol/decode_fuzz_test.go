package protocol

import "testing"

func FuzzDecodeMessage(f *testing.F) {
	f.Add([]byte{MsgTap, 0, 0, 0, 0, 0, 0, 0, 0})
	f.Add([]byte{})
	f.Add([]byte{0xff, 0x01, 0x02})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = DecodeMessage(data)
	})
}

func FuzzDecodeTap(f *testing.F) {
	f.Add([]byte{0, 0, 128, 63, 0, 0, 0, 64})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _, _ = DecodeTap(data)
	})
}
