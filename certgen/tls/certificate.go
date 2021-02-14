package tls

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"time"
)

// Name describes a X.509 name object
type Name struct {
	Organization string
	City         string
	Province     string
	Country      string
	CommonName   string
}

func (n Name) pkix() pkix.Name {
	return pkix.Name{
		Country: []string{
			n.Country,
		},
		Organization: []string{
			n.Organization,
		},
		Locality: []string{
			n.City,
		},
		Province: []string{
			n.Province,
		},
		CommonName: n.CommonName,
	}
}

func nameFromPkix(p pkix.Name) Name {
	n := Name{
		CommonName: p.CommonName,
	}

	if len(p.Organization) > 0 {
		n.Organization = p.Organization[0]
	}

	if len(p.Locality) > 0 {
		n.City = p.Locality[0]
	}

	if len(p.Province) > 0 {
		n.Province = p.Province[0]
	}

	if len(p.Country) > 0 {
		n.Country = p.Country[0]
	}

	return n
}

// DateRange describes a date range
type DateRange struct {
	NotBefore time.Time
	NotAfter  time.Time
}

// IsValid is the current time and date between this date range
func (d DateRange) IsValid() bool {
	return time.Since(d.NotAfter) < 0 && time.Since(d.NotBefore) > 0
}

const (
	// AlternateNameTypeDNS enum value for DNS type alternate names
	AlternateNameTypeDNS = "dns"
	// AlternateNameTypeEmail enum value for Email type alternate names
	AlternateNameTypeEmail = "email"
	// AlternateNameTypeIP enum value for IP type alternate names
	AlternateNameTypeIP = "ip"
	// AlternateNameTypeURI enum value for URI type alternate names
	AlternateNameTypeURI = "uri"
)

// AlternateName describes an alternate name
type AlternateName struct {
	Type  string
	Value string
}

// KeyUsage describes usage properties for an X.509 key
type KeyUsage struct {
	// Basic
	DigitalSignature  bool
	ContentCommitment bool
	KeyEncipherment   bool
	DataEncipherment  bool
	KeyAgreement      bool
	CertSign          bool
	CRLSign           bool
	EncipherOnly      bool
	DecipherOnly      bool

	// Extended
	ServerAuth      bool
	ClientAuth      bool
	CodeSigning     bool
	EmailProtection bool
	TimeStamping    bool
	OCSPSigning     bool
}

func (u KeyUsage) usage() x509.KeyUsage {
	var usage x509.KeyUsage

	if u.DigitalSignature {
		usage = usage | x509.KeyUsageDigitalSignature
	}
	if u.ContentCommitment {
		usage = usage | x509.KeyUsageContentCommitment
	}
	if u.KeyEncipherment {
		usage = usage | x509.KeyUsageKeyEncipherment
	}
	if u.DataEncipherment {
		usage = usage | x509.KeyUsageDataEncipherment
	}
	if u.KeyAgreement {
		usage = usage | x509.KeyUsageKeyAgreement
	}
	if u.CertSign {
		usage = usage | x509.KeyUsageCertSign
	}
	if u.CRLSign {
		usage = usage | x509.KeyUsageCRLSign
	}
	if u.EncipherOnly {
		usage = usage | x509.KeyUsageEncipherOnly
	}
	if u.DecipherOnly {
		usage = usage | x509.KeyUsageDecipherOnly
	}

	return usage
}

func (u KeyUsage) extendedUsage() []x509.ExtKeyUsage {
	usage := []x509.ExtKeyUsage{}
	if u.ServerAuth {
		usage = append(usage, x509.ExtKeyUsageServerAuth)
	}
	if u.ClientAuth {
		usage = append(usage, x509.ExtKeyUsageClientAuth)
	}
	if u.CodeSigning {
		usage = append(usage, x509.ExtKeyUsageCodeSigning)
	}
	if u.EmailProtection {
		usage = append(usage, x509.ExtKeyUsageEmailProtection)
	}
	if u.TimeStamping {
		usage = append(usage, x509.ExtKeyUsageTimeStamping)
	}
	if u.OCSPSigning {
		usage = append(usage, x509.ExtKeyUsageOCSPSigning)
	}
	return usage
}

// CertificateRequest describes a certificate request
type CertificateRequest struct {
	Subject                Name
	Validity               DateRange
	AlternateNames         []AlternateName
	Usage                  KeyUsage
	IsCertificateAuthority bool
	StatusProviders        StatusProviders
}

// StatusProviders describes providers for certificate status
type StatusProviders struct {
	CRL  *string
	OCSP *string
}

// Certificate describes a certificate
type Certificate struct {
	Serial               string
	Subject              Name
	CertificateAuthority bool
	CertificateData      string
	KeyData              string
}

func (c Certificate) certificateDataBytes() []byte {
	data, err := hex.DecodeString(c.CertificateData)
	if err != nil {
		panic(err)
	}
	return data
}

func (c Certificate) keyDataBytes() []byte {
	data, err := hex.DecodeString(c.KeyData)
	if err != nil {
		panic(err)
	}
	return data
}

// Description return a script description of the certificate
func (c Certificate) Description() string {
	return fmt.Sprintf("%v", nameFromPkix(c.x509().Subject))
}

// x509 return the x509.Certificate data structure for this certificate (reading from the
// CertificateData bytes). This will panic on an error, but that shouldn't happen unless
// CertificateData was corrupted.
func (c Certificate) x509() *x509.Certificate {
	x, err := x509.ParseCertificate(c.certificateDataBytes())
	if err != nil {
		panic(err)
	}
	return x
}

// pKey return the crypto.PrivateKey structure for this certificate (reading from the KeyData bytes).
// This will panic on an error, but that shouldn't happen unless KeyData was corrupted.
func (c Certificate) pKey() crypto.PrivateKey {
	k, err := x509.ParsePKCS8PrivateKey(c.keyDataBytes())
	if err != nil {
		panic(err)
	}
	return k
}

// GenerateCertificate will generate a certificate from the given certificate request
func GenerateCertificate(request CertificateRequest, issuer *Certificate) (*Certificate, error) {
	pKey, err := generateKey()
	if err != nil {
		return nil, err
	}
	pub := pKey.(crypto.Signer).Public()
	serial, err := randomSerialNumber()
	if err != nil {
		return nil, err
	}

	pKeyBytes, err := x509.MarshalPKCS8PrivateKey(pKey)
	if err != nil {
		return nil, err
	}

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}
	h := sha1.Sum(publicKeyBytes)

	certificate := Certificate{
		Serial:               serial.String(),
		CertificateAuthority: issuer == nil,
		KeyData:              hex.EncodeToString(pKeyBytes),
		Subject:              request.Subject,
	}

	tpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               request.Subject.pkix(),
		NotBefore:             request.Validity.NotBefore,
		NotAfter:              request.Validity.NotAfter,
		KeyUsage:              request.Usage.usage(),
		BasicConstraintsValid: true,
		SubjectKeyId:          h[:],
		ExtKeyUsage:           []x509.ExtKeyUsage{},
	}

	if issuer != nil {
		tpl.Issuer = issuer.x509().Subject
	}

	for _, name := range request.AlternateNames {
		switch name.Type {
		case AlternateNameTypeDNS:
			tpl.DNSNames = append(tpl.DNSNames, name.Value)
			break
		case AlternateNameTypeEmail:
			tpl.EmailAddresses = append(tpl.EmailAddresses, name.Value)
			break
		case AlternateNameTypeIP:
			ip := net.ParseIP(name.Value)
			if ip == nil {
				return nil, fmt.Errorf("invalid ip address %s", name.Value)
			}
			tpl.IPAddresses = append(tpl.IPAddresses, ip)
			break
		case AlternateNameTypeURI:
			u, err := url.Parse(name.Value)
			if err != nil {
				return nil, err
			}
			tpl.URIs = append(tpl.URIs, u)
			break
		default:
			break
		}
	}

	if request.StatusProviders.CRL != nil {
		tpl.CRLDistributionPoints = []string{*request.StatusProviders.CRL}
	}

	if request.StatusProviders.OCSP != nil {
		tpl.OCSPServer = []string{*request.StatusProviders.OCSP}
	}

	var certBytes []byte

	if issuer == nil {
		certBytes, err = x509.CreateCertificate(rand.Reader, tpl, tpl, pub, pKey)
		if err != nil {
			return nil, err
		}
	} else {
		certBytes, err = x509.CreateCertificate(rand.Reader, tpl, issuer.x509(), pub, issuer.pKey())
		if err != nil {
			return nil, err
		}
	}

	certificate.CertificateData = hex.EncodeToString(certBytes)
	return &certificate, nil
}

func randomSerialNumber() (*big.Int, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, serialNumberLimit)
}

func generateKey() (crypto.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}
