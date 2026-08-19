package realitykey

import (
	"crypto/ecdh"
	"encoding/base64"
	"fmt"
	"io"
)

func Generate(random io.Reader) (privateText, publicText string, err error) {
	key, err := ecdh.X25519().GenerateKey(random)
	if err != nil {
		return "", "", fmt.Errorf("generate X25519 key: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(key.Bytes()),
		base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), nil
}
