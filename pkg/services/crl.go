package services

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/lamassuiot/lamassuiot/v2/pkg/errs"
	"github.com/lamassuiot/lamassuiot/v2/pkg/helpers"
	"github.com/lamassuiot/lamassuiot/v2/pkg/models"
	"github.com/lamassuiot/lamassuiot/v2/pkg/resources"
	"github.com/sirupsen/logrus"
)

var crlValidate *validator.Validate

type CRLService interface {
	GetCRL(ctx context.Context, input GetCRLInput) ([]byte, error)
}

type crlServiceImpl struct {
	caSDK  CAService
	logger *logrus.Entry
}

type CRLServiceBuilder struct {
	Logger   *logrus.Entry
	CAClient CAService
}

func NewCRLService(builder CRLServiceBuilder) CRLService {
	crlValidate = validator.New()
	return &crlServiceImpl{
		caSDK:  builder.CAClient,
		logger: builder.Logger,
	}
}

type GetCRLInput struct {
	CAID string `validate:"required"`
}

func (svc crlServiceImpl) GetCRL(ctx context.Context, input GetCRLInput) ([]byte, error) {
	lFunc := helpers.ConfigureLogger(ctx, svc.logger)

	err := crlValidate.Struct(input)
	if err != nil {
		lFunc.Errorf("struct validation error: %s", err)
		return nil, errs.ErrValidateBadRequest
	}

	certList := []x509.RevocationListEntry{}
	lFunc.Debugf("reading CA %s certificates", input.CAID)
	_, err = svc.caSDK.GetCertificatesByCaAndStatus(ctx, GetCertificatesByCaAndStatusInput{
		CAID:   input.CAID,
		Status: models.StatusRevoked,
		ListInput: resources.ListInput[models.Certificate]{
			ExhaustiveRun: true,
			QueryParameters: &resources.QueryParameters{
				PageSize: 15,
			},
			ApplyFunc: func(cert models.Certificate) {
				certList = append(certList, x509.RevocationListEntry{
					SerialNumber:   cert.Certificate.SerialNumber,
					RevocationTime: time.Now(),
					Extensions:     []pkix.Extension{},
					ReasonCode:     int(cert.RevocationReason),
				})
			},
		},
	})
	if err != nil {
		lFunc.Errorf("something went wrong while reading CA %s certificates: %s", input.CAID, err)
		return nil, err
	}

	ca, err := svc.caSDK.GetCAByID(ctx, GetCAByIDInput(input))
	if err != nil {
		return nil, err
	}

	caSigner := NewCASigner(ctx, ca, svc.caSDK)
	caCert := (*x509.Certificate)(ca.Certificate.Certificate)

	lFunc.Debugf("creating revocation list. CA %s", input.CAID)
	now := time.Now()
	crl, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		RevokedCertificateEntries: certList,
		Number:                    big.NewInt(time.Now().UnixMilli()),
		ThisUpdate:                now,
		NextUpdate:                now.Add(time.Hour * 48),
	}, caCert, caSigner)
	if err != nil {
		lFunc.Errorf("something went wrong while creating revocation list: %s", err)
		return nil, err
	}

	return crl, nil
}
