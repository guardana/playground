package main

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTheServingCertificateVerifiesOnlyForItsSANs(t *testing.T) {
	now := time.Now()
	ca, err := newLabCA([]string{"pdp-double", "10.0.0.5"}, now)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := ca.issueServer([]string{"pdp-double", "10.0.0.5"}, now)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(leaf.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca.pem) {
		t.Fatal("the CA PEM holds no certificate")
	}
	opts := x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	for _, name := range []string{"pdp-double", "10.0.0.5"} {
		opts.DNSName = name
		if _, err := cert.Verify(opts); err != nil {
			t.Errorf("verifying for %s: %v", name, err)
		}
	}
	opts.DNSName = "victim-crm"
	if _, err := cert.Verify(opts); err == nil {
		t.Error("the certificate verified for a name it was not issued for")
	}
}

func TestOnlyTheCACertificateIsWritten(t *testing.T) {
	ca, err := newLabCA([]string{"pdp-double"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "pki")
	path := filepath.Join(dir, "pdp-ca.pem")
	if err := writeCA(path, ca.pem); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	block, rest := pem.Decode(written)
	if block == nil || block.Type != "CERTIFICATE" || len(strings.TrimSpace(string(rest))) != 0 {
		t.Fatalf("want exactly one CERTIFICATE block, got %q", written)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil || !cert.IsCA {
		t.Fatalf("the written certificate is not a CA: %v", err)
	}
	assertAloneAndReadable(t, dir, path)
}

func assertAloneAndReadable(t *testing.T, dir, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("mode %v, want 0644 for a reader under another uid", info.Mode().Perm())
	}
	listing, err := os.ReadDir(dir)
	if err != nil || len(listing) != 1 {
		t.Fatalf("the directory holds %d entries, want the certificate alone", len(listing))
	}
}
