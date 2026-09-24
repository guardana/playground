package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/url"
	"testing"
	"time"
)

func TestTheCASignsForItsOwnNamesAndNothingElse(t *testing.T) {
	foreign := []string{"otel-collector", "victim-crm", "idp.example.com", "sub.pdp-double", "evil.lab.test", "10.0.0.9", "::2"}
	for name, sans := range map[string][]string{
		"names and an IP":   {"pdp-double", "10.0.0.5"},
		"names only":        {"pdp-double"},
		"IPs only":          {"127.0.0.1", "::1"},
		"two sibling names": {"pdp.lab.test", "decide.lab.test"},
	} {
		t.Run(name, func(t *testing.T) {
			now := time.Now()
			ca, err := newLabCA(sans, now)
			if err != nil {
				t.Fatal(err)
			}
			own, err := ca.issueServer(sans, now)
			if err != nil {
				t.Fatal(err)
			}
			leaf, err := x509.ParseCertificate(own.Certificate[0])
			if err != nil {
				t.Fatal(err)
			}
			for _, san := range sans {
				if err := verifyUnder(ca, leaf, san, x509.ExtKeyUsageServerAuth); err != nil {
					t.Errorf("the double's own leaf does not verify for %s: %v", san, err)
				}
			}
			for _, other := range foreign {
				err := verifyUnder(ca, signedBy(t, ca, other, x509.ExtKeyUsageServerAuth), other, x509.ExtKeyUsageServerAuth)
				assertRefusedFor(t, "a leaf for "+other, err, x509.CANotAuthorizedForThisName)
			}
			err = verifyUnder(ca, signedBy(t, ca, sans[0], x509.ExtKeyUsageClientAuth), sans[0], x509.ExtKeyUsageClientAuth)
			assertRefusedFor(t, "a clientAuth leaf", err, x509.IncompatibleUsage)
		})
	}
}

func TestTheCAIsConstrainedAndShortLived(t *testing.T) {
	now := time.Now()
	ca, err := newLabCA([]string{"pdp-double", "10.0.0.5"}, now)
	if err != nil {
		t.Fatal(err)
	}
	own, err := ca.issueServer([]string{"pdp-double", "10.0.0.5"}, now)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(own.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(ca.cert.Raw)
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.PermittedDNSDomainsCritical {
		t.Error("the CA's name constraints are not critical")
	}
	if len(parsed.ExtKeyUsage) != 1 || parsed.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Errorf("the CA's extended key usage is %v, want serverAuth alone", parsed.ExtKeyUsage)
	}
	for which, cert := range map[string]*x509.Certificate{"CA": parsed, "leaf": leaf} {
		if life := cert.NotAfter.Sub(now); life > 24*time.Hour || life < time.Hour {
			t.Errorf("the %s lives %v past now, want hours", which, life)
		}
	}
}

func TestTheCASignsNoMailboxURIOrIntermediate(t *testing.T) {
	now := time.Now()
	ca, err := newLabCA([]string{"pdp-double"}, now)
	if err != nil {
		t.Fatal(err)
	}
	mailbox := signedBy(t, ca, "pdp-double", x509.ExtKeyUsageServerAuth, func(c *x509.Certificate) {
		c.EmailAddresses = []string{"ops@pdp-double"}
	})
	assertRefusedFor(t, "a leaf naming a mailbox", verifyUnder(ca, mailbox, "pdp-double", x509.ExtKeyUsageServerAuth),
		x509.CANotAuthorizedForThisName)
	withURI := signedBy(t, ca, "pdp-double", x509.ExtKeyUsageServerAuth, func(c *x509.Certificate) {
		c.URIs = []*url.URL{{Scheme: "spiffe", Host: "evil", Path: "/x"}}
	})
	assertRefusedFor(t, "a leaf naming a URI", verifyUnder(ca, withURI, "pdp-double", x509.ExtKeyUsageServerAuth),
		x509.CANotAuthorizedForThisName)
	assertRefusedFor(t, "a leaf under an intermediate", verifyThroughIntermediate(t, ca), x509.TooManyIntermediates)
}

// verifyThroughIntermediate issues an intermediate CA under the double's CA and
// a leaf for the double's own name under that, and verifies the leaf.
func verifyThroughIntermediate(t *testing.T, ca *labCA) error {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "intermediate"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	middle, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	leaf := signedBy(t, &labCA{cert: middle, key: key}, "pdp-double", x509.ExtKeyUsageServerAuth)
	roots, intermediates := x509.NewCertPool(), x509.NewCertPool()
	roots.AddCert(ca.cert)
	intermediates.AddCert(middle)
	_, err = leaf.Verify(x509.VerifyOptions{
		DNSName: "pdp-double", Roots: roots, Intermediates: intermediates,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	return err
}

func verifyUnder(ca *labCA, leaf *x509.Certificate, name string, usage x509.ExtKeyUsage) error {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca.pem) {
		return errors.New("the CA PEM holds no certificate")
	}
	_, err := leaf.Verify(x509.VerifyOptions{Roots: pool, DNSName: name, KeyUsages: []x509.ExtKeyUsage{usage}})
	return err
}

// signedBy is a leaf for name that the CA's key signs, as anyone holding that
// key could; the CA's own constraints decide whether it verifies.
func signedBy(t *testing.T, ca *labCA, name string, usage x509.ExtKeyUsage, edits ...func(*x509.Certificate)) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
	}
	if ip := net.ParseIP(name); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{name}
	}
	for _, edit := range edits {
		edit(template)
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func assertRefusedFor(t *testing.T, what string, err error, reason x509.InvalidReason) {
	t.Helper()
	var invalid x509.CertificateInvalidError
	if !errors.As(err, &invalid) || invalid.Reason != reason {
		t.Errorf("%s: verification returned %v, want it refused with reason %d", what, err, reason)
	}
}
