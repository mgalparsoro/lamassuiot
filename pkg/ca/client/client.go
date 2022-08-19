package client

import (
	"context"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"

	"github.com/lamassuiot/lamassuiot/pkg/ca/common/api"
	clientUtils "github.com/lamassuiot/lamassuiot/pkg/utils/client"
	clientFilers "github.com/lamassuiot/lamassuiot/pkg/utils/client/filters"
	"github.com/lamassuiot/lamassuiot/pkg/utils/common"
)

const (
	//Generic Errors
	ErrValidationError = "validation error"

	//Specific Errors
	ErrCADoesNotExist           = "CA does not exist"
	ErrCAAlreadyRevoked         = "CA already revoked"
	ErrDuplicateCA              = "duplicate CA"
	ErrCertificateDoesNotExist  = "certificate does not exist"
	ErrCerificateAlreadyRevoked = "certificate already revoked"
)

type LamassuCAClient interface {
	CreateCA(ctx context.Context, input *api.CreateCAInput) (*api.CreateCAOutput, error)
	GetCAs(ctx context.Context, input *api.GetCAsInput) (*api.GetCAsOutput, error)
	GetCAByName(ctx context.Context, input *api.GetCAByNameInput) (*api.GetCAByNameOutput, error)
	// ImportCA(ctx context.Context, input *api.ImportCAInput) (*api.ImportCAOutput, error)
	RevokeCA(ctx context.Context, input *api.RevokeCAInput) (*api.RevokeCAOutput, error)
	IterateCAsWithPredicate(ctx context.Context, input *api.IterateCAsWithPredicateInput) (*api.IterateCAsWithPredicateOutput, error)

	SignCertificateRequest(ctx context.Context, input *api.SignCertificateRequestInput) (*api.SignCertificateRequestOutput, error)
	RevokeCertificate(ctx context.Context, input *api.RevokeCertificateInput) (*api.RevokeCertificateOutput, error)
	GetCertificateBySerialNumber(ctx context.Context, input *api.GetCertificateBySerialNumberInput) (*api.GetCertificateBySerialNumberOutput, error)
	GetCertificates(ctx context.Context, input *api.GetCertificatesInput) (*api.GetCertificatesOutput, error)
	IterateCertificatessWithPredicate(ctx context.Context, input *api.IterateCertificatesWithPredicateInput) (*api.IterateCertificatesWithPredicateOutput, error)
}

type lamassuCaClientConfig struct {
	client clientUtils.BaseClient
}

func NewLamassuCAClient(config clientUtils.BaseClientConfigurationuration) (LamassuCAClient, error) {
	baseClient, err := clientUtils.NewBaseClient(config)
	if err != nil {
		return nil, err
	}

	return &lamassuCaClientConfig{
		client: baseClient,
	}, nil
}

func (c *lamassuCaClientConfig) GetCAs(ctx context.Context, input *api.GetCAsInput) (*api.GetCAsOutput, error) {

	req, err := c.client.NewRequest("GET", "v1/"+string(input.CAType), nil)
	if err != nil {
		return &api.GetCAsOutput{}, err
	}

	newParams := clientFilers.GenerateHttpQueryParams(input.QueryParameters)
	req.URL.RawQuery = newParams

	var output api.GetCAsOutputSerialized
	_, err = c.client.Do2(req, &output)

	if err != nil {
		return &api.GetCAsOutput{}, err
	}

	deserializedOutput := output.Deserialize()
	return &deserializedOutput, nil
}

func (c *lamassuCaClientConfig) GetCAByName(ctx context.Context, input *api.GetCAByNameInput) (*api.GetCAByNameOutput, error) {
	req, err := c.client.NewRequest("GET", "v1/"+string(input.CAType)+"/"+input.CAName, nil)
	if err != nil {
		return &api.GetCAByNameOutput{}, err
	}

	var output api.GetCAByNameOutputSerialized
	_, err = c.client.Do2(req, &output)

	if err != nil {
		return &api.GetCAByNameOutput{}, err
	}

	deserializedOutput := output.Deserialize()
	return &deserializedOutput, nil
}

func (c *lamassuCaClientConfig) IterateCAsWithPredicate(ctx context.Context, input *api.IterateCAsWithPredicateInput) (*api.IterateCAsWithPredicateOutput, error) {
	limit := 100
	i := 0

	var cas []api.CACertificate = make([]api.CACertificate, 0)

	for {
		getCAsOutput, err := c.GetCAs(ctx, &api.GetCAsInput{
			CAType: api.CATypePKI,
			QueryParameters: common.QueryParameters{
				Pagination: common.PaginationOptions{
					Limit:  i,
					Offset: i * limit,
				},
			},
		})
		if err != nil {
			return &api.IterateCAsWithPredicateOutput{}, errors.New("could not get CAs")
		}

		if len(getCAsOutput.CAs) == 0 {
			break
		}

		cas = append(cas, getCAsOutput.CAs...)
		i++
	}

	for _, ca := range cas {
		input.PredicateFunc(&ca)
	}

	return &api.IterateCAsWithPredicateOutput{}, nil
}

func (c *lamassuCaClientConfig) CreateCA(ctx context.Context, input *api.CreateCAInput) (*api.CreateCAOutput, error) {
	body := api.CreateCAPayload{
		KeyMetadata: api.CreacteCAKeyMetadataSubject{
			KeyType: string(input.KeyMetadata.KeyType),
			KeyBits: input.KeyMetadata.KeyBits,
		},
		Subject: api.CreateCASubjectPayload{
			CommonName:       input.Subject.CommonName,
			Country:          input.Subject.Country,
			State:            input.Subject.State,
			Locality:         input.Subject.Locality,
			Organization:     input.Subject.Organization,
			OrganizationUnit: input.Subject.OrganizationUnit,
		},
		CADuration:       int(input.CADuration.Seconds()),
		IssuanceDuration: int(input.IssuanceDuration.Seconds()),
	}

	req, err := c.client.NewRequest("POST", "v1/pki", body)

	if err != nil {
		return &api.CreateCAOutput{}, err
	}

	var output api.CreateCAOutputSerialized
	resp, err := c.client.Do2(req, &output)

	if err != nil {
		if resp.StatusCode == http.StatusConflict {
			return &api.CreateCAOutput{}, errors.New(ErrDuplicateCA)
		}

		return &api.CreateCAOutput{}, err
	}

	deserializedOutput := output.Deserialize()
	return &deserializedOutput, nil
}

// func (c *lamassuCaClientConfig) ImportCA(ctx context.Context, input *api.ImportCAInput) (*api.ImportCAOutput, error) {
// 	crtBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: input.Certificate.Raw})
// 	base64CrtContent := base64.StdEncoding.EncodeToString(crtBytes)
// 	privKeyString, _ := input.PrivateKey.GetPEMString()
// 	base64CKeyContent := base64.StdEncoding.EncodeToString([]byte(privKeyString))

// 	body := struct {
// 		IssuanceDuration int    `json:"issuance_duration"`
// 		Certificate      string `json:"certificate"`
// 		PrivateKey       string `json:"private_key"`
// 	}{
// 		Certificate:      base64CrtContent,
// 		PrivateKey:       base64CKeyContent,
// 		IssuanceDuration: int(input.IssuanceDuration.Hours()),
// 	}

// 	req, err := c.client.NewRequest("POST", "v1/"+string(input.CAType)+"/import/"+string(input.Certificate.Subject.CommonName), body)
// 	if err != nil {
// 		return &api.ImportCAOutput{}, err
// 	}

// 	var output api.ImportCAOutput
// 	_, err = c.client.Do2(req, &output)
// 	if err != nil {
// 		return &output, err
// 	}

// 	return &output, err
// }

func (c *lamassuCaClientConfig) RevokeCA(ctx context.Context, input *api.RevokeCAInput) (*api.RevokeCAOutput, error) {
	body := api.RevokeCAPayload{
		RevocationReason: input.RevocationReason,
	}

	req, err := c.client.NewRequest("DELETE", "v1/"+string(input.CAType)+"/"+input.CAName, body)
	if err != nil {
		return &api.RevokeCAOutput{}, err
	}

	var output api.RevokeCAOutputSerialized
	_, err = c.client.Do2(req, &output)
	if err != nil {
		return &api.RevokeCAOutput{}, err
	}

	deserializedOutput := output.Deserialize()
	return &deserializedOutput, nil
}

func (c *lamassuCaClientConfig) SignCertificateRequest(ctx context.Context, input *api.SignCertificateRequestInput) (*api.SignCertificateRequestOutput, error) {
	csrBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: input.CertificateSigningRequest.Raw})
	base64CsrContent := base64.StdEncoding.EncodeToString(csrBytes)
	body := api.SignCertificateRequestPayload{
		CertificateRequest: base64CsrContent,
		CommonName:         input.CommonName,
		SignVerbatim:       input.SignVerbatim,
	}

	req, err := c.client.NewRequest("POST", "v1/"+string(input.CAType)+"/"+input.CAName+"/sign", body)

	if err != nil {
		return &api.SignCertificateRequestOutput{}, err
	}

	var output api.SignCertificateRequestOutputSerialized
	_, err = c.client.Do2(req, &output)

	if err != nil {
		return &api.SignCertificateRequestOutput{}, err
	}

	deserializedOutput := output.Deserialize()
	return &deserializedOutput, nil
}

func (c *lamassuCaClientConfig) RevokeCertificate(ctx context.Context, input *api.RevokeCertificateInput) (*api.RevokeCertificateOutput, error) {
	body := api.RevokeCertificatePayload{
		RevocationReason: input.RevocationReason,
	}

	req, err := c.client.NewRequest("DELETE", "v1/"+string(input.CAType)+"/"+input.CAName+"/certificates/"+input.CertificateSerialNumber, body)
	if err != nil {
		return &api.RevokeCertificateOutput{}, nil
	}

	var output api.RevokeCertificateOutputSerialized
	response, err := c.client.Do2(req, &output)
	if response.StatusCode == 409 {
		return &api.RevokeCertificateOutput{}, &AlreadyRevokedError{
			CaName:       input.CAName,
			SerialNumber: input.CertificateSerialNumber,
		}
	} else if err != nil {
		return &api.RevokeCertificateOutput{}, err
	}

	deserializedOutput := output.Deserialize()
	return &deserializedOutput, nil
}

func (c *lamassuCaClientConfig) GetCertificateBySerialNumber(ctx context.Context, input *api.GetCertificateBySerialNumberInput) (*api.GetCertificateBySerialNumberOutput, error) {
	req, err := c.client.NewRequest("GET", "v1/"+string(input.CAType)+"/"+input.CAName+"/certificates/"+input.CertificateSerialNumber, nil)
	if err != nil {
		return &api.GetCertificateBySerialNumberOutput{}, err
	}

	var output api.GetCertificateBySerialNumberOutputSerialized
	_, err = c.client.Do2(req, &output)
	if err != nil {
		return &api.GetCertificateBySerialNumberOutput{}, err
	}

	deserializedOutput := output.Deserialize()
	return &deserializedOutput, nil
}

func (c *lamassuCaClientConfig) GetCertificates(ctx context.Context, input *api.GetCertificatesInput) (*api.GetCertificatesOutput, error) {
	req, err := c.client.NewRequest("GET", "v1/"+string(input.CAType)+"/"+input.CAName+"/issued", nil)
	if err != nil {
		return &api.GetCertificatesOutput{}, err
	}

	newParams := clientFilers.GenerateHttpQueryParams(input.QueryParameters)
	req.URL.RawQuery = newParams

	var output api.GetCertificatesOutputSerialized
	_, err = c.client.Do2(req, &output)
	if err != nil {
		return &api.GetCertificatesOutput{}, err
	}

	deserializedOutput := output.Deserialize()
	return &deserializedOutput, nil
}

func (c *lamassuCaClientConfig) IterateCertificatessWithPredicate(ctx context.Context, input *api.IterateCertificatesWithPredicateInput) (*api.IterateCertificatesWithPredicateOutput, error) {
	limit := 100
	i := 0

	var certs []api.Certificate = make([]api.Certificate, 0)

	for {
		getCAsOutput, err := c.GetCertificates(ctx, &api.GetCertificatesInput{
			CAType: api.CATypePKI,
			CAName: input.CAName,
			QueryParameters: common.QueryParameters{
				Pagination: common.PaginationOptions{
					Limit:  i,
					Offset: i * limit,
				},
			},
		})
		if err != nil {
			return &api.IterateCertificatesWithPredicateOutput{}, errors.New("could not get Certificates")
		}

		if len(getCAsOutput.Certificates) == 0 {
			break
		}
		certs = append(certs, getCAsOutput.Certificates...)
		i++
	}

	for _, cert := range certs {
		input.PredicateFunc(&cert)
	}

	return &api.IterateCertificatesWithPredicateOutput{}, nil
}

type AlreadyRevokedError struct {
	CaName       string
	SerialNumber string
}
type AlreadyRevokedCAError struct {
	CaName string
}

func (e *AlreadyRevokedError) Error() string {
	return fmt.Sprintf("certificate already revoked. CA name=%s Cert Serial Number=%s", e.CaName, e.SerialNumber)
}

func (e *AlreadyRevokedCAError) Error() string {
	return fmt.Sprintf("CA already revoked: %s", e.CaName)
}
