package data

import "testing"

func TestSCRRoundTrip(t *testing.T) {
	for _, value := range []int{0, 1, 9, 10, 99, 100, 101, 1005, 48000, 123456789} {
		if got := SCR_StrToInt(SCR_PackInt(value)); got != value {
			t.Errorf("%d -> %v -> %d", value, SCR_PackInt(value), got)
		}

		for _, b := range SCR_PackInt(value) {
			if b == 0x09 || b == 0x0A {
				t.Errorf("%d packs to a separator byte", value)
			}
		}
	}
}

func TestReaderBounds(t *testing.T) {
	r := NewReader([]byte{1, 5, 'a', 'b'})

	if r.U8() != 1 {
		t.Fatal("wrong u8")
	}

	// Declares 5 bytes but only 2 remain
	if r.String() != "" || r.Err() == nil {
		t.Fatal("expected short buffer error")
	}

	// Errors are sticky
	if r.U32() != 0 || r.Err() != ErrShortBuffer {
		t.Fatal("expected sticky error")
	}
}
