package certgen

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// EnsureCerts checks for cert.pem and key.pem in dir.
// If absent, it generates a self-signed 10-year certificate for localhost
// and all local network interfaces.
func EnsureCerts(dir string) error {
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")

	if fileExists(certFile) && fileExists(keyFile) {
		return nil
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("certgen: crear directorio %s: %w", dir, err)
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("certgen: generar clave: %w", err)
	}

	// Collect all local IP addresses for the SAN extension
	ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok {
				ips = append(ips, ipnet.IP)
			}
		}
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"ROC Modbus Expert"},
			CommonName:   "localhost",
		},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           ips,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return fmt.Errorf("certgen: crear certificado: %w", err)
	}

	// Write cert.pem
	cf, err := os.OpenFile(certFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("certgen: escribir cert.pem: %w", err)
	}
	pem.Encode(cf, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	cf.Close()

	// Write key.pem
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return fmt.Errorf("certgen: serializar clave: %w", err)
	}
	kf, err := os.OpenFile(keyFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("certgen: escribir key.pem: %w", err)
	}
	pem.Encode(kf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	kf.Close()

	fmt.Printf("certgen: certificado auto-firmado generado en %s/\n", dir)
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
