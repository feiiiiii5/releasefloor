package floor

import "testing"

func FuzzInspect(f *testing.F) {
	f.Add([]byte("\x7fELF\x02\x01\x01"))
	f.Add([]byte{0xcf, 0xfa, 0xed, 0xfe})
	f.Add([]byte{0xca, 0xfe, 0xba, 0xbe})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		bins, err := Inspect(data, "fuzz-input")
		if err == nil && len(bins) == 0 {
			t.Fatal("successful inspection with no binaries")
		}
		for _, b := range bins {
			if err == nil && (b.Member != "fuzz-input" || len(b.SHA256) != 64 || b.Format == "") {
				t.Fatal("successful inspection lost identity or hash")
			}
		}
	})
}

func FuzzPolicy(f *testing.F) {
	f.Add([]byte(`{"schema":"releasefloor.policy.v1","rules":[{"platform":"elf","arch":"amd64","max_floors":{"GLIBC":"2.35"}}]}`))
	f.Add([]byte(`{"schema":"x","schema":"y"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<16 {
			t.Skip()
		}
		p, err := ParsePolicy(data)
		if err == nil {
			if err := ValidatePolicy(p); err != nil {
				t.Fatal("parsed policy does not validate:", err)
			}
		}
	})
}
