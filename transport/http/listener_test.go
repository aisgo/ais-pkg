package http

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

func writeSelfSignedCert(t *testing.T) (certFile, keyFile, caFile string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	dir := t.TempDir()
	certFile = filepath.Join(dir, "server.pem")
	keyFile = filepath.Join(dir, "server.key")
	caFile = filepath.Join(dir, "client-ca.pem")

	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if err := os.WriteFile(caFile, certPEM, 0o600); err != nil {
		t.Fatalf("write ca: %v", err)
	}

	return certFile, keyFile, caFile
}

func TestBuildListenerTLSConfigLoadsClientCA(t *testing.T) {
	certFile, keyFile, caFile := writeSelfSignedCert(t)

	cfg, err := buildListenerTLSConfig(fiber.ListenConfig{
		CertFile:       certFile,
		CertKeyFile:    keyFile,
		CertClientFile: caFile,
	})
	if err != nil {
		t.Fatalf("build listener tls config: %v", err)
	}

	if cfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("expected mTLS client auth to be enabled")
	}
	if cfg.ClientCAs == nil {
		t.Fatalf("expected client CA pool to be configured")
	}
}

func TestBuildListenerTLSConfigRejectsInvalidClientCA(t *testing.T) {
	certFile, keyFile, _ := writeSelfSignedCert(t)
	badCAFile := filepath.Join(t.TempDir(), "bad-client-ca.pem")
	if err := os.WriteFile(badCAFile, []byte("not a pem"), 0o600); err != nil {
		t.Fatalf("write invalid ca file: %v", err)
	}

	if _, err := buildListenerTLSConfig(fiber.ListenConfig{
		CertFile:       certFile,
		CertKeyFile:    keyFile,
		CertClientFile: badCAFile,
	}); err == nil {
		t.Fatalf("expected invalid client ca error")
	}
}
