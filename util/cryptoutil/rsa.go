package cryptoutil

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"time"
)

func generateRSAKeyPair(bits int) *rsa.PrivateKey {
	ret, _ := rsa.GenerateKey(rand.Reader, bits)
	return ret
}

func Generate3072RSAKeyPair() *rsa.PrivateKey {
	return generateRSAKeyPair(3072)
}

func Generate2048RSAKeyPair() *rsa.PrivateKey {
	return generateRSAKeyPair(2048)
}

func Generate1024RSAKeyPair() *rsa.PrivateKey {
	return generateRSAKeyPair(1024)
}

type rsaOAEP struct {
	pri   *rsa.PrivateKey
	label []byte
}

func NewRsaOAEP(key *rsa.PrivateKey, label ...[]byte) (Crypto, error) {
	if key == nil {
		return nil, errors.New("empty privateKey")
	}
	if label == nil || len(label) == 0 {
		label = [][]byte{[]byte("")}
	}
	return &rsaOAEP{
		pri:   key,
		label: label[0],
	}, nil
}

// Encrypt 分段加密
func (r *rsaOAEP) Encrypt(messageBytes []byte) ([]byte, error) {
	maxLength := r.pri.PublicKey.Size() - 2*sha256.Size - 2
	var ciphertext []byte
	for len(messageBytes) > 0 {
		chunkSize := maxLength
		if len(messageBytes) < maxLength {
			chunkSize = len(messageBytes)
		}
		chunk := messageBytes[:chunkSize]
		messageBytes = messageBytes[chunkSize:]
		chunkCiphertext, err := rsa.EncryptOAEP(
			sha256.New(),
			rand.Reader,
			&r.pri.PublicKey,
			chunk,
			r.label,
		)
		if err != nil {
			return nil, err
		}
		ciphertext = append(ciphertext, chunkCiphertext...)
	}
	return ciphertext, nil
}

// Decrypt 分段解密
func (r *rsaOAEP) Decrypt(messageBytes []byte) ([]byte, error) {
	maxLength := r.pri.Size()
	var ciphertext []byte
	for len(messageBytes) > 0 {
		chunkSize := maxLength
		if len(messageBytes) < maxLength {
			chunkSize = len(messageBytes)
		}
		chunk := messageBytes[:chunkSize]
		messageBytes = messageBytes[chunkSize:]
		chunkCiphertext, err := rsa.DecryptOAEP(
			sha256.New(),
			rand.Reader,
			r.pri,
			chunk,
			r.label,
		)
		if err != nil {
			return nil, err
		}
		ciphertext = append(ciphertext, chunkCiphertext...)
	}
	return ciphertext, nil
}

type rsaPKCS1v15 struct {
	pri *rsa.PrivateKey
}

func NewRsaPKCS1v15(key *rsa.PrivateKey) (Crypto, error) {
	if key == nil {
		return nil, errors.New("empty privateKey")
	}
	return &rsaPKCS1v15{
		pri: key,
	}, nil
}

func (r *rsaPKCS1v15) Encrypt(messageBytes []byte) ([]byte, error) {
	return EncryptPKCS1v15ByPublicKey(&r.pri.PublicKey, messageBytes)
}

func EncryptPKCS1v15ByPublicKey(key *rsa.PublicKey, messageBytes []byte) ([]byte, error) {
	if key == nil {
		return nil, errors.New("empty cipher key")
	}
	if messageBytes == nil {
		return nil, errors.New("empty message")
	}
	maxLength := key.Size() - 11
	var ciphertext []byte
	for len(messageBytes) > 0 {
		chunkSize := maxLength
		if len(messageBytes) < maxLength {
			chunkSize = len(messageBytes)
		}
		chunk := messageBytes[:chunkSize]
		messageBytes = messageBytes[chunkSize:]
		chunkCiphertext, err := rsa.EncryptPKCS1v15(
			rand.Reader,
			key,
			chunk,
		)
		if err != nil {
			return nil, err
		}
		ciphertext = append(ciphertext, chunkCiphertext...)
	}
	return ciphertext, nil
}

func DecryptPKCS1v15ByPrivateKey(key *rsa.PrivateKey, messageBytes []byte) ([]byte, error) {
	if key == nil {
		return nil, errors.New("empty cipher key")
	}
	if messageBytes == nil {
		return nil, errors.New("empty message")
	}
	maxLength := key.Size()
	var ciphertext []byte
	for len(messageBytes) > 0 {
		chunkSize := maxLength
		if len(messageBytes) < maxLength {
			chunkSize = len(messageBytes)
		}
		chunk := messageBytes[:chunkSize]
		messageBytes = messageBytes[chunkSize:]
		chunkCiphertext, err := rsa.DecryptPKCS1v15(
			rand.Reader,
			key,
			chunk,
		)
		if err != nil {
			return nil, err
		}
		ciphertext = append(ciphertext, chunkCiphertext...)
	}
	return ciphertext, nil
}

func (r *rsaPKCS1v15) Decrypt(messageBytes []byte) ([]byte, error) {
	return DecryptPKCS1v15ByPrivateKey(r.pri, messageBytes)
}

func DecodePemPrivateKey(pemPrivateKey string) (*rsa.PrivateKey, error) {
	if pemPrivateKey == "" {
		return nil, errors.New("empty private key")
	}
	block, _ := pem.Decode([]byte(pemPrivateKey))
	if block == nil {
		return nil, errors.New("PrivateKey format error")
	}
	priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		oldErr := err
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, errors.New(fmt.Sprintf("ParsePKCS1PrivateKey error: %s, ParsePKCS8PrivateKey error: %s", oldErr.Error(), err.Error()))
		}
		switch t := key.(type) {
		case *rsa.PrivateKey:
			priv = key.(*rsa.PrivateKey)
		default:
			return nil, errors.New(fmt.Sprintf("ParsePKCS1PrivateKey error: %s, ParsePKCS8PrivateKey error: Not supported privatekey format, should be *rsa.PrivateKey, got %T", oldErr.Error(), t))
		}
	}
	return priv, nil
}

func DecodePemCA(pemCa string) (*x509.Certificate, error) {
	// Parse the certificate from the PEM data
	certBlock, _ := pem.Decode([]byte(pemCa))
	if certBlock == nil {
		return nil, errors.New("parse ca failed")
	}
	return x509.ParseCertificate(certBlock.Bytes)
}

func DecodePemPublicKey(pemPublicKey string) (*rsa.PublicKey, error) {
	if pemPublicKey == "" {
		return nil, errors.New("empty private key")
	}
	block, _ := pem.Decode([]byte(pemPublicKey))
	if block == nil {
		return nil, errors.New("PrivateKey format error")
	}
	priv, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		oldErr := err
		key, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, errors.New(fmt.Sprintf("ParsePKCS1PublicKey error: %s, ParsePKIXPublicKey error: %s", oldErr.Error(), err.Error()))
		}
		switch t := key.(type) {
		case *rsa.PublicKey:
			priv = key.(*rsa.PublicKey)
		default:
			return nil, errors.New(fmt.Sprintf("ParsePKCS1PrivateKey error: %s, ParsePKCS8PrivateKey error: Not supported privatekey format, should be *rsa.PrivateKey, got %T", oldErr.Error(), t))
		}
	}
	return priv, nil
}

func GeneratePKCS1PrivateKeyPem(key *rsa.PrivateKey) (string, error) {
	if key == nil {
		return "", errors.New("empty private key")
	}
	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}
	ret := pem.EncodeToMemory(block)
	return string(ret), nil
}

func GeneratePemPKCS8PrivateKeyPem(key *rsa.PrivateKey) (string, error) {
	if key == nil {
		return "", errors.New("empty private key")
	}
	pk, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", err
	}
	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: pk,
	}
	ret := pem.EncodeToMemory(block)
	return string(ret), nil
}

func GeneratePKCS1PublicKeyPem(key *rsa.PrivateKey) (string, error) {
	if key == nil {
		return "", errors.New("empty private key")
	}
	block := &pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey),
	}
	ret := pem.EncodeToMemory(block)
	return string(ret), nil
}

func GeneratePemPKIXPublicKeyPem(key *rsa.PrivateKey) (string, error) {
	if key == nil {
		return "", errors.New("empty private key")
	}
	pk, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", err
	}
	block := &pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: pk,
	}
	ret := pem.EncodeToMemory(block)
	return string(ret), nil
}

func GenerateCAPem(key *rsa.PrivateKey, subject pkix.Name, expireTime time.Time) (string, error) {
	if key == nil {
		return "", errors.New("empty private key")
	}
	// Define the certificate template
	now := time.Now()
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               subject,
		NotBefore:             now,
		NotAfter:              expireTime,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	caCertBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return "", err
	}
	// Encode the certificate and private key to PEM format
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertBytes})), nil
}
