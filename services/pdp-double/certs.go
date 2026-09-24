package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// certLifetime outlasts a run and a session driven by hand; a certificate
// carried out of the lab is dead within the day.
const certLifetime = 8 * time.Hour

// labCA is a certificate authority that exists only in this process's memory.
type labCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  []byte
}

func newLabCA(sans []string, now time.Time) (*labCA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	template, err := certTemplate("playground pdp-double CA", now)
	if err != nil {
		return nil, err
	}
	template.IsCA = true
	template.BasicConstraintsValid = true
	template.MaxPathLenZero = true
	template.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	scopeTo(template, sans)
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &labCA{cert: cert, key: key, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}, nil
}

// scopeTo constrains the CA to sign for exactly sans: each name, nothing under
// it, and no other name or address. A kind left without a constraint would
// permit every name of that kind, so an absent kind is excluded whole.
func scopeTo(ca *x509.Certificate, sans []string) {
	ca.PermittedDNSDomainsCritical = true
	var names []string
	for _, san := range sans {
		ip := net.ParseIP(san)
		switch {
		case ip == nil:
			names = append(names, san)
		case ip.To4() != nil:
			ca.PermittedIPRanges = append(ca.PermittedIPRanges, &net.IPNet{IP: ip.To4(), Mask: net.CIDRMask(32, 32)})
		default:
			ca.PermittedIPRanges = append(ca.PermittedIPRanges, &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)})
		}
	}
	ca.PermittedDNSDomains = names
	for _, name := range names {
		ca.ExcludedDNSDomains = append(ca.ExcludedDNSDomains, "."+name)
	}
	ca.ExcludedEmailAddresses = []string{""}
	ca.ExcludedURIDomains = []string{""}
	if len(names) == 0 {
		ca.ExcludedDNSDomains = []string{""}
	}
	if len(ca.PermittedIPRanges) == 0 {
		ca.ExcludedIPRanges = []*net.IPNet{
			{IP: net.IPv4zero.To4(), Mask: net.CIDRMask(0, 32)},
			{IP: net.IPv6zero, Mask: net.CIDRMask(0, 128)},
		}
	}
}

// issueServer makes the serving certificate for sans, each a DNS name or an IP
// address, signed by the CA. Its key stays in the returned value.
func (ca *labCA) issueServer(sans []string, now time.Time) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	template, err := certTemplate(sans[0], now)
	if err != nil {
		return tls.Certificate{}, err
	}
	template.KeyUsage = x509.KeyUsageDigitalSignature
	template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	addSANs(template, sans)
	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der, ca.cert.Raw}, PrivateKey: key}, nil
}

// addSANs files each of sans as an IP address or a DNS name.
func addSANs(cert *x509.Certificate, sans []string) {
	for _, san := range sans {
		if ip := net.ParseIP(san); ip != nil {
			cert.IPAddresses = append(cert.IPAddresses, ip)
		} else {
			cert.DNSNames = append(cert.DNSNames, san)
		}
	}
}

// coversHost reports whether a certificate issued for sans verifies for host,
// by the same check control's TLS client makes.
func coversHost(sans []string, host string) bool {
	cert := &x509.Certificate{}
	addSANs(cert, sans)
	return cert.VerifyHostname(host) == nil
}

func certTemplate(commonName string, now time.Time) (*x509.Certificate, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return nil, err
	}
	return &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName, Organization: []string{"playground lab"}},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(certLifetime),
	}, nil
}

// writeCA writes the CA certificate through a rename, so a reader waiting on
// the path never sees half a file.
func writeCA(path string, certPEM []byte) error {
	dir := filepath.Dir(path)
	// The gateway reads the certificate under another uid.
	if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 -- holds only public material.
		return err
	}
	tmp, err := os.CreateTemp(dir, ".pdp-double-ca-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(certPEM); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil { // #nosec G302 -- a CA certificate is public.
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
