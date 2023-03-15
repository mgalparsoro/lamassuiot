package services

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"

	"github.com/lamassuiot/lamassuiot/pkg/models"
	"github.com/lamassuiot/lamassuiot/pkg/resources"
)

type ListInput[E any] struct {
	QueryParameters *resources.QueryParameters
	ExhaustiveRun   bool //wether to iter all elems
	ApplyFunc       func(cert *E)
}

func insertNth(s string, n int, sep rune) string {
	if len(s)%2 != 0 {
		s = "0" + s
	}
	var buffer bytes.Buffer
	var n_1 = n - 1
	var l_1 = len(s) - 1
	for i, rune := range s {
		buffer.WriteRune(rune)
		if i%n == n_1 && i != l_1 {
			buffer.WriteRune(sep)
		}
	}
	return buffer.String()
}

func toHexInt(n *big.Int) string {
	return fmt.Sprintf("%x", n) // or %X or upper case
}

func SerialNumberToString(n *big.Int) string {
	return insertNth(toHexInt(n), 2, '-')
}

func SubjectToPkixName(subj models.Subject) pkix.Name {
	subjPkix := pkix.Name{}

	if subj.CommonName != "" {
		subjPkix.CommonName = subj.CommonName
	}

	if subj.Country != "" {
		subjPkix.Country = []string{
			subj.Country,
		}
	}

	if subj.Locality != "" {
		subjPkix.Locality = []string{
			subj.Locality,
		}
	}

	if subj.Organization != "" {
		subjPkix.Organization = []string{
			subj.Organization,
		}
	}

	if subj.OrganizationUnit != "" {
		subjPkix.OrganizationalUnit = []string{
			subj.OrganizationUnit,
		}
	}

	if subj.State != "" {
		subjPkix.Province = []string{
			subj.State,
		}
	}

	return subjPkix
}

func PkixNameToSubject(pkixName pkix.Name) models.Subject {
	subject := models.Subject{
		CommonName: pkixName.CommonName,
	}

	if len(pkixName.Country) > 0 {
		subject.Country = pkixName.Country[0]
	}
	if len(pkixName.Organization) > 0 {
		subject.Organization = pkixName.Organization[0]
	}
	if len(pkixName.OrganizationalUnit) > 0 {
		subject.OrganizationUnit = pkixName.OrganizationalUnit[0]
	}
	if len(pkixName.Locality) > 0 {
		subject.Locality = pkixName.Locality[0]
	}
	if len(pkixName.Province) > 0 {
		subject.State = pkixName.Province[0]
	}

	return subject
}

func KeyStrengthMetadataFromCertificate(cert *x509.Certificate) models.KeyStrengthMetadata {
	var keyType models.KeyType
	var keyBits int
	switch cert.PublicKeyAlgorithm.String() {
	case "RSA":
		keyType = models.RSA
		keyBits = cert.PublicKey.(*rsa.PublicKey).N.BitLen()
	case "ECDSA":
		keyType = models.ECDSA
		keyBits = cert.PublicKey.(*ecdsa.PublicKey).Params().BitSize
	}

	var keyStrength models.KeyStrength = models.KeyStrengthLow
	switch keyType {
	case models.RSA:
		if keyBits < 2048 {
			keyStrength = models.KeyStrengthLow
		} else if keyBits >= 2048 && keyBits < 3072 {
			keyStrength = models.KeyStrengthMedium
		} else {
			keyStrength = models.KeyStrengthHigh
		}
	case models.ECDSA:
		if keyBits <= 128 {
			keyStrength = models.KeyStrengthLow
		} else if keyBits > 128 && keyBits < 256 {
			keyStrength = models.KeyStrengthMedium
		} else {
			keyStrength = models.KeyStrengthHigh
		}
	}

	return models.KeyStrengthMetadata{
		Type:     keyType,
		Bits:     keyBits,
		Strength: keyStrength,
	}
}

func ReadCertificateFromFile(filePath string) (*x509.Certificate, error) {
	if filePath == "" {
		return nil, fmt.Errorf("cannot open empty filepath")
	}

	certFileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	certDERBlock, _ := pem.Decode(certFileBytes)

	return x509.ParseCertificate(certDERBlock.Bytes)
}
