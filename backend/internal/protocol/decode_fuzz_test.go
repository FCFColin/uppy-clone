package protocol

import "testing"

func FuzzDecodeMessage(f *testing.F) {
	f.Add([]byte{MsgTap, 0, 0, 0, 0, 0, 0, 0, 0})
	f.Add([]byte{})
	f.Add([]byte{0xff, 0x01, 0x02})
	f.Add([]byte{MsgSnapshot, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	f.Add(make([]byte, 65536))
	f.Add([]byte{MsgSetNickname, 5, 'h', 'e', 'l', 'l', 'o'})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = DecodeMessage(data)
	})
}

func FuzzDecodeTap(f *testing.F) {
	f.Add([]byte{0, 0, 128, 63, 0, 0, 0, 64})
	f.Add([]byte{})
	f.Add([]byte{0, 0, 128})
	f.Add(make([]byte, 8))
	f.Add(make([]byte, 65536))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _, _ = DecodeTap(data)
	})
}

func FuzzDecodeSetNickname(f *testing.F) {
	f.Add([]byte{MsgSetNickname, 3, 'a', 'b', 'c'})
	f.Add([]byte{MsgSetNickname})
	f.Add([]byte{MsgTap, 0})
	f.Add([]byte{})
	f.Add(make([]byte, 256))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = DecodeSetNickname(data)
	})
}

func FuzzDecodeRestartVote(f *testing.F) {
	f.Add([]byte{MsgRestartVote})
	f.Add([]byte{MsgTap})
	f.Add([]byte{})
	f.Add(make([]byte, 100))
	f.Fuzz(func(t *testing.T, data []byte) {
		_ = DecodeRestartVote(data)
	})
}

func FuzzDecodePing(f *testing.F) {
	f.Add([]byte{MsgPing})
	f.Add([]byte{})
	f.Add(make([]byte, 100))
	f.Fuzz(func(t *testing.T, data []byte) {
		_ = DecodePing(data)
	})
}
