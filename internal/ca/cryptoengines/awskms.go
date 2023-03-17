package cryptoengines

import (
	"context"
	"crypto"
	"crypto/elliptic"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/kms"
	"github.com/lamassuiot/lamassuiot/pkg/models"

	"github.com/lstoll/awskms"
	log "github.com/sirupsen/logrus"
)

type AWSKMSCryptoEngine struct {
	config models.CryptoEngineProvider
	kmscli *kms.KMS
}

func NewAWSKMSEngine(accessKeyID string, secretAccessKey string, region string) (CryptoEngine, error) {
	sess := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewStaticCredentials(accessKeyID, secretAccessKey, ""),
	}))
	kmscli := kms.New(sess)

	pkcs11ProviderSupportedKeyTypes := []models.SupportedKeyTypeInfo{}

	pkcs11ProviderSupportedKeyTypes = append(pkcs11ProviderSupportedKeyTypes, models.SupportedKeyTypeInfo{
		Type:        models.RSA,
		MinimumSize: 2048,
		MaximumSize: 4096,
	})

	pkcs11ProviderSupportedKeyTypes = append(pkcs11ProviderSupportedKeyTypes, models.SupportedKeyTypeInfo{
		Type:        models.ECDSA,
		MinimumSize: 256,
		MaximumSize: 512,
	})

	return &AWSKMSCryptoEngine{
		kmscli: kmscli,
		config: models.CryptoEngineProvider{
			Provider:          "Amazon Web Services",
			Manufacturer:      "AWS",
			Model:             "KMS",
			SupportedKeyTypes: pkcs11ProviderSupportedKeyTypes,
		},
	}, nil
}

func (p *AWSKMSCryptoEngine) GetEngineConfig() models.CryptoEngineProvider {
	return p.config
}

func (p *AWSKMSCryptoEngine) GetPrivateKeyByID(keyAlias string) (crypto.Signer, error) {
	var keyID = ""
	keys, err := p.kmscli.ListKeys(&kms.ListKeysInput{
		Limit: aws.Int64(100),
	})

	if err != nil {
		return nil, err
	}

	for _, key := range keys.Keys {
		aliases, err := p.kmscli.ListAliases(&kms.ListAliasesInput{
			KeyId: key.KeyId,
		})
		if err != nil {
			continue
		}

		for _, alias := range aliases.Aliases {
			aliasName := strings.Replace(*alias.AliasName, "alias/", "", -1)
			if aliasName == keyAlias {
				keyID = *key.KeyId
				break
			}
		}

		if keyID == keyAlias {
			break
		}
	}

	if keyID == "" {
		return nil, errors.New("kms key not found")
	}

	return awskms.NewSigner(context.Background(), p.kmscli, keyID)
}

func (p *AWSKMSCryptoEngine) CreateRSAPrivateKey(keySize int, keyID string) (crypto.Signer, error) {
	key, err := p.kmscli.CreateKey(&kms.CreateKeyInput{
		KeyUsage: aws.String("SIGN_VERIFY"),
		KeySpec:  aws.String(fmt.Sprintf("RSA_%d", keySize)),
	})

	if err != nil {
		return nil, err
	}

	log.Debug(fmt.Sprintf("RSA key created with ARN [%s]", *key.KeyMetadata.Arn))

	_, err = p.kmscli.CreateAlias(&kms.CreateAliasInput{
		AliasName:   aws.String(fmt.Sprintf("alias/%s", keyID)),
		TargetKeyId: key.KeyMetadata.Arn,
	})

	if err != nil {
		log.Warn(fmt.Sprintf("Could not create alias for key ARN [%s]: ", *key.KeyMetadata.Arn), err)
	}

	return p.GetPrivateKeyByID(keyID)
}

func (p *AWSKMSCryptoEngine) CreateECDSAPrivateKey(curve elliptic.Curve, keyID string) (crypto.Signer, error) {
	log.Warn("Creating ECDSA key with ", curve.Params().BitSize)
	key, err := p.kmscli.CreateKey(&kms.CreateKeyInput{
		KeyUsage: aws.String("SIGN_VERIFY"),
		KeySpec:  aws.String(fmt.Sprintf("ECC_NIST_P%d", curve.Params().BitSize)),
	})

	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}

	log.Debug(fmt.Sprintf("ECDSA key created with ARN [%s]", *key.KeyMetadata.Arn))

	_, err = p.kmscli.CreateAlias(&kms.CreateAliasInput{
		AliasName:   aws.String(fmt.Sprintf("alias/%s", keyID)),
		TargetKeyId: key.KeyMetadata.Arn,
	})

	if err != nil {
		log.Warn(fmt.Sprintf("Could not create alias for key ARN [%s]: ", *key.KeyMetadata.Arn), err)
	}

	return p.GetPrivateKeyByID(keyID)
}

func (p *AWSKMSCryptoEngine) DeleteAllKeys() error {
	return fmt.Errorf("TODO")
}

func (p *AWSKMSCryptoEngine) DeleteKey(keyID string) error {
	return errors.New(fmt.Sprintf("cannot delete key [%s]. Go to your aws account and do it manually", keyID))
}
