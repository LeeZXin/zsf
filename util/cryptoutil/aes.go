package cryptoutil

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
)

type aesCTR struct {
	block cipher.Block
}

func NewAesCTR(key []byte) (Crypto, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return &aesCTR{block: block}, nil
}

func (a *aesCTR) Encrypt(plaintext []byte) ([]byte, error) {
	ciphertext := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext[:aes.BlockSize]
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}
	stream := cipher.NewCTR(a.block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], plaintext)

	return ciphertext, nil
}

func (a *aesCTR) Decrypt(ciphertext []byte) ([]byte, error) {
	iv := ciphertext[:aes.BlockSize]
	plaintext := make([]byte, len(ciphertext)-aes.BlockSize)
	stream := cipher.NewCTR(a.block, iv)
	stream.XORKeyStream(plaintext, ciphertext[aes.BlockSize:])
	return plaintext, nil
}

type aesCBC struct {
	block cipher.Block
}

func NewAesCBC(key []byte) (Crypto, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return &aesCBC{block: block}, nil
}

func (a *aesCBC) Encrypt(data []byte) ([]byte, error) {
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}
	mode := cipher.NewCBCEncrypter(a.block, iv)
	// Encrypt the data
	paddedData := a.padData(data, aes.BlockSize)
	encryptedData := make([]byte, len(paddedData))
	mode.CryptBlocks(encryptedData, paddedData)
	// Return the IV and encrypted data
	return append(iv, encryptedData...), nil
}

func (a *aesCBC) Decrypt(encryptedData []byte) ([]byte, error) {
	// Extract IV and encrypted data
	iv := encryptedData[:aes.BlockSize]
	encryptedData = encryptedData[aes.BlockSize:]
	mode := cipher.NewCBCDecrypter(a.block, iv)
	// Decrypt the data
	decryptedData := make([]byte, len(encryptedData))
	mode.CryptBlocks(decryptedData, encryptedData)
	decryptedData = a.unpadData(decryptedData)
	return decryptedData, nil
}

func (a *aesCBC) padData(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padtext...)
}

func (a *aesCBC) unpadData(data []byte) []byte {
	padding := int(data[len(data)-1])
	return data[:len(data)-padding]
}
