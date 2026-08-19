package realitykey

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestGenerate(t *testing.T) {
	privateText, publicText, err := Generate(bytes.NewReader(bytes.Repeat([]byte{0x42}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	privateKey, err := base64.RawURLEncoding.DecodeString(privateText)
	if err != nil || len(privateKey) != 32 {
		t.Fatalf("invalid private key: %q, %v", privateText, err)
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(publicText)
	if err != nil || len(publicKey) != 32 {
		t.Fatalf("invalid public key: %q, %v", publicText, err)
	}
}
