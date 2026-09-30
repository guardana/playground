package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestTheCollectorsCertificateVerifiesUnderTheRunsCAForItsNameAlone(t *testing.T) {
	runDir := t.TempDir()
	now := time.Now()
	if err := writeCollectorTLS(runDir, now); err != nil {
		t.Fatalf("writeCollectorTLS: %v", err)
	}
	caPEM, err := os.ReadFile(filepath.Join(runDir, exportCADir, "ca.pem"))
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		t.Fatal("export-ca/ca.pem holds no certificate")
	}
	pair, err := tls.LoadX509KeyPair(filepath.Join(runDir, collectorTLSDir, "cert.pem"),
		filepath.Join(runDir, collectorTLSDir, "key.pem"))
	if err != nil {
		t.Fatalf("the collector's certificate and key do not pair: %v", err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	for name, wantOK := range map[string]bool{"collector": true, "enforcer": false, "victim-web": false} {
		_, err := leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: name, CurrentTime: now,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
		if (err == nil) != wantOK {
			t.Errorf("verifying for %q: %v, want ok=%v", name, err, wantOK)
		}
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: x509.NewCertPool(), DNSName: "collector", CurrentTime: now}); err == nil {
		t.Error("the certificate verified without the run's CA")
	}
}

func TestOnlyTheCACertificateReachesTheEnforcersDirectory(t *testing.T) {
	runDir := t.TempDir()
	if err := writeCollectorTLS(runDir, time.Now()); err != nil {
		t.Fatalf("writeCollectorTLS: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(runDir, exportCADir))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "ca.pem" {
		t.Errorf("export-ca/ holds %v, want ca.pem alone", entries)
	}
	for _, name := range []string{"cert.pem", "key.pem"} {
		info, err := os.Stat(filepath.Join(runDir, collectorTLSDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o644 {
			t.Errorf("%s has mode %v; the collector's uid could not read it", name, info.Mode().Perm())
		}
	}
}

// The CA's key is gone once the run's certificate is signed, and its
// constraints say what it could vouch for had it not been: the collector's name,
// nothing under it, no address, mail box or URI.
func TestTheRunsCAIsScopedToTheCollectorsName(t *testing.T) {
	runDir := t.TempDir()
	if err := writeCollectorTLS(runDir, time.Now()); err != nil {
		t.Fatalf("writeCollectorTLS: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(runDir, exportCADir, "ca.pem"))
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(body)
	if block == nil {
		t.Fatal("export-ca/ca.pem holds no PEM block")
	}
	ca, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	switch {
	case !ca.PermittedDNSDomainsCritical:
		t.Error("the name constraints are not critical")
	case !slices.Equal(ca.PermittedDNSDomains, []string{"collector"}):
		t.Errorf("permitted names %v, want collector alone", ca.PermittedDNSDomains)
	case !slices.Equal(ca.ExcludedDNSDomains, []string{".collector"}):
		t.Errorf("excluded names %v, want everything under collector", ca.ExcludedDNSDomains)
	case len(ca.ExcludedIPRanges) != 2 || len(ca.PermittedIPRanges) != 0:
		t.Errorf("address ranges permitted %v, excluded %v, want every address excluded", ca.PermittedIPRanges, ca.ExcludedIPRanges)
	case len(ca.ExcludedEmailAddresses) != 1 || len(ca.ExcludedURIDomains) != 1:
		t.Errorf("mail %v and URI %v constraints, want both excluded whole", ca.ExcludedEmailAddresses, ca.ExcludedURIDomains)
	}
}

func TestTLSMaterialIsNeverWrittenOverWhatIsThere(t *testing.T) {
	runDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(runDir, exportCADir), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := writeCollectorTLS(runDir, time.Now()); err == nil {
		t.Error("a directory already in the run directory was written into")
	}
}
