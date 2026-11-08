package api

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"konjiyami/server/assets"
)

// ensureTLSCerts generates a self-signed certificate pair if they don't exist.
func ensureTLSCerts() (certFile, keyFile string, err error) {
	certsDir := filepath.Join(assets.GetRootAppDir(), "certs")
	os.MkdirAll(certsDir, 0700)

	certFile = filepath.Join(certsDir, "api-cert.pem")
	keyFile = filepath.Join(certsDir, "api-key.pem")

	if _, e1 := os.Stat(certFile); e1 == nil {
		if _, e2 := os.Stat(keyFile); e2 == nil {
			return certFile, keyFile, nil
		}
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", fmt.Errorf("generate serial: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"KonjiYami C2"},
			CommonName:   "KonjiYami API",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		DNSNames:              []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return "", "", fmt.Errorf("create certificate: %w", err)
	}

	certOut, err := os.Create(certFile)
	if err != nil {
		return "", "", fmt.Errorf("create cert file: %w", err)
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	certOut.Close()

	keyOut, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return "", "", fmt.Errorf("create key file: %w", err)
	}
	privDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return "", "", fmt.Errorf("marshal key: %w", err)
	}
	pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER})
	keyOut.Close()

	apiLog.Infof("Generated self-signed TLS cert: %s", certFile)
	return certFile, keyFile, nil
}

// createTLSListener returns a TLS-wrapped net.Listener using auto-generated certs.
func createTLSListener(addr string) (net.Listener, error) {
	certFile, keyFile, err := ensureTLSCerts()
	if err != nil {
		return nil, fmt.Errorf("tls setup: %w", err)
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load cert pair: %w", err)
	}
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	return tls.Listen("tcp", addr, tlsConfig)
}
