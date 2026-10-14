// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package acme

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tailscale.com/ipn/store/mem"
	xacme "tailscale.com/tempfork/acme"
	"tailscale.com/types/logger"
)

// fakeACMEServer is the smallest RFC 8555 CA that issueACMECert can talk
// to: a directory, a nonce on every response, an account lookup, one
// order with one authorization and one tls-alpn-01 challenge, and the
// finalize and certificate endpoints. Challenge validation is not
// performed; the test decides what the authorization's status becomes
// after Accept. Modeled on acmeServer in tempfork/acme/rfc8555_test.go.
type fakeACMEServer struct {
	ts *httptest.Server

	// afterAccept is the authorization status reported once the client
	// has accepted the challenge: "valid" or "invalid".
	afterAccept string
	// challengeError is the problem attached to the challenge when
	// afterAccept is "invalid".
	challengeError string

	mu       sync.Mutex
	nonce    int
	accepted bool
	// polls counts POST-as-GET requests per path, so a test can tell
	// which endpoint surfaced a failure.
	polls map[string]int

	ca      *testCA
	certPEM []byte // leaf then root, issued at finalize and served from /cert/1
}

func newFakeACMEServer(t *testing.T, domain string) *fakeACMEServer {
	t.Helper()
	s := &fakeACMEServer{polls: map[string]int{}, afterAccept: "valid"}
	s.ca = newTestCA(t)
	s.ts = httptest.NewServer(http.HandlerFunc(s.serveHTTP))
	t.Cleanup(s.ts.Close)
	return s
}

func (s *fakeACMEServer) url(path string) string { return s.ts.URL + path }

func (s *fakeACMEServer) directoryURL() string { return s.url("/") }

func (s *fakeACMEServer) pollCount(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.polls[path]
}

func (s *fakeACMEServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/" {
		fmt.Fprintf(w, `{"newNonce": %q, "newAccount": %q, "newOrder": %q}`,
			s.url("/new-nonce"), s.url("/new-account"), s.url("/new-order"))
		return
	}
	s.mu.Lock()
	s.nonce++
	w.Header().Set("Replay-Nonce", fmt.Sprintf("nonce%d", s.nonce))
	s.polls[r.URL.Path]++
	accepted := s.accepted
	s.mu.Unlock()

	authz := func() string {
		status := "pending"
		var chalErr string
		if accepted {
			status = s.afterAccept
			if status == "invalid" {
				chalErr = fmt.Sprintf(`, "error": {"type": "urn:ietf:params:acme:error:unauthorized", "status": 403, "detail": %q}`, s.challengeError)
			}
		}
		return fmt.Sprintf(`{"status": %q, "identifier": {"type": "dns", "value": "example.com"},
			"challenges": [{"type": "tls-alpn-01", "url": %q, "token": "tok", "status": %q%s}]}`,
			status, s.url("/chal/1"), status, chalErr)
	}
	switch r.URL.Path {
	case "/new-nonce":
	case "/new-account":
		w.Header().Set("Location", s.url("/acct/1"))
		fmt.Fprint(w, `{"status": "valid"}`)
	case "/new-order":
		w.Header().Set("Location", s.url("/order/1"))
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"status": "pending", "authorizations": [%q], "finalize": %q}`, s.url("/authz/1"), s.url("/finalize/1"))
	case "/authz/1":
		fmt.Fprint(w, authz())
	case "/chal/1":
		s.mu.Lock()
		s.accepted = true
		s.mu.Unlock()
		fmt.Fprintf(w, `{"type": "tls-alpn-01", "url": %q, "token": "tok", "status": "processing"}`, s.url("/chal/1"))
	case "/order/1":
		status := "ready"
		if accepted && s.afterAccept == "invalid" {
			status = "invalid"
		}
		fmt.Fprintf(w, `{"status": %q, "authorizations": [%q], "finalize": %q, "certificate": %q}`,
			status, s.url("/authz/1"), s.url("/finalize/1"), s.url("/cert/1"))
	case "/finalize/1":
		body, _ := io.ReadAll(r.Body)
		csr, err := csrFromJWS(body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"detail": "bad CSR: " + err.Error()})
			return
		}
		chain, err := s.ca.issue(csr)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"detail": "issue: " + err.Error()})
			return
		}
		s.mu.Lock()
		s.certPEM = chain
		s.mu.Unlock()
		w.Header().Set("Location", s.url("/order/1"))
		fmt.Fprintf(w, `{"status": "valid", "authorizations": [%q], "finalize": %q, "certificate": %q}`,
			s.url("/authz/1"), s.url("/finalize/1"), s.url("/cert/1"))
	case "/cert/1":
		s.mu.Lock()
		chain := s.certPEM
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/pem-certificate-chain")
		w.Write(chain)
	default:
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"detail": "unhandled " + r.URL.Path})
	}
}

// testCA is a throwaway root CA for the fake server to issue from.
type testCA struct {
	key  *ecdsa.PrivateKey
	cert *x509.Certificate
	pem  []byte
}

func newTestCA(t *testing.T) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "fake ACME root"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &testCA{key: key, cert: cert, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

// issue returns a PEM chain (leaf then root) for the CSR's domain and key.
func (ca *testCA) issue(csr *x509.CertificateRequest) ([]byte, error) {
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      csr.Subject,
		DNSNames:     csr.DNSNames,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, csr.PublicKey, ca.key)
	if err != nil {
		return nil, err
	}
	return append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), ca.pem...), nil
}

// csrFromJWS extracts the CSR from a finalize request body, which is a JWS
// whose payload is {"csr": <base64url DER>}. Signatures are not checked.
func csrFromJWS(body []byte) (*x509.CertificateRequest, error) {
	var jws struct {
		Payload string `json:"payload"`
	}
	if err := json.Unmarshal(body, &jws); err != nil {
		return nil, err
	}
	payload, err := base64.RawURLEncoding.DecodeString(jws.Payload)
	if err != nil {
		return nil, err
	}
	var req struct {
		CSR string `json:"csr"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	der, err := base64.RawURLEncoding.DecodeString(req.CSR)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificateRequest(der)
}

func newTestACMEClient(t *testing.T, s *fakeACMEServer) *xacme.Client {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &xacme.Client{Key: key, DirectoryURL: s.directoryURL()}
}

// TestIssueACMECertSurfacesAuthorizationError checks that when the CA
// marks the authorization invalid, the error returned to the caller is
// the authorization's problem detail, not just the order's status.
// See tailscale/tailscale#21759.
func TestIssueACMECertSurfacesAuthorizationError(t *testing.T) {
	const domain = "example.com"
	s := newFakeACMEServer(t, domain)
	s.afterAccept = "invalid"
	s.challengeError = `Incorrect TXT record "abc" (and 29 more) found at _acme-challenge.example.com`

	e := &extension{logf: t.Logf}
	var logged atomic.Bool
	logf := func(format string, args ...any) {
		if strings.Contains(fmt.Sprintf(format, args...), "WaitAuthorization") {
			logged.Store(true)
		}
		t.Logf(format, args...)
	}
	_, err := e.issueACMECert(context.Background(), nil, newTestACMEClient(t, s), acmeCertIssueArgs{
		cs:            certStateStore{StateStore: new(mem.Store)},
		logf:          logf,
		domain:        domain,
		challengeType: acmeChallengeTLSALPN01,
	})
	if err == nil {
		t.Fatal("issueACMECert succeeded; want an authorization error")
	}
	var ae *xacme.AuthorizationError
	if !errors.As(err, &ae) {
		t.Fatalf("error is %T, want *acme.AuthorizationError: %v", err, err)
	}
	if !strings.Contains(err.Error(), s.challengeError) {
		t.Errorf("error does not carry the CA's challenge detail:\n got: %v\nwant substring: %s", err, s.challengeError)
	}
	if !strings.Contains(err.Error(), domain) {
		t.Errorf("error does not name the identifier %q: %v", domain, err)
	}
	if n := s.pollCount("/order/1"); n != 0 {
		t.Errorf("order was polled %d times; the authorization error should be reported before WaitOrder", n)
	}
	if !logged.Load() {
		t.Error("the WaitAuthorization failure was not logged")
	}
}

// TestIssueACMECertValidAuthorization checks that waiting on the
// authorizations does not disturb the happy path: a valid authorization
// leads to a finalized order and a stored certificate.
func TestIssueACMECertValidAuthorization(t *testing.T) {
	const domain = "example.com"
	s := newFakeACMEServer(t, domain)

	e := &extension{logf: t.Logf}
	roots := x509.NewCertPool()
	roots.AddCert(s.ca.cert)
	cs := certStateStore{StateStore: new(mem.Store), testRoots: roots}
	pair, err := e.issueACMECert(context.Background(), nil, newTestACMEClient(t, s), acmeCertIssueArgs{
		cs:            cs,
		logf:          logger.Logf(t.Logf),
		domain:        domain,
		challengeType: acmeChallengeTLSALPN01,
	})
	if err != nil {
		t.Fatalf("issueACMECert: %v", err)
	}
	if pair == nil || len(pair.CertPEM) == 0 || len(pair.KeyPEM) == 0 {
		t.Fatalf("issueACMECert returned an incomplete pair: %+v", pair)
	}
	s.mu.Lock()
	served := string(s.certPEM)
	s.mu.Unlock()
	if string(pair.CertPEM) != served {
		t.Error("returned CertPEM is not the certificate the CA served")
	}
	if n := s.pollCount("/authz/1"); n < 2 {
		t.Errorf("authorization was fetched %d times; want the initial fetch plus at least one WaitAuthorization poll", n)
	}
	if _, err := cs.Read(domain, time.Now()); err != nil {
		t.Errorf("stored certificate does not read back: %v", err)
	}
}
