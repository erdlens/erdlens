package erdfile

import (
	"bytes"
	"os"
	"testing"
)

// TestRealWorldRoundTrip is skipped by default. Run with:
//
//	ERDLENS_ROUNDTRIP_FILE=/tmp/dbname.erd go test -run RealWorld ./internal/erdfile
func TestRealWorldRoundTrip(t *testing.T) {
	path := os.Getenv("ERDLENS_ROUNDTRIP_FILE")
	if path == "" {
		t.Skip("set ERDLENS_ROUNDTRIP_FILE to a real .erd file to run this test")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sc, err := Parse(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := Write(&buf, sc); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, buf.Bytes()) {
		t.Fatalf("round-trip differs (%d -> %d bytes)\nfirst 500 bytes of got:\n%s",
			len(data), len(buf.Bytes()), buf.String()[:min(500, len(buf.String()))])
	}
	t.Logf("round-trip OK: %d bytes, %d tables", len(data), len(sc.Tables))
}
