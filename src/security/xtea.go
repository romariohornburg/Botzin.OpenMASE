package security

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"

	"github.com/iagoMAO/Botzin.OpenMASE/utils"
	"github.com/rs/zerolog/log"
)

// Pad packet data to ensure it's a multiple of 8.
func pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padded := bytes.Repeat([]byte{byte(padding)}, padding)

	return append(data, padded...)
}

// Unpad packet data for decryption.
func unpad(data []byte) []byte {
	// No need to unpad
	if len(data)%8 == 0 {
		return data
	}

	length := len(data)
	unpadded := int(data[length-1])

	return data[:(length - unpadded)]
}

func decryptBlock(data, key []byte) []byte {
	v0 := binary.LittleEndian.Uint32(data[0:4])
	v1 := binary.LittleEndian.Uint32(data[4:8])

	k0 := binary.LittleEndian.Uint32(key[0:4])
	k1 := binary.LittleEndian.Uint32(key[4:8])
	k2 := binary.LittleEndian.Uint32(key[8:12])
	k3 := binary.LittleEndian.Uint32(key[12:16])

	delta := uint32(0x61C88647)
	sum := uint32(0) - (32 * delta)

	for range 32 {
		v1 -= (sum + v0) ^ (k3 + (v0 >> 5)) ^ (k2 + (v0 << 4))
		v0 -= (sum + v1) ^ (k1 + (v1 >> 5)) ^ (k0 + (v1 << 4))
		sum += delta
	}

	out := make([]byte, 8)
	binary.LittleEndian.PutUint32(out[0:4], v0)
	binary.LittleEndian.PutUint32(out[4:8], v1)

	return out
}

func encryptBlock(data, key []byte) []byte {
	v0 := binary.LittleEndian.Uint32(data[0:4])
	v1 := binary.LittleEndian.Uint32(data[4:8])

	k0 := binary.LittleEndian.Uint32(key[0:4])
	k1 := binary.LittleEndian.Uint32(key[4:8])
	k2 := binary.LittleEndian.Uint32(key[8:12])
	k3 := binary.LittleEndian.Uint32(key[12:16])

	delta := uint32(0x61C88647)
	sum := uint32(0)

	for range 32 {
		sum -= delta
		v0 += (sum + v1) ^ (k1 + (v1 >> 5)) ^ (k0 + (v1 << 4))
		v1 += (sum + v0) ^ (k3 + (v0 >> 5)) ^ (k2 + (v0 << 4))
	}

	out := make([]byte, 8)
	binary.LittleEndian.PutUint32(out[0:4], v0)
	binary.LittleEndian.PutUint32(out[4:8], v1)

	return out
}

func getKey() []byte {
	key, err := hex.DecodeString(utils.GetConfig().XTEA_KEY)

	if err != nil || len(key) != 16 {
		log.Error().Msgf("XTEA failed to load the cipher key (expected 32 hex chars): %v", err)
		return nil
	}

	return key
}

func EncryptXTEA(data []byte) []byte {
	key := getKey()

	if key == nil {
		return nil
	}

	paddedData := pad(data, 8)

	out := []byte(nil)

	// Iterate through data by blocks
	for i := 0; i < len(paddedData); i += 8 {
		out = append(out, encryptBlock(paddedData[i:i+8], key)...)
	}

	return out
}

func DecryptXTEA(data []byte) []byte {
	key := getKey()

	// XTEA works on 8 byte blocks, anything else is malformed
	if key == nil || len(data)%8 != 0 {
		return nil
	}

	out := []byte(nil)

	// Iterate through data by blocks
	for i := 0; i < len(data); i += 8 {
		out = append(out, decryptBlock(data[i:i+8], key)...)
	}

	return unpad(out)
}
