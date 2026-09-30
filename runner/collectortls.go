package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// The enforcer exports its trail to the collector over TLS: it accepts a
// plaintext collector only on a loopback IP literal, and the collector is
// another container. Each run makes a CA of its own that signs one certificate
// for the collector's name; the CA's key never leaves this process.
const (
	collectorTLSDir = "collector-tls"
	exportCADir     = "export-ca"
	collectorHost   = "collector"
	// certSkew keeps a certificate valid on a Docker VM whose clock trails the host's.
	certSkew     = time.Hour
	certLifetime = 24 * time.Hour
)

// writeCollectorTLS writes the collector's certificate and key into
// collector-tls/ and the CA certificate the enforcer trusts into export-ca/.
func writeCollectorTLS(runDir string, now time.Time) error {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	ca, err := certificate(scopedCA(), nil, &caKey.PublicKey, caKey, now)
	if err != nil {
		return err
	}
	parent, err := x509.ParseCertificate(ca)
	if err != nil {
		return err
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	leaf, err := certificate(&x509.Certificate{
		Subject:     pkix.Name{CommonName: collectorHost},
		DNSNames:    []string{collectorHost},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, parent, &leafKey.PublicKey, caKey, now)
	if err != nil {
		return err
	}
	private, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		return err
	}
	return writePEMs(runDir, map[string]pemFile{
		filepath.Join(exportCADir, "ca.pem"):       {"CERTIFICATE", ca},
		filepath.Join(collectorTLSDir, "cert.pem"): {"CERTIFICATE", leaf},
		filepath.Join(collectorTLSDir, "key.pem"):  {"PRIVATE KEY", private},
	})
}

// scopedCA is the CA's template, constrained to sign for the collector's name
// alone: nothing under it and no other name, address, mail box or URI.
func scopedCA() *x509.Certificate {
	return &x509.Certificate{
		Subject:                     pkix.Name{CommonName: "playground run CA"},
		IsCA:                        true,
		BasicConstraintsValid:       true,
		MaxPathLenZero:              true,
		KeyUsage:                    x509.KeyUsageCertSign,
		PermittedDNSDomainsCritical: true,
		PermittedDNSDomains:         []string{collectorHost},
		ExcludedDNSDomains:          []string{"." + collectorHost},
		ExcludedIPRanges: []*net.IPNet{
			{IP: net.IPv4zero.To4(), Mask: net.CIDRMask(0, 32)},
			{IP: net.IPv6zero, Mask: net.CIDRMask(0, 128)},
		},
		ExcludedEmailAddresses: []string{""},
		ExcludedURIDomains:     []string{""},
	}
}

type pemFile struct {
	kind  string
	bytes []byte
}

// certificate signs template for public with signer, as parent or, with no
// parent, as itself.
func certificate(template, parent *x509.Certificate, public any, signer *ecdsa.PrivateKey, now time.Time) ([]byte, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return nil, err
	}
	template.SerialNumber = serial
	template.NotBefore, template.NotAfter = now.Add(-certSkew), now.Add(certLifetime)
	if parent == nil {
		parent = template
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, public, signer)
	if err != nil {
		return nil, fmt.Errorf("signing the certificate for %s: %w", template.Subject.CommonName, err)
	}
	return der, nil
}

// writePEMs writes each file under runDir into a directory it makes. The files
// are world-readable: the collector reads its key as its own uid, and each
// directory is mounted into one service only. A directory or file already
// there is refused, since the run directory is writable by every service.
func writePEMs(runDir string, files map[string]pemFile) error {
	for _, dir := range []string{exportCADir, collectorTLSDir} {
		path := filepath.Join(runDir, dir)
		if err := os.Mkdir(path, reportsMode); err != nil { // #nosec G301,G703 -- inside the run directory the runner made.
			return err
		}
		if err := os.Chmod(path, reportsMode); err != nil { // #nosec G302,G703 -- a umask must not hide it from the service.
			return err
		}
	}
	for name, file := range files {
		if err := writeNew(filepath.Join(runDir, name), pem.EncodeToMemory(&pem.Block{Type: file.kind, Bytes: file.bytes})); err != nil {
			return err
		}
	}
	return nil
}

func writeNew(path string, body []byte) error {
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) // #nosec G302,G304,G703 -- a key for this run's collector alone; see above.
	if err != nil {
		return err
	}
	if _, err := out.Write(body); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(path, 0o644) // #nosec G302,G703 -- as above.
}

// dropCollectorKey removes the collector's key once the run's services are
// down. It served that one run, and a key left in the reports would outlive it.
func dropCollectorKey(runDir string) error {
	err := os.Remove(filepath.Join(runDir, collectorTLSDir, "key.pem")) // #nosec G703 -- inside the run directory the runner made.
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
